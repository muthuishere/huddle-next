package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/paths"
)

func TestHooksListTestFireExport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh hooks")
	}
	root := coreEnv(t)
	mustWrite(t, filepath.Join(root, ".huddle", "config.json"), `{"hooks":[
		{"on":"gate.request","match":{"stage":"deploy"},"run":"echo '{\"decision\":\"deny\",\"reason\":\"PR not approved\"}'","blocking":true},
		{"on":"verdict.issued","run":"echo token=abcdefgh12345678"}
	]}`)

	out, err := runCore(t, root, "hooks", "list")
	if err != nil || !strings.Contains(out, "gate.request") || !strings.Contains(out, "stage=deploy") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if _, err := os.Stat(paths.HooksSeen("proj")); err != nil {
		t.Fatalf("hooks list did not record review: %v", err)
	}

	payload := filepath.Join(root, "p.json")
	mustWrite(t, payload, `{"stage":"deploy"}`)
	out, err = runCore(t, root, "hooks", "test", "gate.request", "--payload", payload, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil || res["decision"] != "deny" {
		t.Fatalf("test: %v %s", err, out)
	}
	if _, err := runCore(t, root, "hooks", "fire", "gate.request", "--payload", payload); err == nil {
		t.Fatal("fire should fail on deny")
	}
	out, err = runCore(t, root, "hooks", "test", "verdict.issued")
	if err != nil || strings.Contains(out, "abcdefgh12345678") || !strings.Contains(out, "DECISION: allow") {
		t.Fatalf("redaction/allow: %v %s", err, out)
	}

	out, err = runCore(t, root, "hooks", "export", "--host", "claude")
	if err != nil || !strings.Contains(out, "DRAFT") || !strings.Contains(out, "disableBypassPermissionsMode") {
		t.Fatalf("export draft: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); err == nil {
		t.Fatal("draft export wrote files")
	}
	if _, err := runCore(t, root, "hooks", "export", "--host", "claude", "--write"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "hooks", "huddle-production-gate.sh")); err != nil {
		t.Fatal(err)
	}
	if _, err := runCore(t, root, "hooks", "export", "--host", "codex"); err == nil {
		t.Fatal("unsupported host accepted")
	}
}
