package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/paths"
)

// coreEnv isolates HUDDLE_HOME and HOME and returns a fake git repo root.
func coreEnv(t testing.TB) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HUDDLE_HOME", filepath.Join(tmp, "huddle-home"))
	t.Setenv("HOME", filepath.Join(tmp, "home"))
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
	if res.RepoHooksChanged || res.Proto != 1 || len(res.Decisions) != 5 || len(res.Learnings) != 3 || res.Partial {
		t.Fatalf("unexpected start json: changed=%v proto=%d decisions=%d learnings=%d partial=%v %v",
			res.RepoHooksChanged, res.Proto, len(res.Decisions), len(res.Learnings), res.Partial, res.Errors)
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

func TestStartP50Budget(t *testing.T) {
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
	t.Logf("start p50=%v p95=%v", p50, p95)
	if p50 >= 60*time.Millisecond {
		t.Fatalf("start p50 %v >= 60ms budget", p50)
	}
}
