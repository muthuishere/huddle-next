package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle/assets"
	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/output"
	"github.com/muthuishere/huddle/internal/paths"
	"github.com/muthuishere/huddle/internal/repo"
	"github.com/muthuishere/huddle/internal/version"
)

func init() {
	register(Command{
		Name:    "start",
		Summary: "The one startup call: repo, config, decisions, gates, personas (no network)",
		Run:     runStart,
	})
}

const (
	startProbeDeadline = 300 * time.Millisecond
	startBudgetBytes   = 2048
	startListLimit     = 5
	startLearnings     = 3
)

// StartItem is one event surfaced by `huddle start`.
type StartItem struct {
	ID   string    `json:"id"`
	TS   time.Time `json:"ts"`
	By   string    `json:"by,omitempty"`
	Text string    `json:"text"`
	Door string    `json:"door,omitempty"`
	Key  string    `json:"key,omitempty"`
}

// StartGate is a gate request with no approval or rejection yet.
type StartGate struct {
	Gate      string    `json:"gate"`
	RequestID string    `json:"requestId"`
	TS        time.Time `json:"ts"`
	By        string    `json:"by,omitempty"`
	Artifact  string    `json:"artifact,omitempty"`
}

// StartConfig summarises the merged config without dumping it.
type StartConfig struct {
	User       string   `json:"user,omitempty"`
	OwnerRoles []string `json:"ownerRoles"`
	Hooks      int      `json:"hooks"`
	RepoHooks  int      `json:"repoHooks"`
}

// StartPersonas counts persona entries per layer (directory entries only).
type StartPersonas struct {
	Builtin int `json:"builtin"`
	Global  int `json:"global"`
	Repo    int `json:"repo"`
	Total   int `json:"total"` // unique ids across layers
}

// StartResult is the full `huddle start --json` payload.
type StartResult struct {
	Proto            int           `json:"HUDDLE_PROTO"`
	Version          string        `json:"version"`
	Repo             repo.Info     `json:"repo"`
	Config           StartConfig   `json:"config"`
	RepoHooksChanged bool          `json:"repoHooksChanged"`
	Events           int           `json:"events"`
	DecisionsActive  int           `json:"decisionsActive"`
	Decisions        []StartItem   `json:"decisions"`
	QuestionsOpen    int           `json:"questionsOpen"`
	Questions        []StartItem   `json:"questions"`
	ActionsOpen      int           `json:"actionsOpen"`
	Actions          []StartItem   `json:"actions"`
	Learnings        []StartItem   `json:"learnings"`
	PendingGates     []StartGate   `json:"pendingGates"`
	Personas         StartPersonas `json:"personas"`
	Drafts           int           `json:"drafts"`
	MigrationNeeded  bool          `json:"migrationNeeded"`
	Subagents        string        `json:"subagents"`
	Next             string        `json:"next"`
	Partial          bool          `json:"partial"`
	Slow             []string      `json:"slowProbes,omitempty"`
	Errors           []string      `json:"errors,omitempty"`
	ElapsedMS        float64       `json:"elapsedMs"`
	repoHooksSeen    int
}

func runStart(c *Ctx, args []string) error {
	a, err := parseCoreArgs("start", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, "Usage: huddle start [--json]\n\nOne call, no network: protocol, repo, config summary, active decisions,\nopen questions/actions, learnings, pending gates, persona and draft counts.\n")
	}
	res, err := buildStart(c)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, res)
	}
	_, err = c.Stdout.Write(renderStart(res))
	return err
}

type startProbe struct {
	name string
	run  func() (func(*StartResult), error)
}

