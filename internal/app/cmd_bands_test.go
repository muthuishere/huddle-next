package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/eventlog"
)

const bandsTestYAML = `metric: checkout_p95_ms
baseline:
  window: 5
rules:
  direction: up
  min_points: 3
tiers:
  log: 1
  diagnose: 2
  breach: 3
`

func TestBandsCheckTiersBreachAndDismiss(t *testing.T) {
	root := artTestEnv(t)
	artWrite(t, filepath.Join(root, "bands.yaml"), bandsTestYAML)
	// baseline 100,102,98,101,99 → mean 100, σ ≈ 1.58
	series := func(name, last string) string {
		p := filepath.Join(root, name)
		artWrite(t, p, "ts,value\n1,100\n2,102\n3,98\n4,101\n5,99\n6,"+last+"\n")
		return p
	}
	for last, tier := range map[string]string{"100.5": "OK", "102": "LOG", "104": "DIAGNOSE", "90": "OK"} {
		out := artMust(t, root, "bands", "check", "--series", series("s.csv", last))
		if !strings.Contains(out, "TIER: "+tier) {
			t.Errorf("latest %s: want %s\n%s", last, tier, out)
		}
	}
	out := artMust(t, root, "bands", "check", "--series", series("b.csv", "120"), "--date", "2026-09-14", "--json")
	var res struct {
		Result struct{ Tier string }
		Event  string
		Intent struct {
			Path   string
			DryRun bool
		}
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	wantPath := "intent/checkout-p95-ms-breach-2026-09-14/intent.md"
	if res.Result.Tier != "breach" || res.Intent.Path != wantPath || !res.Intent.DryRun || res.Event == "" {
		t.Fatalf("breach: %+v\n%s", res, out)
	}
	if _, err := os.Stat(filepath.Join(root, wantPath)); !os.IsNotExist(err) {
		t.Fatal("dry run wrote the intent")
	}
	artMust(t, root, "bands", "check", "--series", series("b.csv", "120"), "--date", "2026-09-14", "--write")
	b, err := os.ReadFile(filepath.Join(root, wantPath))
	if err != nil || !strings.Contains(string(b), "breached its control band") {
		t.Fatalf("breach intent: %v\n%s", err, b)
	}
	if out, err := artRun(t, root, "intent", "lint"); err != nil {
		t.Fatalf("breach intent fails lint: %v\n%s", err, out)
	}
	evs, _ := eventlog.List(artRepoName(t, root), eventlog.Filter{Types: map[string]bool{"bands.breach": true}})
	if len(evs) != 2 {
		t.Fatalf("bands.breach events: %d", len(evs))
	}
	if _, err := artRun(t, root, "bands", "dismiss", res.Event); err == nil {
		t.Fatal("dismiss without --reason accepted")
	}
	if _, err := artRun(t, root, "bands", "dismiss", res.Event, "--reason", "  "); err == nil {
		t.Fatal("blank reason accepted")
	}
	artMust(t, root, "bands", "dismiss", res.Event, "--reason", "load test")
	if _, err := artRun(t, root, "bands", "dismiss", "nope", "--reason", "x"); err == nil {
		t.Fatal("unknown event dismissed")
	}
}

func TestEvalCIDraft(t *testing.T) {
	root := artTestEnv(t)
	artWrite(t, filepath.Join(root, ".huddle", "config.json"), `{"user":"alice","eval":{"command":"make evals"}}`)
	out := artMust(t, root, "eval", "ci-draft", "--threshold", "0.85")
	for _, want := range []string{`- "CLAUDE.md"`, `- "AGENTS.md"`, `- ".claude/**"`, `- "evals/**"`, "make evals", `EVAL_THRESHOLD: "0.85"`, "unverified"} {
		if !strings.Contains(out, want) {
			t.Errorf("draft missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "{{") && !strings.Contains(out, "${{") {
		t.Error("unrendered placeholder")
	}
	wf := filepath.Join(root, ".github/workflows/agent-evals.yml")
	if _, err := os.Stat(wf); !os.IsNotExist(err) {
		t.Fatal("draft wrote without --write")
	}
	artMust(t, root, "eval", "ci-draft", "--write")
	if _, err := os.Stat(wf); err != nil {
		t.Fatal(err)
	}
	if _, err := artRun(t, root, "eval", "ci-draft", "--write"); err == nil {
		t.Fatal("ci-draft overwrote an existing workflow")
	}
	if _, err := artRun(t, root, "eval", "ci-draft", "--threshold", "2"); err == nil {
		t.Fatal("threshold > 1 accepted")
	}
}

func TestEvalCIDraftTriageAndSpec(t *testing.T) {
	root := artTestEnv(t)
	tri := artMust(t, root, "ci", "draft", "triage", "--workflow", "build")
	for _, want := range []string{"claude -p", `--allowedTools "Read,Grep,Glob"`, "contents: read", `workflows: ["build"]`} {
		if !strings.Contains(tri, want) {
			t.Errorf("triage missing %q", want)
		}
	}
	spec := artMust(t, root, "ci", "draft", "spec", "--branch", "dev")
	for _, want := range []string{`"intent/**/intent.md"`, "branches: [dev]", "gh pr create", "claude -p"} {
		if !strings.Contains(spec, want) {
			t.Errorf("spec draft missing %q", want)
		}
	}
	artMust(t, root, "ci", "draft", "spec", "--write")
	if _, err := artRun(t, root, "ci", "draft", "spec", "--write"); err == nil {
		t.Fatal("overwrote spec workflow")
	}
	if _, err := artRun(t, root, "ci", "draft", "deploy"); err == nil {
		t.Fatal("unknown draft accepted")
	}
}
