package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/hooks"
)

// The exported production-gate hook and `gate status --json` share one
// contract: a named deploy gate, approved and not stale, opens deploys.
func TestDeployGateContractWithExportedHook(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "plan", "new", "release-1") // author: alice
	out := artMust(t, root, "gate", "request", "plan-release-1", "--role", "tech-lead", "--name", "deploy")
	gid := artGateID(t, out)
	artMust(t, root, "gate", "approve", gid, "--as", "bob")

	status := func() (string, []gates.Gate) {
		raw := artMust(t, root, "gate", "status", "--json")
		var list []map[string]any
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			t.Fatalf("status --json: %v\n%s", err, raw)
		}
		for _, f := range []string{"id", "name", "artifact", "role", "state", "stale", "approvedBy", "ts"} {
			if len(list) == 0 {
				t.Fatalf("no gates: %s", raw)
			}
			if _, ok := list[0][f]; !ok {
				t.Errorf("status --json missing field %q: %s", f, raw)
			}
		}
		var gs []gates.Gate
		json.Unmarshal([]byte(raw), &gs)
		return raw, gs
	}

	raw, gs := status()
	if len(gs) != 1 || gs[0].ID != gid || gs[0].Name != "deploy" || gs[0].ArtifactID != "plan-release-1" ||
		gs[0].State != gates.StateApproved || gs[0].Stale || gs[0].ApprovedBy != "bob" || gs[0].TS.IsZero() {
		t.Fatalf("approved deploy gate: %s", raw)
	}
	if runtime.GOOS != "windows" {
		if code, msg := runProductionGate(t, root, raw); code != 0 {
			t.Fatalf("approved deploy gate blocked the hook (%d): %s", code, msg)
		}
	}

	p := filepath.Join(root, "intent/release-1/plan.md")
	b, _ := os.ReadFile(p)
	artWrite(t, p, string(b)+"\nlate change\n")
	raw, gs = status()
	if len(gs) != 1 || gs[0].State != gates.StateApproved || !gs[0].Stale {
		t.Fatalf("changed artifact must be stale: %s", raw)
	}
	if runtime.GOOS != "windows" {
		if code, msg := runProductionGate(t, root, raw); code != 2 || !strings.Contains(msg, "--name deploy") {
			t.Fatalf("stale deploy gate must block (%d): %s", code, msg)
		}
	}
}

// Skills write a reviewed draft with --from; huddle keeps id/kind/author and
// restarts status at draft so the gate still decides.
func TestNewFromDraftAndTimebox(t *testing.T) {
	root := artTestEnv(t)
	draft := filepath.Join(t.TempDir(), "intent.md")
	artWrite(t, draft, "---\nid: wrong\nkind: intent\nstatus: approved\nauthor: mallory\n---\n# Refunds\n\n## Problem\nreviewed text\n")
	artMust(t, root, "intent", "new", "refunds", "--from", draft)
	b, _ := os.ReadFile(filepath.Join(root, "intent/refunds/intent.md"))
	for _, want := range []string{"id: intent-refunds", "status: draft", "author: alice", "reviewed text"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("--from lost %q:\n%s", want, b)
		}
	}
	out := artMust(t, root, "spike", "new", "lock contention", "--timebox", "1d", "--dry-run")
	if !strings.Contains(out, "_1d — started ") {
		t.Fatalf("--timebox not applied:\n%s", out)
	}
}

// runProductionGate runs the exported hook with the jq-free parser against a
// fake huddle that prints statusJSON.
func runProductionGate(t *testing.T, root, statusJSON string) (int, string) {
	t.Helper()
	hookRoot := t.TempDir()
	if _, err := hooks.WritePack(hooks.ExportClaude(config.Config{}), hookRoot, false); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	jsonFile := filepath.Join(bin, "status.json")
	artWrite(t, jsonFile, statusJSON)
	artWrite(t, filepath.Join(bin, "huddle"), "#!/bin/sh\ncat '"+jsonFile+"'\n")
	os.Chmod(filepath.Join(bin, "huddle"), 0o755)

	cmd := exec.Command("sh", filepath.Join(hookRoot, ".claude/hooks/huddle-production-gate.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+hookRoot, "HUDDLE_HOOK_NO_JQ=1", "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"kubectl apply -f k8s/"}}`)
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	} else if err != nil {
		t.Fatal(err)
	}
	return 0, string(out)
}