// buildStart runs every probe concurrently with one shared deadline. A probe
// that misses it is reported in Slow and the result is marked partial.
func buildStart(c *Ctx) (StartResult, error) {
	t0 := time.Now()
	info, err := c.Repo()
	if err != nil {
		return StartResult{}, fmt.Errorf("identify repo in %s: %w — run huddle from inside a project folder", c.Cwd, err)
	}
	res := StartResult{
		Proto:     version.Proto,
		Version:   version.Version,
		Repo:      info,
		Subagents: "unknown",
	}
	probes := []startProbe{
		{"config", func() (func(*StartResult), error) { return probeConfig(info.Root) }},
		{"hooks-changed", func() (func(*StartResult), error) { return probeHooksChanged(info) }},
		{"events", func() (func(*StartResult), error) { return probeEvents(info.Name) }},
		{"personas", func() (func(*StartResult), error) { return probePersonas(info.Root) }},
		{"drafts", func() (func(*StartResult), error) { return probeDrafts(info.Name) }},
		{"migration", func() (func(*StartResult), error) {
			need := legacyMigrationNeeded()
			return func(r *StartResult) { r.MigrationNeeded = need }, nil
		}},
	}
	type done struct {
		name  string
		apply func(*StartResult)
		err   error
	}
	ch := make(chan done, len(probes)) // buffered: late probes never block
	for _, p := range probes {
		go func(p startProbe) {
			apply, err := p.run()
			ch <- done{p.name, apply, err}
		}(p)
	}
	pending := map[string]bool{}
	for _, p := range probes {
		pending[p.name] = true
	}
	timer := time.NewTimer(startProbeDeadline)
	defer timer.Stop()
collect:
	for len(pending) > 0 {
		select {
		case d := <-ch:
			delete(pending, d.name)
			if d.err != nil {
				res.Errors = append(res.Errors, d.name+": "+d.err.Error())
				res.Partial = true
			}
			if d.apply != nil {
				d.apply(&res)
			}
		case <-timer.C:
			break collect
		}
	}
	for name := range pending {
		res.Slow = append(res.Slow, name)
		res.Partial = true
	}
	sort.Strings(res.Slow)
	res.Next = startNext(res)
	res.ElapsedMS = float64(time.Since(t0).Microseconds()) / 1000
	return res, nil
}

func probeConfig(root string) (func(*StartResult), error) {
	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}
	sc := StartConfig{User: cfg.User, OwnerRoles: coreSortedKeys(cfg.Owners), Hooks: len(cfg.Hooks)}
	for _, h := range cfg.Hooks {
		if h.Scope == string(config.Repo) {
			sc.RepoHooks++
		}
	}
	return func(r *StartResult) { r.Config = sc }, nil
}

// hooksSeen is paths.HooksSeen(repo). `huddle hooks list` records the hash
// with writeHooksSeen; `huddle start` only compares.
type hooksSeen struct {
	SchemaVersion int       `json:"schemaVersion"`
	SHA256        string    `json:"sha256"`
	Count         int       `json:"count"`
	SeenAt        time.Time `json:"seenAt"`
}

// repoHooksHash returns the sha256 of the canonical JSON of the repo config's
// hooks array (map keys sorted by encoding/json) and the hook count. A missing
// file or key hashes as [].
func repoHooksHash(root string) (string, int, error) {
	var hooks []any
	b, err := os.ReadFile(paths.RepoConfig(root))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 0, err
	}
	if len(bytes.TrimSpace(b)) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return "", 0, fmt.Errorf("%s is not valid JSON: %w — fix it, then run: huddle config show", paths.RepoConfig(root), err)
		}
		if raw, ok := m["hooks"]; ok {
			if err := json.Unmarshal(raw, &hooks); err != nil {
				return "", 0, fmt.Errorf("%s: hooks must be an array — run: huddle hooks list", paths.RepoConfig(root))
			}
		}
	}
	if hooks == nil {
		hooks = []any{}
	}
	canon, err := json.Marshal(hooks)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), len(hooks), nil
}

// writeHooksSeen marks the current repo hooks as reviewed. Called by `huddle hooks list`.
func writeHooksSeen(info repo.Info) error {
	h, n, err := repoHooksHash(info.Root)
	if err != nil {
		return err
	}
	return config.WriteJSON(paths.HooksSeen(info.Name), hooksSeen{SchemaVersion: 1, SHA256: h, Count: n, SeenAt: time.Now().UTC()})
}

func probeHooksChanged(info repo.Info) (func(*StartResult), error) {
	h, n, err := repoHooksHash(info.Root)
	if err != nil {
		return nil, err
	}
	var seen hooksSeen
	b, err := os.ReadFile(paths.HooksSeen(info.Name))
	haveSeen := err == nil && json.Unmarshal(b, &seen) == nil && seen.SHA256 != ""
	changed := false
	if haveSeen {
		changed = seen.SHA256 != h
	} else {
		changed = n > 0
	}
	return func(r *StartResult) { r.RepoHooksChanged, r.repoHooksSeen = changed, n }, nil
}

func probeEvents(repoName string) (func(*StartResult), error) {
	events, err := eventlog.List(repoName, eventlog.Filter{})
	if err != nil {
		return nil, err
	}
	snap := snapshotEvents(events)
	return func(r *StartResult) {
		r.Events = len(events)
		r.DecisionsActive, r.Decisions = snap.decisionsN, snap.decisions
		r.QuestionsOpen, r.Questions = snap.questionsN, snap.questions
		r.ActionsOpen, r.Actions = snap.actionsN, snap.actions
		r.Learnings = snap.learnings
		r.PendingGates = snap.gates
	}, nil
}

