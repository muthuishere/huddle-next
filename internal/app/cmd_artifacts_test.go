package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle/internal/artifacts"
	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/gates"
	"github.com/muthuishere/huddle/internal/hooks"
)

// artTestEnv isolates HUDDLE_HOME/HOME, makes a fake git repo, and writes a
// repo config with user alice and owners.
func artTestEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HUDDLE_HOME", filepath.Join(tmp, "huddle-home"))
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("USERPROFILE", filepath.Join(tmp, "home"))
	for _, name := range gates.AgentEnvVars { // tests may run inside an agent session
		t.Setenv(name, "")
	}
	root := filepath.Join(tmp, "proj")
	artWrite(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	artWrite(t, filepath.Join(root, ".huddle", "config.json"), `{
  "user": "alice",
  "owners": {"product": ["bob", "alice"], "tech-lead": ["bob", "alice"], "policy": ["carol"]}
}`)
	prevFire, prevTTY := artFire, gateInteractive
	artFire = func(config.Config, string, string, hooks.Payload) hooks.Result {
		return hooks.Result{Decision: "allow"}
	}
	gateInteractive = func(*Ctx) bool { return false }
	t.Cleanup(func() { artFire, gateInteractive = prevFire, prevTTY })
	return root
}

func artWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func artRun(t *testing.T, root string, args ...string) (string, error) {
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

func artMust(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := artRun(t, root, args...)
	if err != nil {
		t.Fatalf("huddle %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func artRepoName(t *testing.T, root string) string {
	c := &Ctx{Cwd: root}
	info, err := c.Repo()
	if err != nil {
		t.Fatal(err)
	}
	return info.Name
}

func TestArtifactFreshTemplatesLint(t *testing.T) {
	root := artTestEnv(t)
	cases := [][]string{
		{"intent", "new", "Pay retries fail silently"},
		{"spec", "new", "pay-retries-fail-silently"},
		{"plan", "new", "pay-retries-fail-silently"},
		{"adr", "new", "Use pgvector"},
		{"spike", "new", "Can pgvector handle 50M rows?"},
		{"guardrails", "new"},
		{"review-policy", "new"},
		{"eval", "new", "refund-flow"},
		{"bands", "new", "checkout_p95_ms"},
		{"skill", "new", "money-rules"},
		{"lessons", "new", "outage", "--eval", "eval-refund-flow", "--intent", "intent-pay-retries-fail-silently"},
	}
	for _, args := range cases {
		artMust(t, root, args...)
		if out, err := artRun(t, root, args[0], "lint"); err != nil {
			t.Fatalf("fresh %s template fails lint: %v\n%s", args[0], err, out)
		}
	}
	for _, p := range []string{"intent/pay-retries-fail-silently/intent.md", "intent/pay-retries-fail-silently/spec.md",
		"docs/adr/1-use-pgvector.md", "spikes/1-can-pgvector-handle-50m-rows/spike.md", "CLAUDE.md", "REVIEW.md",
		"evals/refund-flow.json", "bands.yaml", ".claude/skills/money-rules/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}
	plan, _ := os.ReadFile(filepath.Join(root, "intent/pay-retries-fail-silently/plan.md"))
	if !strings.Contains(string(plan), "intent: intent-pay-retries-fail-silently") || !strings.Contains(string(plan), "spec: spec-pay-retries-fail-silently") {
		t.Errorf("plan should link intent and spec:\n%s", plan)
	}
}

func TestArtifactNewNeverOverwritesAndDryRun(t *testing.T) {
	root := artTestEnv(t)
	out := artMust(t, root, "intent", "new", "checkout", "--dry-run")
	if !strings.Contains(out, "DRY RUN") {
		t.Fatalf("dry run output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "intent/checkout/intent.md")); !os.IsNotExist(err) {
		t.Fatal("dry run wrote a file")
	}
	artMust(t, root, "intent", "new", "checkout")
	p := filepath.Join(root, "intent/checkout/intent.md")
	artWrite(t, p, "hand written\n")
	if _, err := artRun(t, root, "intent", "new", "checkout"); !errors.Is(err, artifacts.ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "hand written\n" {
		t.Fatal("existing file was overwritten")
	}
}

func TestArtifactWriteLogsSHA(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "intent", "new", "search")
	raw, _ := os.ReadFile(filepath.Join(root, "intent/search/intent.md"))
	evs, err := eventlog.List(artRepoName(t, root), eventlog.Filter{Types: map[string]bool{"artifact.written": true}})
	if err != nil || len(evs) != 1 {
		t.Fatalf("events: %v %v", evs, err)
	}
	if evs[0].Artifact == nil || evs[0].Artifact.SHA256 != artifacts.SHA256(raw) || evs[0].Artifact.Path != "intent/search/intent.md" {
		t.Fatalf("artifact ref: %+v", evs[0].Artifact)
	}
}

func TestArtifactADRNumberingAndOverride(t *testing.T) {
	root := artTestEnv(t)
	artWrite(t, filepath.Join(root, "docs/adr/19-existing.md"), "# ADR 19\n\n- Status: proposed\n")
	artMust(t, root, "adr", "new", "Next decision")
	if _, err := os.Stat(filepath.Join(root, "docs/adr/20-next-decision.md")); err != nil {
		t.Fatal("adr not auto-numbered after 19:", err)
	}
	out := artMust(t, root, "adr", "list", "--json")
	var list []artifacts.Artifact
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list) != 2 {
		t.Fatalf("adr list: %v %s", err, out)
	}
	if list[0].ID != "adr-19" || list[0].Status != "proposed" {
		t.Errorf("legacy adr without front matter: %+v", list[0])
	}

	artWrite(t, filepath.Join(root, ".huddle", "config.json"), `{"user":"alice","artifacts":{"intent":"work/{slug}/INTENT.md"}}`)
	artMust(t, root, "intent", "new", "moved")
	if _, err := os.Stat(filepath.Join(root, "work/moved/INTENT.md")); err != nil {
		t.Fatal("config override ignored:", err)
	}
	artMust(t, root, "intent", "show", "moved")
}

func TestArtifactLintRules(t *testing.T) {
	root := artTestEnv(t)
	artWrite(t, filepath.Join(root, "intent/x/spec.md"), `---
id: spec-x
kind: spec
status: draft
author: alice
links: {}
---
## Requirements

### R1: retries are bounded

No acceptance here.

## Design
d
## Policy concerns
p
## Non-goals
n
## Links
l
`)
	out, err := artRun(t, root, "spec", "lint", "spec-x")
	if err == nil || !strings.Contains(out, "spec.acceptance") {
		t.Fatalf("spec without acceptance must fail lint: %v\n%s", err, out)
	}
	artWrite(t, filepath.Join(root, "intent/x/plan.md"), "---\nid: plan-x\nkind: plan\nstatus: draft\nauthor: a\n---\n## Files that change\nf\n## Order of work\no\n## Risks\nr\n## Proof\n<!-- todo -->\n")
	out, err = artRun(t, root, "plan", "lint", "plan-x")
	if err == nil || !strings.Contains(out, `"Proof" is empty`) {
		t.Fatalf("empty Proof must fail lint: %v\n%s", err, out)
	}
	artMust(t, root, "skill", "new", "long")
	p := filepath.Join(root, ".claude/skills/long/SKILL.md")
	b, _ := os.ReadFile(p)
	artWrite(t, p, strings.Replace(string(b), "Use when", "Trigger for", 1))
	if out, err := artRun(t, root, "skill", "lint"); err == nil || !strings.Contains(out, "skill.trigger") {
		t.Fatalf("skill without trigger phrase must fail: %v\n%s", err, out)
	}
}

func TestArtifactSupersedeAndLink(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "adr", "new", "old way")
	artMust(t, root, "adr", "new", "new way")
	artMust(t, root, "adr", "supersede", "adr-1", "--by", "adr-2")
	artMust(t, root, "adr", "link", "adr-2", "--to", "https://jira.example/X-1")
	old, _ := os.ReadFile(filepath.Join(root, "docs/adr/1-old-way.md"))
	nw, _ := os.ReadFile(filepath.Join(root, "docs/adr/2-new-way.md"))
	if !strings.Contains(string(old), "status: superseded") || !strings.Contains(string(old), "supersededBy: adr-2") {
		t.Errorf("old adr:\n%s", old)
	}
	if !strings.Contains(string(nw), "supersedes: adr-1") || !strings.Contains(string(nw), "external: https://jira.example/X-1") {
		t.Errorf("new adr:\n%s", nw)
	}
	if !strings.Contains(string(nw), "## Decision") {
		t.Error("body lost on front matter rewrite")
	}
}

func artGateID(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, "GATE: "); ok {
			return v
		}
	}
	t.Fatalf("no gate id in %q", out)
	return ""
}

func artFileStatus(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	fm, _ := artifacts.SplitFrontMatter(b)
	return fm.Get("status")
}

// H-edge: an agent runs `huddle gate approve` itself.
func TestGateAgentCannotSelfApprove(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "intent", "new", "refunds") // author: alice
	gid := artGateID(t, artMust(t, root, "gate", "request", "intent-refunds"))

	for _, name := range gates.AgentEnvVars {
		t.Setenv(name, "1")
		if _, err := artRun(t, root, "gate", "approve", gid, "--as", "bob"); !errors.Is(err, gates.ErrAgent) {
			t.Fatalf("%s=1 with --as must be refused, got %v", name, err)
		}
		t.Setenv(name, "")
	}
	if _, err := artRun(t, root, "gate", "approve", gid); !errors.Is(err, gates.ErrAgent) {
		t.Fatalf("non-TTY without --as must be refused, got %v", err)
	}
	if _, err := artRun(t, root, "gate", "approve", gid, "--as", "alice"); err == nil || !strings.Contains(err.Error(), "separation of duties") {
		t.Fatalf("author must not approve own artifact, got %v", err)
	}
	if _, err := artRun(t, root, "gate", "approve", gid, "--as", "mallory"); err == nil || !strings.Contains(err.Error(), "not an owner") {
		t.Fatalf("non-owner must be refused, got %v", err)
	}
	gateInteractive = func(*Ctx) bool { return true } // alice at a terminal is still the author
	if _, err := artRun(t, root, "gate", "approve", gid); err == nil {
		t.Fatal("interactive author approved own artifact")
	}
	if st := artFileStatus(t, root, "intent/refunds/intent.md"); st != "draft" {
		t.Fatalf("refused approvals changed status to %q", st)
	}
	gateInteractive = func(*Ctx) bool { return false }
	out := artMust(t, root, "gate", "approve", gid, "--as", "bob")
	if !strings.Contains(out, "STATE: approved") {
		t.Fatalf("approve: %s", out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "intent/refunds/intent.md"))
	if !strings.Contains(string(b), "status: approved") || !strings.Contains(string(b), "approvedBy: bob") {
		t.Fatalf("front matter not updated:\n%s", b)
	}
	if !strings.Contains(string(b), "## Problem") {
		t.Fatal("body lost")
	}
}

