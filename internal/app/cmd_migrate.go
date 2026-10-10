package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/redact"
)

func init() {
	register(Command{
		Name:    "migrate",
		Summary: "Copy legacy ~/.config/muthuishere-agent-skills state into ~/.config/.huddle",
		Run:     runMigrate,
	})
}

const migrateHelp = `
Usage: huddle migrate [--dry-run] [--json]

Copies (never moves, never overwrites) from the legacy roots:
  personas/                           -> global personas
  <repo>/personas/                    -> global personas (reported per repo)
  <repo>/<branch>/huddle/raw/*.json   -> events in repos/<repo>/events.jsonl
  userconfig.json git_user            -> global config "user" (if unset)
Idempotent: imported raw files are recorded in migrated.json.
`

// migratedRecord is ~/.config/.huddle/migrated.json.
type migratedRecord struct {
	SchemaVersion int               `json:"schemaVersion"`
	MigratedAt    time.Time         `json:"migratedAt"`
	Sources       []string          `json:"sources"`
	RawFiles      map[string]string `json:"rawFiles"` // legacy raw path -> event id
}

// MigrateReport is what migrate did (or would do with --dry-run).
type MigrateReport struct {
	DryRun          bool           `json:"dryRun"`
	Sources         []string       `json:"sources"`
	PersonaFiles    int            `json:"personaFilesCopied"`
	PersonaSkipped  int            `json:"personaFilesSkipped"`
	RepoPersonas    map[string]int `json:"repoPersonaFilesCopied,omitempty"`
	EventsAppended  map[string]int `json:"eventsAppended,omitempty"`
	EventsSkipped   int            `json:"eventsAlreadyMigrated"`
	UserSet         string         `json:"userSet,omitempty"`
	Errors          []string       `json:"errors,omitempty"`
	AlreadyComplete bool           `json:"alreadyComplete"`
}

func runMigrate(c *Ctx, args []string) error {
	a, err := parseCoreArgs("migrate", args, nil, []string{"dry-run", "json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, migrateHelp)
	}
	rep, err := runLegacyMigration(a.bools["dry-run"])
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, rep)
	}
	if len(rep.Sources) == 0 {
		fmt.Fprintln(c.Stdout, "MIGRATE: nothing to do — no legacy root found")
		return nil
	}
	mode := "done"
	if rep.DryRun {
		mode = "dry-run (nothing written — run: huddle migrate)"
	}
	events := 0
	for _, n := range rep.EventsAppended {
		events += n
	}
	output.KV(c.Stdout,
		"MIGRATE", mode,
		"SOURCES", strings.Join(rep.Sources, ", "),
		"PERSONA_FILES_COPIED", fmt.Sprint(rep.PersonaFiles),
		"PERSONA_FILES_SKIPPED", fmt.Sprint(rep.PersonaSkipped),
		"EVENTS_APPENDED", fmt.Sprint(events),
		"EVENTS_ALREADY_MIGRATED", fmt.Sprint(rep.EventsSkipped))
	for _, repoName := range coreSortedKeys(rep.RepoPersonas) {
		output.KV(c.Stdout, "REPO_PERSONAS", fmt.Sprintf("%s: %d files -> %s", repoName, rep.RepoPersonas[repoName], paths.GlobalPersonas()))
	}
	for _, repoName := range coreSortedKeys(rep.EventsAppended) {
		output.KV(c.Stdout, "REPO_EVENTS", fmt.Sprintf("%s: %d", repoName, rep.EventsAppended[repoName]))
	}
	if rep.UserSet != "" {
		output.KV(c.Stdout, "USER", rep.UserSet)
	}
	for _, e := range rep.Errors {
		output.KV(c.Stdout, "ERROR", e)
	}
	return nil
}

func coreSortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// legacyMigrationNeeded is the cheap check used by `huddle start`.
func legacyMigrationNeeded() bool {
	if _, err := os.Stat(paths.Migrated()); err == nil {
		return false
	}
	for _, root := range paths.LegacyRoots() {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func loadMigrated() migratedRecord {
	rec := migratedRecord{SchemaVersion: 1, RawFiles: map[string]string{}}
	b, err := os.ReadFile(paths.Migrated())
	if err != nil {
		return rec
	}
	if json.Unmarshal(b, &rec) != nil || rec.RawFiles == nil {
		rec.RawFiles = map[string]string{}
	}
	return rec
}

func runLegacyMigration(dryRun bool) (MigrateReport, error) {
	rep := MigrateReport{DryRun: dryRun, RepoPersonas: map[string]int{}, EventsAppended: map[string]int{}}
	rec := loadMigrated()
	var roots []string
	for _, root := range paths.LegacyRoots() {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			roots = append(roots, root)
		}
	}
	rep.Sources = roots
	if len(roots) == 0 {
		return rep, nil
	}
	for _, root := range roots {
		// Global personas.
		copied, skipped, errs := copyTreeNoOverwrite(filepath.Join(root, "personas"), paths.GlobalPersonas(), dryRun)
		rep.PersonaFiles += copied
		rep.PersonaSkipped += skipped
		rep.Errors = append(rep.Errors, errs...)

		entries, err := os.ReadDir(root)
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() || ent.Name() == "personas" || strings.HasPrefix(ent.Name(), ".") {
				continue
			}
			repoName := ent.Name()
			repoDir := filepath.Join(root, repoName)
			copied, skipped, errs := copyTreeNoOverwrite(filepath.Join(repoDir, "personas"), paths.GlobalPersonas(), dryRun)
			if copied > 0 {
				rep.RepoPersonas[repoName] += copied
			}
			rep.PersonaFiles += copied
			rep.PersonaSkipped += skipped
			rep.Errors = append(rep.Errors, errs...)
			migrateRepoEvents(&rep, &rec, repoDir, repoName, dryRun)
		}
		migrateUser(&rep, root, dryRun)
	}
	if len(rep.RepoPersonas) == 0 {
		rep.RepoPersonas = nil
	}
	if len(rep.EventsAppended) == 0 {
		rep.EventsAppended = nil
	}
	rep.AlreadyComplete = rep.PersonaFiles == 0 && rep.EventsAppended == nil && rep.UserSet == ""
	if dryRun {
		return rep, nil
	}
	rec.SchemaVersion = 1
	rec.MigratedAt = time.Now().UTC()
	rec.Sources = roots
	if err := config.WriteJSON(paths.Migrated(), rec); err != nil {
		return rep, fmt.Errorf("write %s: %w — check permissions on %s", paths.Migrated(), err, paths.Global())
	}
	return rep, nil
}

func migrateRepoEvents(rep *MigrateReport, rec *migratedRecord, repoDir, repoName string, dryRun bool) {
	branches, err := os.ReadDir(repoDir)
	if err != nil {
		return
	}
	type rawEvent struct {
		path   string
		branch string
		ev     eventlog.Event
	}
	var batch []rawEvent
	for _, br := range branches {
		if !br.IsDir() || br.Name() == "personas" {
			continue
		}
		rawDir := filepath.Join(repoDir, br.Name(), "huddle", "raw")
		files, err := os.ReadDir(rawDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			p := filepath.Join(rawDir, f.Name())
			if _, done := rec.RawFiles[p]; done {
				rep.EventsSkipped++
				continue
			}
			ev, err := rawToEvent(p, br.Name())
			if err != nil {
				rep.Errors = append(rep.Errors, err.Error())
				continue
			}
			batch = append(batch, rawEvent{path: p, branch: br.Name(), ev: ev})
		}
	}
	// Oldest first so the log stays chronological.
	sort.SliceStable(batch, func(i, j int) bool { return batch[i].ev.TS.Before(batch[j].ev.TS) })
	for _, r := range batch {
		if dryRun {
			rep.EventsAppended[repoName]++
			continue
		}
		e, err := eventlog.Append(repoName, r.ev)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", r.path, err))
			continue
		}
		rec.RawFiles[r.path] = e.ID
		rep.EventsAppended[repoName]++
	}
}