type eventSnapshot struct {
	decisionsN, questionsN, actionsN int
	decisions, questions, actions    []StartItem
	learnings                        []StartItem
	gates                            []StartGate
}

func dataString(e eventlog.Event, key string) string {
	s, _ := e.Data[key].(string)
	return s
}

// snapshotEvents computes the active view from an oldest-first log:
// supersede/kill/redact close what they reference; a decision answers the
// questions it references; the latest learning per key wins; a gate request
// is pending until a gate.approved or gate.rejected names its gate (or refs it).
func snapshotEvents(events []eventlog.Event) eventSnapshot {
	closed := map[string]bool{}
	answered := map[string]bool{}
	resolvedGates := map[string]int{} // gate or "id:<request>" -> index of last resolution
	for i, e := range events {
		switch e.Type {
		case "supersede", "kill", "redact":
			for _, id := range e.Refs {
				closed[id] = true
			}
		case "decision":
			for _, id := range e.Refs {
				answered[id] = true
			}
		case "gate.approved", "gate.rejected":
			if g := dataString(e, "gate"); g != "" {
				resolvedGates[g] = i + 1
			}
			for _, id := range e.Refs {
				resolvedGates["id:"+id] = i + 1
			}
		}
	}
	var s eventSnapshot
	var decisions, questions, actions []StartItem
	learningByKey := map[string]int{}
	var learnings []StartItem
	var gates []StartGate
	for i, e := range events {
		if closed[e.ID] {
			continue
		}
		item := StartItem{ID: e.ID, TS: e.TS, By: e.By, Text: e.Text, Door: e.Door}
		switch e.Type {
		case "decision":
			decisions = append(decisions, item)
		case "question":
			if !answered[e.ID] {
				questions = append(questions, item)
			}
		case "action":
			actions = append(actions, item)
		case "learning":
			item.Key = dataString(e, "key")
			if item.Key != "" {
				if i, ok := learningByKey[item.Key]; ok {
					learnings[i] = item
					continue
				}
				learningByKey[item.Key] = len(learnings)
			}
			learnings = append(learnings, item)
		case "gate.request":
			g := dataString(e, "gate")
			if (g != "" && resolvedGates[g] > i) || resolvedGates["id:"+e.ID] > 0 {
				continue
			}
			if g == "" {
				g = e.ID
			}
			sg := StartGate{Gate: g, RequestID: e.ID, TS: e.TS, By: e.By}
			if e.Artifact != nil {
				sg.Artifact = e.Artifact.Kind + ":" + e.Artifact.Path
			} else {
				sg.Artifact = dataString(e, "artifact")
			}
			gates = append(gates, sg)
		}
	}
	// A later request for the same gate replaces an earlier one.
	seenGate := map[string]bool{}
	var dedup []StartGate
	for i := len(gates) - 1; i >= 0; i-- {
		if !seenGate[gates[i].Gate] {
			seenGate[gates[i].Gate] = true
			dedup = append(dedup, gates[i])
		}
	}
	sort.SliceStable(learnings, func(i, j int) bool { return learnings[i].TS.After(learnings[j].TS) })
	s.decisionsN, s.decisions = len(decisions), newestFirst(decisions, startListLimit)
	s.questionsN, s.questions = len(questions), newestFirst(questions, startListLimit)
	s.actionsN, s.actions = len(actions), newestFirst(actions, startListLimit)
	if len(learnings) > startLearnings {
		learnings = learnings[:startLearnings]
	}
	s.learnings = nonNilItems(learnings)
	if dedup == nil {
		dedup = []StartGate{}
	}
	s.gates = dedup
	return s
}

func newestFirst(items []StartItem, n int) []StartItem {
	out := []StartItem{}
	for i := len(items) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, items[i])
	}
	return out
}

func nonNilItems(items []StartItem) []StartItem {
	if items == nil {
		return []StartItem{}
	}
	return items
}

// personaIDs lists non-hidden entries of a persona layer; *.md files and
// directories both count, keyed by id.
func personaIDs(entries []fs.DirEntry) []string {
	var ids []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		if !e.IsDir() {
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			name = strings.TrimSuffix(name, ".md")
		}
		ids = append(ids, name)
	}
	return ids
}

