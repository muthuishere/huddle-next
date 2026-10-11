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

	// The skill family is the product: a bare `huddle install` installs it. This
	// is asserted rather than assumed because the whole value proposition is that
	// nobody has to know a flag to get the skills, and a stray positional must
	// still be refused rather than silently treated as an install.
	out, err := runCore(t, root, "install")
	if err != nil || !strings.Contains(out, "huddle-sdlc") {
		t.Fatalf("bare install did not install the skill family: %v %s", err, out)
	}
	if _, err := runCore(t, root, "install", "bogus"); err == nil {
		t.Fatal("install accepted a positional argument")
	}
	// --skills keeps working: every published postinstall hook and the brew cask
	// pass it, so dropping it would break installs already in the wild.
	out, err = runCore(t, root, "install", "--skills")
	if err != nil || !strings.Contains(out, "huddle-sdlc") {
		t.Fatalf("--skills no longer accepted: %v %s", err, out)
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
