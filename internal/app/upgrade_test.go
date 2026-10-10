package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muthuishere/huddle/internal/install"
	"github.com/muthuishere/huddle/internal/paths"
)

// TestUpgradeReplacesLegacyLinkAndKeepsMemories is the upgrade contract: taking
// over the old install.sh symlink must retire the old single-skill checkout
// while the user's memories survive. Losing a memory on upgrade is silent data
// loss, so it is asserted here rather than trusted.
func TestUpgradeReplacesLegacyLinkAndKeepsMemories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDDLE_HOME", filepath.Join(home, ".config", ".huddle"))
	skillsDir := filepath.Join(home, ".claude", "skills")
	t.Setenv("CLAUDE_SKILLS_DIR", skillsDir)
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(home, ".agents", "skills"))
	t.Setenv(install.EnvNoSync, "")

	// The old Python skill: a repo checkout with SKILL.md, symlinked into place
	// exactly as install.sh used to do.
	oldRepo := filepath.Join(home, "workspace", "huddle-old")
	mustMkdir(t, filepath.Join(oldRepo, ".git"))
	mustWrite(t, filepath.Join(oldRepo, "SKILL.md"), "---\nname: huddle\n---\nold\n")
	mustWrite(t, filepath.Join(oldRepo, "install.sh"), "#!/bin/sh\n")
	mustMkdir(t, skillsDir)
	if err := os.Symlink(oldRepo, filepath.Join(skillsDir, "huddle")); err != nil {
		t.Fatal(err)
	}

	// A memory the user accumulated under the legacy config root.
	legacy := filepath.Join(home, ".config", "muthuishere-agent-skills")
	memDir := filepath.Join(legacy, "personas", "shaama", "memories")
	mustMkdir(t, memDir)
	const memBody = "# what shaama learned\n\nThe provider call blocks the response.\n"
	mustWrite(t, filepath.Join(memDir, "provider-blocking.md"), memBody)
	mustWrite(t, filepath.Join(legacy, "personas", "shaama", "shaama.md"), "---\nid: shaama\n---\nvoice\n")

	synced, blocked, memories := syncSkills("")

	// 1. The legacy symlink is gone, replaced by a real huddle-owned dir.
	linkPath := filepath.Join(skillsDir, "huddle")
	st, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("huddle skill missing after upgrade: %v", err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		t.Error("legacy symlink was not replaced — the upgrade did not take over the link")
	}
	if !install.Owned(linkPath) {
		t.Error("replacement dir carries no ownership marker")
	}
	if _, err := os.Stat(filepath.Join(linkPath, "modes", "research.md")); err != nil {
		t.Errorf("new skill family not fully installed: %v", err)
	}
	if len(synced) == 0 {
		t.Error("upgrade reported nothing as synced")
	}

	// 2. The old checkout itself is untouched — a link was removed, not a repo.
	if _, err := os.Stat(filepath.Join(oldRepo, "SKILL.md")); err != nil {
		t.Errorf("the old repo was damaged by the upgrade: %v", err)
	}

	// 3. The memory survived, byte for byte, and the original is still there.
	carried := filepath.Join(paths.GlobalPersonas(), "shaama", "memories", "provider-blocking.md")
	got, err := os.ReadFile(carried)
	if err != nil {
		t.Fatalf("memory lost on upgrade: %v (blocked=%v, memories=%q)", err, blocked, memories)
	}
	if string(got) != memBody {
		t.Errorf("memory changed on upgrade:\n got %q\nwant %q", got, memBody)
	}
	if _, err := os.Stat(filepath.Join(memDir, "provider-blocking.md")); err != nil {
		t.Errorf("migration moved the original instead of copying it: %v", err)
	}
	if memories == "" {
		t.Error("upgrade did not report that memories were carried")
	}
}

// TestSyncIsIdempotentAndQuietWhenCurrent keeps the start path from doing work,
// or saying anything, on every invocation after the first.
func TestSyncIsIdempotentAndQuietWhenCurrent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDDLE_HOME", filepath.Join(home, ".config", ".huddle"))
	t.Setenv("CLAUDE_SKILLS_DIR", filepath.Join(home, ".claude", "skills"))
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(home, ".agents", "skills"))

	if synced, _, _ := syncSkills(""); len(synced) == 0 {
		t.Fatal("first sync installed nothing")
	}
	synced, blocked, memories := syncSkills("")
	if len(synced) != 0 {
		t.Errorf("second sync rewrote skills that were already current: %v", synced)
	}
	if len(blocked) != 0 || memories != "" {
		t.Errorf("second sync was not silent: blocked=%v memories=%q", blocked, memories)
	}
}

// TestSyncLeavesForeignDirsAlone: huddle must never claim a skill dir someone
// else owns, however much it would like to.
func TestSyncLeavesForeignDirsAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDDLE_HOME", filepath.Join(home, ".config", ".huddle"))
	skillsDir := filepath.Join(home, ".claude", "skills")
	t.Setenv("CLAUDE_SKILLS_DIR", skillsDir)
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(home, ".agents", "skills"))

	mustMkdir(t, filepath.Join(skillsDir, "huddle"))
	const mine = "someone else's skill\n"
	mustWrite(t, filepath.Join(skillsDir, "huddle", "SKILL.md"), mine)

	_, blocked, _ := syncSkills("")

	got, err := os.ReadFile(filepath.Join(skillsDir, "huddle", "SKILL.md"))
	if err != nil || string(got) != mine {
		t.Fatalf("a foreign skill dir was overwritten: err=%v content=%q", err, got)
	}
	found := false
	for _, b := range blocked {
		if b != "" {
			found = true
		}
	}
	if !found {
		t.Error("overwriting was refused but never reported — the user would not know")
	}
}

// TestSyncRespectsOptOut covers the env kill switch.
func TestSyncRespectsOptOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDDLE_HOME", filepath.Join(home, ".config", ".huddle"))
	skillsDir := filepath.Join(home, ".claude", "skills")
	t.Setenv("CLAUDE_SKILLS_DIR", skillsDir)
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(home, ".agents", "skills"))
	t.Setenv(install.EnvNoSync, "1")

	synced, blocked, memories := syncSkills("")
	if len(synced) != 0 || len(blocked) != 0 || memories != "" {
		t.Errorf("opt-out ignored: synced=%v blocked=%v memories=%q", synced, blocked, memories)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "huddle")); !os.IsNotExist(err) {
		t.Error("opt-out set, yet skills were installed anyway")
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
