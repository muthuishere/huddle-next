package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUninstallSkills(t *testing.T) {
	root := coreEnv(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_SKILLS_DIR", filepath.Join(dir, "claude"))
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(dir, "agents"))
	t.Setenv("PATH", t.TempDir()) // no codex

	if _, err := runCore(t, root, "install"); err == nil {
		t.Fatal("install without --skills accepted")
	}
	out, err := runCore(t, root, "install", "--skills")
	if err != nil || !strings.Contains(out, "huddle-sdlc") {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "claude", "huddle-sdlc", "stages", "plan.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agents")); err == nil {
		t.Fatal("agents target used without codex or --agents")
	}
	out, _ = runCore(t, root, "doctor", "--json")
	if !strings.Contains(out, `"skills-claude"`) {
		t.Fatal(out)
	}
	out, err = runCore(t, root, "uninstall", "--skills")
	if err != nil || strings.Count(out, "removed") != 3 {
		t.Fatalf("%v %s", err, out)
	}
}
