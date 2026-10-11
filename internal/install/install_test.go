package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func fakeSkills() fstest.MapFS {
	return fstest.MapFS{
		"huddle/SKILL.md":               {Data: []byte("---\nname: huddle\n---\n")},
		"huddle/modes/discussion.md":    {Data: []byte("d")},
		"huddle-sdlc/SKILL.md":          {Data: []byte("---\nname: huddle-sdlc\n---\n")},
		"huddle-sdlc/stages/plan.md":    {Data: []byte("p")},
		"not-a-skill/readme.md":         {Data: []byte("x")},
		"huddle-personas/SKILL.md":      {Data: []byte("x")},
		"huddle-personas/flows/.keep":   {Data: nil},
		"huddle-personas/flows/make.md": {Data: []byte("m")},
	}
}

func TestInstallUpdateUninstall(t *testing.T) {
	dir := t.TempDir()
	targets := []Target{{Host: "claude", Dir: filepath.Join(dir, "claude")}, {Host: "agents", Dir: filepath.Join(dir, "agents")}}
	res, err := Install(fakeSkills(), targets, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 6 || res[0].Action != "installed" {
		t.Fatalf("%+v", res)
	}
	for _, p := range []string{"claude/huddle/modes/discussion.md", "agents/huddle-sdlc/stages/plan.md", "claude/huddle/" + Marker} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "claude/not-a-skill")); err == nil {
		t.Fatal("installed a dir without SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(dir, "claude/huddle-personas/flows/.keep")); err == nil {
		t.Fatal(".keep copied")
	}
	if m, err := ReadMarker(filepath.Join(dir, "claude/huddle")); err != nil || m.Owner != "huddle" {
		t.Fatal(m, err)
	}
	// stale file from a previous version disappears on update
	os.WriteFile(filepath.Join(dir, "claude/huddle/stale.md"), []byte("old"), 0o644)
	res, err = Install(fakeSkills(), targets, Options{})
	if err != nil || res[0].Action != "updated" {
		t.Fatal(res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "claude/huddle/stale.md")); err == nil {
		t.Fatal("stale file survived update")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "claude", ".huddle*"))
	if len(leftovers) != 0 {
		t.Fatal("temp dirs left:", leftovers)
	}
	// user's own dir is never touched
	user := filepath.Join(dir, "claude/mine")
	os.MkdirAll(user, 0o755)
	res, err = Uninstall(fakeSkills(), targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Action != "removed" {
			t.Fatalf("%+v", r)
		}
	}
	if _, err := os.Stat(user); err != nil {
		t.Fatal("removed user dir")
	}
}

func TestUnownedDirSkippedUnlessForce(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "huddle")
	os.MkdirAll(own, 0o755)
	os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("user"), 0o644)
	targets := []Target{{Host: "claude", Dir: dir}}
	res, _ := Install(fakeSkills(), targets, Options{})
	if res[0].Skill != "huddle" || res[0].Action != "skipped" {
		t.Fatalf("%+v", res)
	}
	res, _ = Uninstall(fakeSkills(), targets)
	if b, _ := os.ReadFile(filepath.Join(own, "SKILL.md")); string(b) != "user" {
		t.Fatal("uninstall removed unowned dir")
	}
	if _, err := Install(fakeSkills(), targets, Options{Force: true}); err != nil || !Owned(own) {
		t.Fatal("force did not replace", err)
	}
}

func TestLegacySymlinkPruned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(repo, "install.sh"), []byte("#!/bin/sh"), 0o755)
	skills := filepath.Join(dir, "skills")
	os.MkdirAll(skills, 0o755)
	os.Symlink(repo, filepath.Join(skills, "huddle"))
	res, err := Uninstall(fakeSkills(), []Target{{Host: "claude", Dir: skills}})
	if err != nil || res[0].Action != "pruned-legacy" {
		t.Fatal(res, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "SKILL.md")); err != nil {
		t.Fatal("pruning touched the repo")
	}
	os.Symlink(repo, filepath.Join(skills, "huddle"))
	res, err = Install(fakeSkills(), []Target{{Host: "claude", Dir: skills}}, Options{})
	if err != nil || res[0].Action != "pruned-legacy" || !strings.Contains(res[0].Note, repo) {
		t.Fatal(res, err)
	}
	if st, _ := os.Lstat(filepath.Join(skills, "huddle")); st.Mode()&os.ModeSymlink != 0 || !Owned(filepath.Join(skills, "huddle")) {
		t.Fatal("symlink not replaced by owned dir")
	}
	if _, err := os.Stat(filepath.Join(repo, "install.sh")); err != nil {
		t.Fatal("repo damaged")
	}
}

func TestDefaultTargetsEnv(t *testing.T) {
	t.Setenv("CLAUDE_SKILLS_DIR", "/tmp/c")
	t.Setenv("AGENTS_SKILLS_DIR", "/tmp/a")
	ts, err := DefaultTargets(true)
	if err != nil || len(ts) != 2 || ts[0].Dir != "/tmp/c" || ts[1].Dir != "/tmp/a" {
		t.Fatal(ts, err)
	}
	t.Setenv("PATH", t.TempDir())
	if ts, _ := DefaultTargets(false); len(ts) != 1 {
		t.Fatal("agents target without codex or --agents", ts)
	}
}

// A relocated CLAUDE_CONFIG_DIR is the root Claude really reads. Installing
// into ~/.claude/skills instead left the old symlink there resolving the
// `huddle` name to a pre-CLI skill, so the new one was never loaded.
func TestDefaultTargetsHonoursClaudeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_SKILLS_DIR", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "/tmp/cys")
	ts, err := DefaultTargets(false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/tmp/cys", "skills"); ts[0].Dir != want {
		t.Fatalf("claude root = %q, want %q", ts[0].Dir, want)
	}

	// An explicit CLAUDE_SKILLS_DIR still wins over the config dir.
	t.Setenv("CLAUDE_SKILLS_DIR", "/tmp/explicit")
	ts, err = DefaultTargets(false)
	if err != nil || ts[0].Dir != "/tmp/explicit" {
		t.Fatal(ts, err)
	}

	// With neither set, the default home root is used.
	t.Setenv("CLAUDE_SKILLS_DIR", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	ts, err = DefaultTargets(false)
	if err != nil || ts[0].Dir != filepath.Join(home, ".claude", "skills") {
		t.Fatal(ts, err)
	}
}

func TestEmbeddedSkillsInstall(t *testing.T) {
	names, err := SkillNames(Skills())
	if err != nil {
		t.Fatal(err)
	}
	want := "huddle,huddle-personas,huddle-sdlc"
	if strings.Join(names, ",") != want {
		t.Fatalf("embedded skills = %v, want %s", names, want)
	}
	if _, err := Install(Skills(), []Target{{Host: "claude", Dir: t.TempDir()}}, Options{}); err != nil {
		t.Fatal(err)
	}
	_ = fs.ValidPath
}