// H-edge: a blocking hook denies; approval in chat must not open the gate.
func TestGateHookDenyKeepsGateClosed(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "plan", "new", "payments")
	gid := artGateID(t, artMust(t, root, "gate", "request", "plan-payments"))
	var seen []string
	artFire = func(_ config.Config, _ string, event string, _ hooks.Payload) hooks.Result {
		seen = append(seen, event)
		if event == "gate.request" {
			return hooks.Result{Decision: "deny", Reasons: []string{"PR checks failing"}}
		}
		return hooks.Result{Decision: "allow"}
	}
	if _, err := artRun(t, root, "gate", "approve", gid, "--as", "bob"); !errors.Is(err, gates.ErrHookBlocked) {
		t.Fatalf("hook deny must block, got %v", err)
	}
	if st := artFileStatus(t, root, "intent/payments/plan.md"); st != "draft" {
		t.Fatalf("status changed to %q despite deny", st)
	}
	out := artMust(t, root, "gate", "status", "--json")
	if !strings.Contains(out, `"state": "requested"`) {
		t.Fatalf("gate should stay requested: %s", out)
	}
	if _, err := artRun(t, root, "gate", "approve", gid, "--as", "bob", "--override"); err == nil {
		t.Fatal("override without reason accepted")
	}
	artMust(t, root, "gate", "approve", gid, "--as", "bob", "--override", "--reason", "checks flaky, verified manually")
	if st := artFileStatus(t, root, "intent/payments/plan.md"); st != "approved" {
		t.Fatalf("override did not approve: %q", st)
	}
	evs, _ := eventlog.List(artRepoName(t, root), eventlog.Filter{Types: map[string]bool{gates.EvOverride: true}})
	if len(evs) != 1 || evs[0].By != "bob" {
		t.Fatalf("override not logged: %+v", evs)
	}
	if !strings.Contains(strings.Join(seen, ","), "gate.approved") {
		t.Errorf("gate.approved hook not fired: %v", seen)
	}
}