func probePersonas(root string) (func(*StartResult), error) {
	var p StartPersonas
	unique := map[string]bool{}
	if entries, err := fs.ReadDir(assets.Personas, "personas"); err == nil {
		ids := personaIDs(entries)
		p.Builtin = len(ids)
		for _, id := range ids {
			unique[id] = true
		}
	}
	for _, layer := range []struct {
		dir string
		n   *int
	}{{paths.GlobalPersonas(), &p.Global}, {paths.RepoPersonas(root), &p.Repo}} {
		entries, err := os.ReadDir(layer.dir)
		if err != nil {
			continue
		}
		ids := personaIDs(entries)
		*layer.n = len(ids)
		for _, id := range ids {
			unique[id] = true
		}
	}
	p.Total = len(unique)
	return func(r *StartResult) { r.Personas = p }, nil
}

func probeDrafts(repoName string) (func(*StartResult), error) {
	entries, err := os.ReadDir(paths.Drafts(repoName))
	if err != nil {
		return func(r *StartResult) {}, nil
	}
	n := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return func(r *StartResult) { r.Drafts = n }, nil
}

func startNext(r StartResult) string {
	switch {
	case r.MigrationNeeded:
		return "huddle migrate --dry-run"
	case r.RepoHooksChanged:
		return "huddle hooks list"
	case len(r.PendingGates) > 0:
		return "huddle gate status"
	case r.Events == 0:
		return "discuss; record choices with: huddle log append --type decision --door two-way \"...\""
	default:
		return "discuss"
	}
}

// renderStart prints compact KEY: value lines and keeps them within
// startBudgetBytes by shortening item text, then dropping item lines.
func renderStart(r StartResult) []byte {
	for _, width := range []int{100, 72, 48, 32} {
		for _, maxItems := range []int{startListLimit, 3, 2, 1, 0} {
			b := renderStartWith(r, width, maxItems)
			if len(b) <= startBudgetBytes {
				return b
			}
		}
	}
	b := renderStartWith(r, 32, 0)
	if len(b) > startBudgetBytes {
		b = append(b[:startBudgetBytes-4:startBudgetBytes-4], "…\n"...)
	}
	return b
}

func renderStartWith(r StartResult, width, maxItems int) []byte {
	var b bytes.Buffer
	kv := func(k, v string) { fmt.Fprintf(&b, "%s: %s\n", k, v) }
	yesNo := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	kv("HUDDLE_PROTO", fmt.Sprint(r.Proto))
	kv("VERSION", r.Version)
	repoLine := r.Repo.Name + " @ " + r.Repo.Branch
	if !r.Repo.IsGit {
		repoLine += " (local folder, no git)"
	}
	kv("REPO", repoLine)
	kv("ROOT", r.Repo.Root)
	if r.Config.User != "" {
		kv("USER", r.Config.User)
	}
	owners := "none — set with: huddle config set owners.tech-lead '[\"name\"]'"
	if len(r.Config.OwnerRoles) > 0 {
		owners = strings.Join(r.Config.OwnerRoles, ",")
	}
	kv("OWNERS", owners)
	kv("HOOKS", fmt.Sprint(r.Config.Hooks))
	if r.RepoHooksChanged {
		kv("REPO_HOOKS_CHANGED", fmt.Sprintf("yes (%d hooks) — review: huddle hooks list", r.repoHooksSeen))
	}
	kv("PERSONAS", fmt.Sprintf("%d (builtin %d, global %d, repo %d)", r.Personas.Total, r.Personas.Builtin, r.Personas.Global, r.Personas.Repo))
	kv("DRAFTS", fmt.Sprint(r.Drafts))
	kv("EVENTS", fmt.Sprint(r.Events))
	items := func(label string, total int, list []StartItem) {
		kv(label, fmt.Sprint(total))
		for i, it := range list {
			if i >= maxItems {
				break
			}
			door := ""
			if it.Door == "one-way" {
				door = " [one-way]"
			}
			kv("- "+it.ID, coreTruncate(it.Text, width)+door)
		}
	}
	items("DECISIONS_ACTIVE", r.DecisionsActive, r.Decisions)
	items("QUESTIONS_OPEN", r.QuestionsOpen, r.Questions)
	items("ACTIONS_OPEN", r.ActionsOpen, r.Actions)
	items("LEARNINGS", len(r.Learnings), r.Learnings)
	kv("GATES_PENDING", fmt.Sprint(len(r.PendingGates)))
	for i, g := range r.PendingGates {
		if i >= maxItems {
			break
		}
		kv("- "+g.Gate, coreTruncate(g.Artifact, width))
	}
	kv("MIGRATION_NEEDED", yesNo(r.MigrationNeeded))
	kv("SUBAGENTS", r.Subagents)
	if r.Partial {
		detail := strings.Join(append(append([]string{}, r.Slow...), r.Errors...), "; ")
		kv("PARTIAL", "yes — "+coreTruncate(detail, 160))
	}
	kv("NEXT", r.Next)
	return b.Bytes()
}
