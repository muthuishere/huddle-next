package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/redact"
)

func init() {
	register(
		Command{Name: "log", Summary: "Append to or list the repo's event log (audit trail)", Run: runLog},
		Command{Name: "timeline", Summary: "Human timeline of recent events, grouped by day", Run: runTimeline},
	)
}

// logTypes are the event types `huddle log append` accepts (ADR 14 §1).
var logTypes = map[string]string{
	"decision":   "a choice the user made (needs --door)",
	"assumption": "something taken as true without proof",
	"question":   "an open question (needs --door)",
	"claim":      "a factual claim; --status verified|disputed|unverified",
	"lock":       "a scope or design element frozen",
	"kill":       "an idea or item dropped; --refs the killed ids",
	"learning":   "pattern|pitfall|preference learned; --key dedupes",
	"action":     "a follow-up to do; close with --refs from a later kill/supersede",
	"supersede":  "replaces earlier events; --refs the replaced ids",
	"redact":     "hides earlier events from snapshots; --refs the ids",
}

func logTypeList() string {
	names := make([]string, 0, len(logTypes))
	for k := range logTypes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

const logHelp = `
Usage:
  huddle log append --type <type> [--by <persona|user>] [--door one-way|two-way]
                    [--refs id1,id2] [--artifact kind:path] [--status s] [--key k] "<text>"
  huddle log list [--type t1,t2] [--branch b] [--limit N] [--json]

Types: %s
Text is redacted before it is stored. The log is append-only; correct an
event by appending a supersede or redact event that --refs it.
`

func runLog(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return coreHelp(c, fmt.Sprintf(logHelp, logTypeList()))
	}
	switch args[0] {
	case "append":
		return logAppend(c, args[1:])
	case "list":
		return logList(c, args[1:])
	default:
		return Usagef("unknown log action %q — run: huddle log --help", args[0])
	}
}

func logAppend(c *Ctx, args []string) error {
	a, err := parseCoreArgs("log", args,
		[]string{"type", "by", "door", "refs", "artifact", "status", "key"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, fmt.Sprintf(logHelp, logTypeList()))
	}
	typ := a.values["type"]
	if _, ok := logTypes[typ]; !ok {
		return Usagef("--type must be one of %s — run: huddle log append --type decision --door two-way \"...\"", logTypeList())
	}
	text := strings.TrimSpace(strings.Join(a.pos, " "))
	if text == "" {
		return Usagef("log append needs the event text — run: huddle log append --type %s \"<text>\"", typ)
	}
	door := a.values["door"]
	if door != "" && door != "one-way" && door != "two-way" {
		return Usagef("--door must be one-way or two-way, got %q", door)
	}
	if door == "" && (typ == "decision" || typ == "question") {
		return Usagef("a %s needs --door one-way|two-way (one-way doors are always handed to the user)", typ)
	}
	refs := coreSplitCSV(a.values["refs"])
	if len(refs) == 0 && (typ == "supersede" || typ == "redact" || typ == "kill") && a.values["artifact"] == "" {
		return Usagef("a %s event needs --refs <event ids> — find ids with: huddle log list --json", typ)
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	cfg, err := config.Load(info.Root)
	if err != nil {
		return err
	}
	by := a.values["by"]
	if by == "" {
		by = cfg.User
	}
	if by == "" {
		by = "user"
	}
	e := eventlog.Event{
		Type:   typ,
		By:     redact.String(by),
		Branch: info.Branch,
		Text:   redact.String(text),
		Door:   door,
		Refs:   refs,
	}
	data := map[string]any{}
	if s := a.values["status"]; s != "" {
		if typ == "claim" && s != "verified" && s != "disputed" && s != "unverified" {
			return Usagef("claim --status must be verified|disputed|unverified, got %q", s)
		}
		data["status"] = s
	}
	if k := a.values["key"]; k != "" {
		data["key"] = k
	}
	if len(data) > 0 {
		e.Data = data
	}
	if spec := a.values["artifact"]; spec != "" {
		ref, err := artifactRef(info.Root, spec)
		if err != nil {
			return err
		}
		e.Artifact = ref
	}
	e, err = eventlog.Append(info.Name, e)
	if err != nil {
		return fmt.Errorf("append event: %w — check permissions on %s", err, filepath.Dir(paths.Events(info.Name)))
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, e)
	}
	output.KV(c.Stdout, "LOGGED", e.ID, "TYPE", e.Type)
	return nil
}

