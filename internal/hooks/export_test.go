package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle/internal/config"
)

func writtenPack(t *testing.T, extra map[string]string) (string, Pack) {
	t.Helper()
	cfg := config.Config{Extra: map[string]json.RawMessage{}}
	for k, v := range extra {
		cfg.Extra[k] = json.RawMessage(v)
	}
	root := t.TempDir()
	p := ExportClaude(cfg)
	if _, err := WritePack(p, root, false); err != nil {
		t.Fatal(err)
	}
	return root, p
}

// runHook runs a pack script like Claude Code would; returns exit code and output.
func runHook(t *testing.T, root, script, input string, env ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", filepath.Join(root, ".claude/hooks", script))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), append([]string{"CLAUDE_PROJECT_DIR=" + root}, env...)...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	} else if err != nil {
		t.Fatal(err)
	}
	return 0, string(out)
}

func edit(root, rel string) string {
	return `{"tool_name":"Edit","tool_input":{"file_path":"` + filepath.Join(root, rel) + `","old_string":"a"}}`
}

func TestExportDraftShape(t *testing.T) {
	p := ExportClaude(config.Config{Extra: map[string]json.RawMessage{
		"innerLoop":     json.RawMessage(`["go test:*"]`),
		"formatCommand": json.RawMessage(`"gofmt -w"`),
	}})
	b, _ := json.Marshal(p.Settings)
	for _, want := range []string{"PreToolUse", "PostToolUse", "huddle-production-gate.sh", `"Bash(go test:*)"`, "huddle-post-edit.sh"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("settings missing %s: %s", want, b)
		}
	}
	m, _ := json.Marshal(p.Managed)
	for _, want := range []string{"Read(.env*)", "Read(**/secrets/**)", `"disableBypassPermissionsMode":"disable"`} {
		if !strings.Contains(string(m), want) {
			t.Errorf("managed missing %s: %s", want, m)
		}
	}
	for name, body := range p.Files {
		if !strings.HasPrefix(body, "#!/bin/sh\n") || strings.Contains(body, "@@") {
			t.Errorf("%s: bad script", name)
		}
	}
}

func TestExportNoOverwriteWithoutForce(t *testing.T) {
	root, p := writtenPack(t, nil)
	target := filepath.Join(root, ".claude/hooks/huddle-lib.sh")
	os.WriteFile(target, []byte("mine"), 0o644)
	os.WriteFile(filepath.Join(root, PackSettingsPath), []byte(`{"hooks":{"x":1},"model":"opus"}`), 0o644)
	rep, err := WritePack(p, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "mine" {
		t.Fatal("overwrote without force")
	}
	if b, _ := os.ReadFile(filepath.Join(root, PackSettingsPath)); !strings.Contains(string(b), `"x":1`) {
		t.Fatalf("settings replaced: %s %+v", b, rep)
	}
	if _, err := WritePack(p, root, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, PackSettingsPath))
	if !strings.Contains(string(b), "PreToolUse") || !strings.Contains(string(b), "opus") {
		t.Fatalf("force merge wrong: %s", b)
	}
}

func TestExportScripts(t *testing.T) {
	skipWindows(t)
	for _, noJQ := range []string{"0", "1"} {
		t.Run("nojq="+noJQ, func(t *testing.T) {
			t.Setenv("HUDDLE_HOOK_NO_JQ", noJQ)
			exportScripts(t)
		})
	}
}