// rawToEvent maps a legacy raw event file onto an Event. Unknown fields go to Data.
func rawToEvent(path, branch string) (eventlog.Event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return eventlog.Event{}, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return eventlog.Event{}, fmt.Errorf("%s is not valid JSON, skipped: %v", path, err)
	}
	str := func(k string) string {
		s, _ := m[k].(string)
		delete(m, k)
		return s
	}
	typ := str("type")
	if typ == "" {
		typ = str("kind")
	}
	if typ == "" {
		typ = "note"
		if base := filepath.Base(path); strings.Contains(base, "_") {
			typ = strings.TrimSuffix(base[strings.LastIndex(base, "_")+1:], ".json")
		}
	}
	ev := eventlog.Event{Type: typ, Branch: branch, By: redact.String(str("by")), Door: str("door")}
	switch v := m["ts"].(type) {
	case float64:
		ev.TS = time.UnixMilli(int64(v)).UTC()
		if v < 1e11 { // seconds, not milliseconds
			ev.TS = time.Unix(int64(v), 0).UTC()
		}
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			ev.TS = t.UTC()
		}
	}
	delete(m, "ts")
	if ev.TS.IsZero() {
		if st, err := os.Stat(path); err == nil {
			ev.TS = st.ModTime().UTC()
		}
	}
	for _, k := range []string{"topic", "title", "summary", "text", "decision"} {
		if s := str(k); s != "" && ev.Text == "" {
			ev.Text = redact.String(s)
		} else if s != "" {
			m[k] = s
		}
	}
	if content := str("content"); content != "" {
		if ev.Text == "" {
			ev.Text = redact.String(content)
		} else {
			m["content"] = redact.String(content)
		}
	}
	if ev.Door != "one-way" && ev.Door != "two-way" {
		ev.Door = ""
	}
	m["migratedFrom"] = path
	// Redact every remaining string value, however nested.
	ev.Data = coreRedactAny(m).(map[string]any)
	return ev, nil
}

func coreRedactAny(v any) any {
	switch x := v.(type) {
	case string:
		return redact.String(x)
	case []any:
		for i := range x {
			x[i] = coreRedactAny(x[i])
		}
		return x
	case map[string]any:
		for k := range x {
			x[k] = coreRedactAny(x[k])
		}
		return x
	}
	return v
}

func migrateUser(rep *MigrateReport, root string, dryRun bool) {
	b, err := os.ReadFile(filepath.Join(root, "userconfig.json"))
	if err != nil {
		return
	}
	var uc struct {
		GitUser string `json:"git_user"`
	}
	if json.Unmarshal(b, &uc) != nil || strings.TrimSpace(uc.GitUser) == "" {
		return
	}
	g, err := scopeView(config.Global, "")
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	if u, _ := g["user"].(string); u != "" {
		return
	}
	rep.UserSet = uc.GitUser
	if dryRun {
		return
	}
	val, _ := json.Marshal(uc.GitUser)
	if err := config.Set(config.Global, "", "user", string(val)); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
}

// copyTreeNoOverwrite copies regular files from src into dst, skipping any
// destination file that already exists. A missing src is not an error.
func copyTreeNoOverwrite(src, dst string, dryRun bool) (copied, skipped int, errs []string) {
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return 0, 0, nil
	}
	_ = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if _, err := os.Lstat(target); err == nil {
			skipped++
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err.Error())
			return nil
		}
		copied++
		if dryRun {
			return nil
		}
		if err := coreCopyFileExcl(p, target); err != nil {
			copied--
			errs = append(errs, fmt.Sprintf("copy %s: %v", p, err))
		}
		return nil
	})
	return copied, skipped, errs
}

func coreCopyFileExcl(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