// H-edge: stale gate on changed hash.
func TestGateStaleWhenFileChanges(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "spec", "new", "auth")
	gid := artGateID(t, artMust(t, root, "gate", "request", "spec-auth"))
	artMust(t, root, "gate", "approve", gid, "--as", "bob")
	var gs []gates.Gate
	json.Unmarshal([]byte(artMust(t, root, "gate", "status", "--all", "--json")), &gs)
	if len(gs) != 1 || gs[0].Stale {
		t.Fatalf("fresh approval should not be stale: %+v", gs)
	}
	p := filepath.Join(root, "intent/auth/spec.md")
	b, _ := os.ReadFile(p)
	artWrite(t, p, string(b)+"\nA late requirement.\n")
	gs = nil
	json.Unmarshal([]byte(artMust(t, root, "gate", "status", "--json")), &gs)
	if len(gs) != 1 || !gs[0].Stale {
		t.Fatalf("changed file must mark the gate STALE: %+v", gs)
	}
	if out := artMust(t, root, "gate", "status"); !strings.Contains(out, "APPROVED STALE") {
		t.Fatalf("human output: %s", out)
	}
}

func TestGateSkillNeedsPolicyOwner(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "skill", "new", "money")
	if _, err := artRun(t, root, "gate", "request", "skill-money", "--role", "tech-lead"); err == nil {
		t.Fatal("skill gate with non-policy role accepted")
	}
	gid := artGateID(t, artMust(t, root, "gate", "request", "skill-money"))
	if _, err := artRun(t, root, "gate", "approve", gid, "--as", "bob"); err == nil {
		t.Fatal("bob is not a policy owner")
	}
	artMust(t, root, "gate", "approve", gid, "--as", "carol")
}
