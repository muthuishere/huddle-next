package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/huddle/internal/gates"
	"github.com/muthuishere/huddle/internal/sdlc"
)

func sdlcReport(t *testing.T, root string) sdlc.Report {
	t.Helper()
	var r sdlc.Report
	out := artMust(t, root, "sdlc", "status", "--json")
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return r
}

func TestSdlcNextWalksTheChain(t *testing.T) {
	root := artTestEnv(t)
	step := func() sdlc.Step { return sdlcReport(t, root).Next }
	if s := step(); s.Command != "huddle guardrails new" {
		t.Fatalf("empty repo: %+v", s)
	}
	artMust(t, root, "guardrails", "new")
	if s := step(); !strings.HasPrefix(s.Command, "huddle intent new") {
		t.Fatalf("no intents: %+v", s)
	}
	artMust(t, root, "intent", "new", "billing")
	if s := step(); !strings.Contains(s.Command, "huddle gate request intent-billing --role product") {
		t.Fatalf("draft intent: %+v", s)
	}
	gid := artGateID(t, artMust(t, root, "gate", "request", "intent-billing"))
	if s := step(); s.Command != "huddle gate approve "+gid {
		t.Fatalf("pending gate: %+v", s)
	}
	artMust(t, root, "gate", "approve", gid, "--as", "bob")
	if s := step(); s.Command != "huddle spec new billing" {
		t.Fatalf("approved intent: %+v", s)
	}
	human := artMust(t, root, "sdlc")
	if !strings.Contains(human, "billing") || !strings.Contains(human, "NEXT:") {
		t.Fatalf("human status: %s", human)
	}
}

// H-edge: user hand-edits plan.md mid-build; the file wins, nothing re-derives it.
func TestSdlcHandEditedPlanStatusWins(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "plan", "new", "retry")
	gid := artGateID(t, artMust(t, root, "gate", "request", "plan-retry"))
	artMust(t, root, "gate", "approve", gid, "--as", "bob")
	if r := sdlcReport(t, root); r.Changes[0].Plan.Status != "approved" {
		t.Fatalf("after approval: %+v", r.Changes[0].Plan)
	}
	p := filepath.Join(root, "intent/retry/plan.md")
	b, _ := os.ReadFile(p)
	artWrite(t, p, strings.Replace(string(b), "status: approved", "status: draft", 1))
	r := sdlcReport(t, root)
	if r.Changes[0].Plan.Status != "draft" {
		t.Fatalf("hand edit lost: %+v", r.Changes[0].Plan)
	}
	for _, cmd := range [][]string{{"plan", "status", "plan-retry", "--json"}, {"plan", "list", "--json"}} {
		if out := artMust(t, root, cmd...); !strings.Contains(out, `"status": "draft"`) {
			t.Fatalf("%v ignores the file: %s", cmd, out)
		}
	}
	if b2, _ := os.ReadFile(p); !strings.Contains(string(b2), "status: draft") {
		t.Fatal("a read command rewrote the file")
	}
	var gs []gates.Gate
	json.Unmarshal([]byte(artMust(t, root, "gate", "status", "--json")), &gs)
	if len(gs) != 1 || !gs[0].Stale || gs[0].FileStatus != "draft" {
		t.Fatalf("gate should report STALE with file status draft: %+v", gs)
	}
}

// H-edge: eval runner blocked/unverified ⇒ no PASS.
func TestVerifyEvalBlockedNeverPass(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "plan", "new", "evals")
	for _, status := range []string{"blocked", "unverified"} {
		var v gates.Verdict
		out := artMust(t, root, "verify", "record", "plan-evals", "--verdict", "PASS", "--eval-status", status, "--json")
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		if v.Verdict != gates.CONCERNS || v.Requested != gates.PASS {
			t.Fatalf("eval %s recorded %+v", status, v)
		}
	}
	findings := filepath.Join(root, "findings.json")
	artWrite(t, findings, `[{"severity":"important","text":"x"}]`)
	artMust(t, root, "verify", "record", "plan-evals", "--verdict", "PASS", "--findings", findings)
	var vs []gates.Verdict
	json.Unmarshal([]byte(artMust(t, root, "verify", "status", "--json")), &vs)
	if len(vs) != 1 || vs[0].Verdict != gates.PASS || vs[0].Stale || vs[0].Findings != 1 {
		t.Fatalf("fresh PASS: %+v", vs)
	}
	p := filepath.Join(root, "intent/evals/plan.md")
	b, _ := os.ReadFile(p)
	artWrite(t, p, string(b)+"\nchanged\n")
	vs = nil
	json.Unmarshal([]byte(artMust(t, root, "verify", "status", "plan-evals", "--json")), &vs)
	if len(vs) != 1 || !vs[0].Stale {
		t.Fatalf("verdict must go stale when the tree changes: %+v", vs)
	}
	if r := sdlcReport(t, root); r.Changes[0].Verdict == nil || !r.Changes[0].Verdict.Stale {
		t.Fatalf("sdlc status stale verdict: %+v", r.Changes[0].Verdict)
	}
	if _, err := artRun(t, root, "verify", "record", "plan-evals", "--verdict", "MAYBE"); err == nil {
		t.Fatal("bad verdict accepted")
	}
}

func TestSdlcStatus50IntentsUnder30ms(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budget is measured on a dev machine, not shared CI runners")
	}
	root := artTestEnv(t)
	for i := 0; i < 50; i++ {
		slug := fmt.Sprintf("change-%02d", i)
		artMust(t, root, "intent", "new", slug)
		if i%2 == 0 {
			artMust(t, root, "spec", "new", slug)
			artMust(t, root, "plan", "new", slug)
		}
	}
	gid := artGateID(t, artMust(t, root, "gate", "request", "intent-change-00"))
	artMust(t, root, "gate", "approve", gid, "--as", "bob")
	artMust(t, root, "verify", "record", "plan-change-00", "--verdict", "CONCERNS")
	c := &Ctx{Cwd: root}
	env, err := artEnv(c)
	if err != nil {
		t.Fatal(err)
	}
	var times []time.Duration
	for i := 0; i < 7; i++ {
		start := time.Now()
		r, err := sdlc.Status(env.Repo, env.Cfg)
		times = append(times, time.Since(start))
		if err != nil || len(r.Changes) != 50 {
			t.Fatalf("status: %v changes=%d", err, len(r.Changes))
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	if p50 := times[len(times)/2]; p50 > 30*time.Millisecond {
		t.Fatalf("sdlc status p50 %v > 30ms on 50 intents (all: %v)", p50, times)
	}
}