// artifactRef parses kind:path and hashes the file if it exists.
func artifactRef(root, spec string) (*eventlog.ArtifactRef, error) {
	kind, p, ok := strings.Cut(spec, ":")
	if !ok || kind == "" || p == "" {
		return nil, Usagef("--artifact must be kind:path, e.g. spec:docs/specs/login/spec.md")
	}
	ref := &eventlog.ArtifactRef{Kind: kind, Path: filepath.ToSlash(p)}
	full := p
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, p)
	}
	if rel, err := filepath.Rel(root, full); err == nil && !strings.HasPrefix(rel, "..") {
		ref.Path = filepath.ToSlash(rel)
	}
	if f, err := os.Open(full); err == nil {
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err == nil {
			ref.SHA256 = hex.EncodeToString(h.Sum(nil))
		}
	}
	return ref, nil
}

func logFilter(a coreArgs, defLimit int) (eventlog.Filter, error) {
	limit, err := a.intValue("limit", defLimit)
	if err != nil {
		return eventlog.Filter{}, err
	}
	f := eventlog.Filter{Branch: a.values["branch"], Limit: limit}
	if t := coreSplitCSV(a.values["type"]); len(t) > 0 {
		f.Types = map[string]bool{}
		for _, x := range t {
			f.Types[x] = true
		}
	}
	return f, nil
}

func logList(c *Ctx, args []string) error {
	a, err := parseCoreArgs("log", args, []string{"type", "limit", "branch"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, fmt.Sprintf(logHelp, logTypeList()))
	}
	f, err := logFilter(a, 20)
	if err != nil {
		return err
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	events, err := eventlog.List(info.Name, f)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		if events == nil {
			events = []eventlog.Event{}
		}
		return output.JSON(c.Stdout, events)
	}
	if len(events) == 0 {
		fmt.Fprintln(c.Stdout, "No events yet — run: huddle log append --type decision --door two-way \"<text>\"")
		return nil
	}
	rows := make([][]string, 0, len(events))
	for _, e := range events {
		rows = append(rows, []string{e.TS.Local().Format("2006-01-02 15:04"), e.Type, e.By, e.ID, coreTruncate(e.Summary(), 80)})
	}
	output.Table(c.Stdout, []string{"TIME", "TYPE", "BY", "ID", "TEXT"}, rows)
	return nil
}

func runTimeline(c *Ctx, args []string) error {
	a, err := parseCoreArgs("timeline", args, []string{"limit", "type", "branch"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, "Usage: huddle timeline [--limit N] [--type t1,t2] [--branch b] [--json]\n")
	}
	f, err := logFilter(a, 30)
	if err != nil {
		return err
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	events, err := eventlog.List(info.Name, f)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		if events == nil {
			events = []eventlog.Event{}
		}
		return output.JSON(c.Stdout, events)
	}
	if len(events) == 0 {
		fmt.Fprintf(c.Stdout, "No events for %s yet — run: huddle log append --type decision --door two-way \"<text>\"\n", info.Name)
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Timeline — %s\n", info.Name)
	day := ""
	for _, e := range events {
		ts := e.TS.Local()
		if d := ts.Format("Mon 2006-01-02"); d != day {
			day = d
			fmt.Fprintf(&b, "\n%s\n", d)
		}
		extra := ""
		if e.Door == "one-way" {
			extra = " [one-way]"
		}
		if e.Branch != "" && e.Branch != info.Branch {
			extra += " (" + e.Branch + ")"
		}
		by := ""
		if e.By != "" {
			by = " " + e.By + ":"
		}
		fmt.Fprintf(&b, "  %s  %-10s%s %s%s\n", ts.Format("15:04"), e.Type, by, coreTruncate(e.Summary(), 100), extra)
	}
	_, err = io.WriteString(c.Stdout, b.String())
	return err
}
