package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/usage"
)

// TestMain puts the real agent skill roots out of reach for the whole package.
//
// Isolating HOME is not enough: CLAUDE_CONFIG_DIR and the two *_SKILLS_DIR
// variables are absolute paths read from the ambient environment, so a HOME
// temp dir cannot contain them. `huddle start` syncs the skills now, and the
// start tests here go through that path — with CLAUDE_CONFIG_DIR set to a real
// relocated config dir, running the suite replaced the developer's own
// installed skills. Neutralising the variables before any test runs makes that
// impossible; a test that wants a specific root still sets it with t.Setenv.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "huddle-test-roots")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test roots:", err)
		os.Exit(1)
	}
	for k, v := range map[string]string{
		"CLAUDE_CONFIG_DIR": filepath.Join(dir, "claude-config"),
		"CLAUDE_SKILLS_DIR": filepath.Join(dir, "claude-skills"),
		"AGENTS_SKILLS_DIR": filepath.Join(dir, "agents-skills"),
	} {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintln(os.Stderr, "test roots:", err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// coreEnv isolates HUDDLE_HOME and HOME and returns a fake git repo root.
func coreEnv(t testing.TB) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HUDDLE_HOME", filepath.Join(tmp, "huddle-home"))
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("USERPROFILE", filepath.Join(tmp, "home"))
	root := filepath.Join(tmp, "proj")
	mustWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	return root
}

func mustWrite(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCore(t testing.TB, root string, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	c := &Ctx{Stdout: &out, Stderr: &errb, Cwd: root}
	cmd, ok := registry[args[0]]
	if !ok {
		t.Fatalf("command %q not registered", args[0])
	}
	err := cmd.Run(c, args[1:])
	return out.String(), err
}

func TestParseCoreArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantPos []string
		wantVal map[string]string
		wantErr bool
	}{
		{"interleaved", []string{"hello", "--type", "decision", "world", "--json"}, []string{"hello", "world"}, map[string]string{"type": "decision"}, false},
		{"equals", []string{"--type=learning", "x"}, []string{"x"}, map[string]string{"type": "learning"}, false},
		{"missing value", []string{"--type"}, nil, nil, true},
		{"unknown", []string{"--nope"}, nil, nil, true},
		{"double dash", []string{"--", "--type"}, []string{"--type"}, map[string]string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := parseCoreArgs("log", tc.args, []string{"type"}, []string{"json"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if fmt.Sprint(a.pos) != fmt.Sprint(tc.wantPos) {
				t.Errorf("pos = %v, want %v", a.pos, tc.wantPos)
			}
			for k, v := range tc.wantVal {
				if a.values[k] != v {
					t.Errorf("%s = %q, want %q", k, a.values[k], v)
				}
			}
		})
	}
}

func TestConfigCommand(t *testing.T) {
	root := coreEnv(t)
	steps := []struct {
		args    []string
		want    string
		wantErr bool
	}{
		{[]string{"config", "set", "user", "Ada", "--scope", "global"}, "SCOPE: global", false},
		{[]string{"config", "set", "owners.tech-lead", `["bob"]`}, "SCOPE: repo", false},
		{[]string{"config", "set", "user", "Grace", "--scope", "repo"}, "SCOPE: repo", false},
		{[]string{"config", "get", "user"}, "Grace", false},
		{[]string{"config", "get", "user", "--scope", "global"}, "Ada", false},
		{[]string{"config", "get", "owners.tech-lead", "--json"}, `"bob"`, false},
		{[]string{"config", "get", "missing.key"}, "", true},
		{[]string{"config", "show", "--scope", "bogus"}, "", true},
		{[]string{"config", "show", "--json"}, `"merged"`, false},
	}
	for _, s := range steps {
		out, err := runCore(t, root, s.args...)
		if (err != nil) != s.wantErr {
			t.Fatalf("%v: err = %v", s.args, err)
		}
		if !strings.Contains(out, s.want) {
			t.Fatalf("%v: output %q missing %q", s.args, out, s.want)
		}
	}
}

func TestLogAppendListTimeline(t *testing.T) {
	root := coreEnv(t)
	mustWrite(t, filepath.Join(root, "docs", "spec.md"), "# spec\n")
	bad := [][]string{
		{"log", "append", "--type", "nope", "x"},
		{"log", "append", "--type", "decision", "no door"},
		{"log", "append", "--type", "decision", "--door", "sideways", "x"},
		{"log", "append", "--type", "supersede", "no refs"},
		{"log", "append", "--type", "learning"},
	}
	for _, args := range bad {
		if _, err := runCore(t, root, args...); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
	out, err := runCore(t, root, "log", "append", "Use Go", "--type", "decision", "--door", "one-way",
		"--by", "shaama", "--artifact", "spec:docs/spec.md", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var e eventlog.Event
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatal(err)
	}
	if e.Branch != "main" || e.Door != "one-way" || e.Artifact == nil || e.Artifact.SHA256 == "" || e.Artifact.Path != "docs/spec.md" {
		t.Fatalf("unexpected event %+v", e)
	}
	if _, err := runCore(t, root, "log", "append", "--type", "learning", "--key", "k1", "tests first"); err != nil {
		t.Fatal(err)
	}
	out, err = runCore(t, root, "log", "list", "--type", "learning", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var list []eventlog.Event
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 1 || list[0].Text != "tests first" {
		t.Fatalf("list = %v (%v)", list, err)
	}
	out, err = runCore(t, root, "timeline")
	if err != nil || !strings.Contains(out, "Use Go") || !strings.Contains(out, "[one-way]") {
		t.Fatalf("timeline = %q (%v)", out, err)
	}
}

func TestSnapshotEvents(t *testing.T) {
	ts := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ev := func(i int, typ string, refs []string, data map[string]any) eventlog.Event {
		return eventlog.Event{ID: fmt.Sprintf("e%02d", i), TS: ts.Add(time.Duration(i) * time.Minute), Type: typ, Text: typ, Refs: refs, Data: data}
	}
	events := []eventlog.Event{
		ev(1, "decision", nil, nil),
		ev(2, "decision", nil, nil),
		ev(3, "supersede", []string{"e01"}, nil),
		ev(4, "question", nil, nil),
		ev(5, "question", nil, nil),
		ev(6, "decision", []string{"e04"}, nil), // answers e04
		ev(7, "learning", nil, map[string]any{"key": "a"}),
		ev(8, "learning", nil, map[string]any{"key": "a"}),
		ev(9, "learning", nil, nil),
		ev(10, "gate.request", nil, map[string]any{"gate": "spec"}),
		ev(11, "gate.approved", nil, map[string]any{"gate": "spec"}),
		ev(12, "gate.request", nil, map[string]any{"gate": "plan"}),
		ev(13, "gate.request", nil, map[string]any{"gate": "spec"}), // re-requested after approval
		ev(14, "action", nil, nil),
		ev(15, "kill", []string{"e14"}, nil),
	}
	s := snapshotEvents(events)
	if s.decisionsN != 2 || s.decisions[0].ID != "e06" {
		t.Errorf("decisions = %d %+v", s.decisionsN, s.decisions)
	}
	if s.questionsN != 1 || s.questions[0].ID != "e05" {
		t.Errorf("questions = %+v", s.questions)
	}
	if s.actionsN != 0 {
		t.Errorf("actions = %+v", s.actions)
	}
	if len(s.learnings) != 2 || s.learnings[0].ID != "e09" || s.learnings[1].ID != "e08" {
		t.Errorf("learnings = %+v", s.learnings)
	}
	var gates []string
	for _, g := range s.gates {
		gates = append(gates, g.Gate)
	}
	sort.Strings(gates)
	if fmt.Sprint(gates) != "[plan spec]" {
		t.Errorf("gates = %v", gates)
	}
}

func TestStartOutputBudget(t *testing.T) {
	root := coreEnv(t)
	long := strings.Repeat("a very long decision text that keeps going ", 20)
	for i := 0; i < 40; i++ {
		for _, typ := range []string{"decision", "question", "action", "learning"} {
			if _, err := eventlog.Append("proj", eventlog.Event{Type: typ, Text: long, Door: "one-way"}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := eventlog.Append("proj", eventlog.Event{Type: "gate.request", Data: map[string]any{"gate": fmt.Sprintf("g%d", i), "artifact": long}}); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, paths.RepoConfig(root), `{"owners":{"product":["a"],"tech-lead":["b"]},"hooks":[{"on":"gate.request","run":"true"}]}`)
	out, err := runCore(t, root, "start")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > startBudgetBytes {
		t.Fatalf("start output %d bytes > %d:\n%s", len(out), startBudgetBytes, out)
	}
	for _, want := range []string{"HUDDLE_PROTO: 1\n", "REPO: proj @ main", "OWNERS: product,tech-lead", "HOOKS: 1",
		"REPO_HOOKS_CHANGED: yes (1 hooks)", "DECISIONS_ACTIVE: 40", "GATES_PENDING: 40", "SUBAGENTS: unknown", "NEXT: huddle hooks list"} {
		if !strings.Contains(out, want) {
			t.Errorf("start output missing %q:\n%s", want, out)
		}
	}
	// Once hooks are marked seen, the flag clears; start itself never writes it.
	if _, err := os.Stat(paths.HooksSeen("proj")); err == nil {
		t.Fatal("start must not write hooks-seen.json")
	}
	info, _ := (&Ctx{Cwd: root}).Repo()
	if err := writeHooksSeen(info); err != nil {
		t.Fatal(err)
	}
	out, _ = runCore(t, root, "start", "--json")
	var res StartResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	// --json is held to the same ceiling as the text output (ADR 13, 19): with
	// 40 oversized items per list the item lines go, the counts stay.
	if len(out) > startBudgetBytes {
		t.Fatalf("start --json %d bytes > %d:\n%s", len(out), startBudgetBytes, out)
	}
	if res.RepoHooksChanged || res.Proto != 1 || res.Partial {
		t.Fatalf("unexpected start json: changed=%v proto=%d partial=%v %v",
			res.RepoHooksChanged, res.Proto, res.Partial, res.Errors)
	}
	if res.DecisionsActive != 40 || res.GatesPending != 40 || res.LearningsTotal != startLearnings || res.ItemsElided == 0 {
		t.Fatalf("truncation lost the counts: decisions=%d gates=%d learnings=%d elided=%d",
			res.DecisionsActive, res.GatesPending, res.LearningsTotal, res.ItemsElided)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	root := coreEnv(t)
	_ = root
	legacy := filepath.Join(os.Getenv("HOME"), ".config", "muthuishere-agent-skills")
	mustWrite(t, filepath.Join(legacy, "userconfig.json"), `{"git_user":"Muthu","python_bin":"/x"}`)
	mustWrite(t, filepath.Join(legacy, "personas", "vidya", "vidya.md"), "# vidya")
	mustWrite(t, filepath.Join(legacy, "myrepo", "personas", "suren", "memories", "m.md"), "memory")
	mustWrite(t, filepath.Join(legacy, "myrepo", "feature-x", "huddle", "raw", "1757404800000_decision.json"),
		`{"type":"decision","ts":1757404800000,"topic":"Keep codebase","content":"details","by":"Muthu"}`)
	mustWrite(t, filepath.Join(legacy, "myrepo", "main", "huddle", "raw", "1757405400000_milestone.json"),
		`{"kind":"milestone","ts":1757405400000,"content":"shipped"}`)
	// An existing global persona file must never be overwritten.
	mustWrite(t, filepath.Join(paths.GlobalPersonas(), "vidya", "vidya.md"), "# mine")

	if !legacyMigrationNeeded() {
		t.Fatal("migration should be needed")
	}
	dry, err := runCore(t, root, "migrate", "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Migrated()); err == nil {
		t.Fatal("dry-run wrote migrated.json")
	}
	if !strings.Contains(dry, `"myrepo": 2`) {
		t.Fatalf("dry-run report: %s", dry)
	}
	if _, err := runCore(t, root, "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCore(t, root, "migrate"); err != nil { // second run: no-op
		t.Fatal(err)
	}
	events, err := eventlog.List("myrepo", eventlog.Filter{})
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %d (%v)", len(events), err)
	}
	if events[0].Type != "decision" || events[0].Text != "Keep codebase" || events[0].Branch != "feature-x" || events[1].Type != "milestone" {
		t.Fatalf("events = %+v", events)
	}
	if b, _ := os.ReadFile(filepath.Join(paths.GlobalPersonas(), "vidya", "vidya.md")); string(b) != "# mine" {
		t.Fatalf("existing persona overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(paths.GlobalPersonas(), "suren", "memories", "m.md")); err != nil {
		t.Fatal("repo persona not copied")
	}
	cfg, _ := config.Load(root)
	if cfg.User != "Muthu" {
		t.Fatalf("user = %q", cfg.User)
	}
	if _, err := os.Stat(filepath.Join(legacy, "myrepo", "main", "huddle", "raw", "1757405400000_milestone.json")); err != nil {
		t.Fatal("legacy file was moved")
	}
	if legacyMigrationNeeded() {
		t.Fatal("migration still flagged after migrate")
	}
}

// startFixture builds a repo with 1,000 events and 50 persona dirs.
func startFixture(b testing.TB) string {
	root := coreEnv(b)
	types := []string{"decision", "question", "action", "learning", "claim", "gate.request"}
	for i := 0; i < 1000; i++ {
		e := eventlog.Event{Type: types[i%len(types)], By: "bench", Branch: "main", Door: "two-way",
			Text: fmt.Sprintf("event %d with a realistic sentence about the design and its tradeoffs", i),
			Data: map[string]any{"gate": fmt.Sprintf("g%d", i%7), "key": fmt.Sprintf("k%d", i%11)}}
		if _, err := eventlog.Append("proj", e); err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < 50; i++ {
		dir := paths.GlobalPersonas()
		if i%2 == 1 {
			dir = paths.RepoPersonas(root)
		}
		mustWrite(b, filepath.Join(dir, fmt.Sprintf("p%02d", i), fmt.Sprintf("p%02d.md", i)), "# persona")
	}
	mustWrite(b, paths.RepoConfig(root), `{"owners":{"tech-lead":["a"]},"hooks":[{"on":"session.start","run":"true"}]}`)
	return root
}

func BenchmarkStart(b *testing.B) {
	root := startFixture(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := runCore(b, root, "start"); err != nil {
			b.Fatal(err)
		}
	}
}

// Events the CLI appends itself have no `text` field — `log list` and
// `timeline` used to print an empty TEXT column for them, so the file that was
// written could only be found by reading events.jsonl by hand.
func TestLogListAndTimelineRenderStructuredEvents(t *testing.T) {
	root := coreEnv(t)
	if _, err := eventlog.Append("proj", eventlog.Event{
		Type: "artifact.written", By: "muthu", Branch: "main", Refs: []string{"intent:v1-readiness"},
		Artifact: &eventlog.ArtifactRef{Kind: "intent", Path: "intent/v1-readiness/intent.md", SHA256: "deadbeef"},
		Data:     map[string]any{"id": "intent:v1-readiness", "action": "new"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eventlog.Append("proj", eventlog.Event{
		Type: "gate.request", By: "muthu", Branch: "main", Refs: []string{"spec:login"},
		Artifact: &eventlog.ArtifactRef{Kind: "spec", Path: "intent/login/spec.md"},
		Data:     map[string]any{"gate": "g7", "name": "spec", "role": "tech-lead", "artifactId": "spec:login"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"log", "timeline"} {
		args := []string{cmd}
		if cmd == "log" {
			args = append(args, "list")
		}
		out, err := runCore(t, root, args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"intent/v1-readiness/intent.md", "action=new", "intent/login/spec.md", "gate=g7"} {
			if !strings.Contains(out, want) {
				t.Errorf("huddle %s output missing %q:\n%s", strings.Join(args, " "), want, out)
			}
		}
		if strings.Contains(out, "deadbeef") {
			t.Errorf("huddle %s should not print the artifact hash:\n%s", strings.Join(args, " "), out)
		}
	}
}

// Every command `huddle stats` can show needs a p50 budget, or the BUDGET
// column cannot say whether the command is fast or slow. Two commands are
// deliberately exempt and documented in usage.Budgets.
func TestEveryCommandHasALatencyBudget(t *testing.T) {
	exempt := map[string]bool{"serve": true, "context": true}
	for name := range registry {
		if exempt[name] {
			if usage.Budgets[name] != 0 {
				t.Errorf("%q is documented as having no latency contract but has a budget", name)
			}
			continue
		}
		if usage.Budgets[name] <= 0 {
			t.Errorf("command %q has no p50 budget — huddle stats shows BUDGET: - for it", name)
		}
	}
	// Budgets for things that are not commands would never be shown.
	for name := range usage.Budgets {
		if _, ok := registry[name]; !ok && name != "help" && name != "version" {
			t.Errorf("budget for unknown command %q", name)
		}
	}
}

// startRealisticHistory writes an ordinary repo history: a handful of decisions,
// questions, actions, learnings and one pending gate, with ~150-character text.
func startRealisticHistory(t testing.TB, repoName string) {
	t.Helper()
	text := "we will keep the event log append-only and fix the renderer instead, " +
		"because the audit trail must stay byte-for-byte what happened at the time"
	if len(text) < 140 || len(text) > 170 {
		t.Fatalf("fixture text is %d chars, wanted ~150", len(text))
	}
	counts := map[string]int{"decision": 9, "question": 3, "action": 3, "learning": 3}
	for typ, n := range counts {
		for i := 0; i < n; i++ {
			e := eventlog.Event{Type: typ, By: "muthu", Branch: "main", Text: text, Door: "two-way"}
			if typ == "learning" {
				e.Data = map[string]any{"key": fmt.Sprintf("k%d", i)}
			}
			if _, err := eventlog.Append(repoName, e); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := eventlog.Append(repoName, eventlog.Event{
		Type: "gate.request", By: "muthu", Branch: "main",
		Artifact: &eventlog.ArtifactRef{Kind: "spec", Path: "intent/observability-and-budgets/spec.md"},
		Data:     map[string]any{"gate": "spec", "role": "tech-lead"},
	}); err != nil {
		t.Fatal(err)
	}
}

// `huddle start --json` is a machine protocol parsed on every session start, so
// it is held to the same 2 KB ceiling as the text output (ADR 13, 19). An
// ordinary history — 9 decisions with ~150-character text — used to produce
// 3,151 bytes.
func TestStartJSONOutputBudget(t *testing.T) {
	root := coreEnv(t)
	startRealisticHistory(t, "proj")
	out, err := runCore(t, root, "start", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > startBudgetBytes {
		t.Fatalf("start --json %d bytes > %d:\n%s", len(out), startBudgetBytes, out)
	}
	var res StartResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("start --json is not valid JSON: %v\n%s", err, out)
	}
	if res.Proto != 1 || res.Version == "" || res.Repo.Name != "proj" || res.Next == "" {
		t.Fatalf("field contract broken: %+v", res)
	}
	if res.DecisionsActive != 9 || res.QuestionsOpen != 3 || res.ActionsOpen != 3 || res.GatesPending != 1 {
		t.Fatalf("counts must survive truncation: %d decisions, %d questions, %d actions, %d gates",
			res.DecisionsActive, res.QuestionsOpen, res.ActionsOpen, res.GatesPending)
	}
	if len(res.Decisions) == 0 {
		t.Fatal("no decision survived: a realistic history should still list some")
	}
	// itemsElided counts the lines dropped from the lists start would otherwise
	// have shown (the newest startListLimit per list, startLearnings learnings).
	want := 0
	for _, l := range []struct{ shown, total, cap int }{
		{len(res.Decisions), res.DecisionsActive, startListLimit},
		{len(res.Questions), res.QuestionsOpen, startListLimit},
		{len(res.Actions), res.ActionsOpen, startListLimit},
		{len(res.Learnings), res.LearningsTotal, startLearnings},
		{len(res.PendingGates), res.GatesPending, res.GatesPending},
	} {
		listed := l.total
		if listed > l.cap {
			listed = l.cap
		}
		want += listed - l.shown
	}
	if res.ItemsElided != want || want == 0 {
		t.Fatalf("itemsElided = %d, want %d", res.ItemsElided, want)
	}
	t.Logf("start --json = %d bytes, %d item lines elided", len(out), res.ItemsElided)
}

// TestStartDispatchP50Budget guards the in-process half of start: everything
// app.Run does, which is what usage.Entry.MS records and what `huddle stats`
// compares against usage.Budgets. It is NOT the 60 ms ADR 13/19 budget — that
// one is user-felt and lives in TestStartEndToEndP50Budget below.
func TestStartDispatchP50Budget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped under -short")
	}
	root := startFixture(t)
	const runs = 50
	durs := make([]time.Duration, runs)
	for i := range durs {
		t0 := time.Now()
		if _, err := runCore(t, root, "start"); err != nil {
			t.Fatal(err)
		}
		durs[i] = time.Since(t0)
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50, p95 := durs[runs/2], durs[runs*95/100]
	budget := time.Duration(usage.Budgets["start"]) * time.Millisecond
	t.Logf("start dispatch p50=%v p95=%v (budget %v)", p50, p95, budget)
	if p50 >= budget {
		t.Fatalf("start dispatch p50 %v >= %v budget (usage.Budgets[\"start\"])", p50, budget)
	}
}

// TestStartEndToEndP50Budget is the honest reading of the 60 ms budget in ADR
// 13/19: the wall-clock time a user or agent waits, measured on the real
// binary, process start included. The in-process measurement it used to be
// checked against (~0.2 ms against 60 ms) could not fail.
func TestStartEndToEndP50Budget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped under -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain: cannot build the binary to time it")
	}
	root := startFixture(t)
	bin := filepath.Join(t.TempDir(), "huddle")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command(goBin, "build", "-o", bin, "./cmd/huddle")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build huddle: %v\n%s", err, out)
	}
	const runs = 30
	durs := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		cmd := exec.Command(bin, "start")
		cmd.Dir = root
		cmd.Env = os.Environ() // coreEnv + TestMain already point every root at a temp dir
		t0 := time.Now()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("huddle start: %v\n%s", err, out)
		}
		durs = append(durs, time.Since(t0))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50, p95 := durs[runs/2], durs[runs*95/100]
	budget := time.Duration(usage.StartEndToEndBudgetMS) * time.Millisecond
	t.Logf("start end-to-end p50=%v p95=%v (budget %v)", p50, p95, budget)
	if p50 >= budget {
		t.Fatalf("start end-to-end p50 %v >= %v budget (ADR 13/19)", p50, budget)
	}
}