func exportScripts(t *testing.T) {
	root, _ := writtenPack(t, map[string]string{"deployPatterns": `["kubectl apply","./deploy.sh prod"]`})

	t.Run("protected paths", func(t *testing.T) {
		if code, out := runHook(t, root, "huddle-protected-paths.sh", edit(root, ".huddle/config.json")); code != 2 || !strings.Contains(out, "protected") {
			t.Fatalf("%d %s", code, out)
		}
		if code, _ := runHook(t, root, "huddle-protected-paths.sh", edit(root, "src/api/generated/types.go")); code != 2 {
			t.Fatal("generated not protected")
		}
		if code, out := runHook(t, root, "huddle-protected-paths.sh", edit(root, "src/main.go")); code != 0 {
			t.Fatalf("%d %s", code, out)
		}
	})

	t.Run("fix mode tests", func(t *testing.T) {
		if code, _ := runHook(t, root, "huddle-fix-mode-tests.sh", edit(root, "pkg/a_test.go")); code != 0 {
			t.Fatal("blocked outside fix mode")
		}
		if code, _ := runHook(t, root, "huddle-fix-mode-tests.sh", edit(root, "pkg/a_test.go"), "HUDDLE_FIX_MODE=1"); code != 2 {
			t.Fatal("test edit allowed in fix mode")
		}
		if code, _ := runHook(t, root, "huddle-fix-mode-tests.sh", edit(root, "pkg/a.go"), "HUDDLE_FIX_MODE=1"); code != 0 {
			t.Fatal("code edit blocked in fix mode")
		}
	})

	t.Run("plan drift", func(t *testing.T) {
		os.MkdirAll(filepath.Join(root, "intent/pay"), 0o755)
		os.WriteFile(filepath.Join(root, "intent/pay/plan.md"), []byte("# Plan\n\n## Files that change\n\n- `internal/pay/pay.go` — add refund\n- docs/ — notes\n\n## Risks\n\n- other.go\n"), 0o644)
		if code, out := runHook(t, root, "huddle-plan-drift.sh", edit(root, "internal/pay/pay.go")); code != 0 || out != "" {
			t.Fatalf("%d %q", code, out)
		}
		if _, out := runHook(t, root, "huddle-plan-drift.sh", edit(root, "docs/x.md")); out != "" {
			t.Fatalf("dir listing not honoured: %q", out)
		}
		code, out := runHook(t, root, "huddle-plan-drift.sh", edit(root, "other.go"))
		if code != 0 || !strings.Contains(out, "additionalContext") {
			t.Fatalf("%d %q", code, out)
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("invalid JSON %q: %v", out, err)
		}
	})

	t.Run("production gate", func(t *testing.T) {
		bin := t.TempDir()
		fake := filepath.Join(bin, "huddle")
		bash := func(c string) string { return `{"tool_name":"Bash","tool_input":{"command":"` + c + `"}}` }
		path := "PATH=" + bin + ":" + os.Getenv("PATH")

		os.WriteFile(fake, []byte("#!/bin/sh\necho '[{\"id\":\"g-1\",\"name\":\"deploy\",\"state\":\"requested\",\"stale\":false}]'\n"), 0o755)
		if code, _ := runHook(t, root, "huddle-production-gate.sh", bash("go test ./..."), path); code != 0 {
			t.Fatal("non-deploy blocked")
		}
		if code, out := runHook(t, root, "huddle-production-gate.sh", bash("kubectl apply -f k8s/"), path); code != 2 || !strings.Contains(out, "gate") {
			t.Fatalf("%d %s", code, out)
		}
		os.WriteFile(fake, []byte("#!/bin/sh\necho '[{\"id\":\"g-1\",\"name\":\"deploy\",\"state\":\"approved\",\"stale\":true}]'\n"), 0o755)
		if code, _ := runHook(t, root, "huddle-production-gate.sh", bash("kubectl apply -f k8s/"), path); code != 2 {
			t.Fatal("stale approval accepted")
		}
		os.WriteFile(fake, []byte("#!/bin/sh\necho '[{\"id\":\"g-2\",\"name\":\"plan\",\"state\":\"approved\",\"stale\":false},{\"id\":\"g-1\",\"name\":\"deploy\",\"state\":\"approved\",\"stale\":false}]'\n"), 0o755)
		if code, out := runHook(t, root, "huddle-production-gate.sh", bash("./deploy.sh prod --now"), path); code != 0 {
			t.Fatalf("approved gate blocked: %s", out)
		}
	})
}

func TestExportPostEdit(t *testing.T) {
	skipWindows(t)
	root, _ := writtenPack(t, map[string]string{"formatCommand": `"sed -i.bak s/x/y/"`, "lintCommand": `"grep -q y"`})
	f := filepath.Join(root, "a.txt")
	os.WriteFile(f, []byte("x\n"), 0o644)
	if code, out := runHook(t, root, "huddle-post-edit.sh", edit(root, "a.txt")); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if b, _ := os.ReadFile(f); string(b) != "y\n" {
		t.Fatalf("format not run: %q", b)
	}
	os.WriteFile(f, []byte("z\n"), 0o644)
	lintOnly := ExportClaude(config.Config{Extra: map[string]json.RawMessage{"lintCommand": json.RawMessage(`"grep -q q"`)}})
	os.WriteFile(filepath.Join(root, ".claude/hooks/huddle-post-edit.sh"), []byte(lintOnly.Files[".claude/hooks/huddle-post-edit.sh"]), 0o755)
	if code, out := runHook(t, root, "huddle-post-edit.sh", edit(root, "a.txt")); code != 2 || !strings.Contains(out, "lint failed") {
		t.Fatalf("%d %s", code, out)
	}
}
