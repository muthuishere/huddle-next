package install

import (
	"os"
	"path/filepath"
	"testing"
)

// byPath indexes results so a case can assert on one destination.
func byPath(res []Result) map[string]Result {
	out := make(map[string]Result, len(res))
	for _, r := range res {
		out[r.Path] = r
	}
	return out
}

// TestPlanReportsWithoutWriting covers `huddle install --skills --dry-run`:
// every verdict is reported and the filesystem is left exactly as it was.
func TestPlanReportsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	targets := []Target{{Host: "claude", Dir: filepath.Join(dir, "claude")}}
	fsys := fakeSkills()

	// absent everywhere
	res, err := Plan(fsys, targets, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("%+v", res)
	}
	for _, r := range res {
		if r.Action != "installed" {
			t.Errorf("%s: %s", r.Skill, r.Action)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "claude")); !os.IsNotExist(err) {
		t.Fatal("dry run created the target dir")
	}

	// a huddle-owned copy, a foreign dir
	if _, err := Install(fsys, targets, Options{}); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "claude", "huddle-personas")
	if err := os.RemoveAll(foreign); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, foreign)
	mustWrite(t, filepath.Join(foreign, "SKILL.md"), "someone else's skill")

	res, err = Plan(fsys, targets, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(res)
	if r := got[filepath.Join(dir, "claude", "huddle")]; r.Action != "updated" {
		t.Errorf("owned copy: %+v", r)
	}
	if r := got[foreign]; r.Action != "skipped" || r.Note == "" {
		t.Errorf("foreign dir: %+v", r)
	} else if want := "rerun with --force to replace"; r.Note[len(r.Note)-len(want):] != want {
		t.Errorf("note should tell the user how to proceed: %q", r.Note)
	}
	if res, err = Plan(fsys, targets, Options{Force: true}); err != nil {
		t.Fatal(err)
	} else if r := byPath(res)[foreign]; r.Action != "updated" {
		t.Errorf("--force: %+v", r)
	}
	if body, err := os.ReadFile(filepath.Join(foreign, "SKILL.md")); err != nil || string(body) != "someone else's skill" {
		t.Fatalf("dry run overwrote a foreign skill: %q %v", body, err)
	}

	// the legacy install.sh symlink into a repo checkout
	repo := filepath.Join(dir, "huddle-repo")
	mustMkdir(t, repo)
	mustWrite(t, filepath.Join(repo, "SKILL.md"), "legacy")
	mustWrite(t, filepath.Join(repo, "install.sh"), "#!/bin/sh")
	legacy := filepath.Join(dir, "claude", LegacyName)
	if err := os.RemoveAll(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, legacy); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res, err = Plan(fsys, targets, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r := byPath(res)[legacy]; r.Action != "pruned-legacy" {
		t.Errorf("legacy symlink: %+v", r)
	}
	if st, err := os.Lstat(legacy); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Error("dry run replaced the legacy symlink")
	}
}

func TestPlanUninstallReportsWithoutRemoving(t *testing.T) {
	dir := t.TempDir()
	targets := []Target{{Host: "claude", Dir: filepath.Join(dir, "claude")}}
	fsys := fakeSkills()

	res, err := PlanUninstall(fsys, targets)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Action != "not-installed" {
			t.Errorf("%s: %s", r.Skill, r.Action)
		}
	}
	if _, err := Install(fsys, targets, Options{}); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "claude", "huddle-sdlc")
	if err := os.RemoveAll(filepath.Join(foreign, Marker)); err != nil {
		t.Fatal(err)
	}
	res, err = PlanUninstall(fsys, targets)
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(res)
	if r := got[filepath.Join(dir, "claude", "huddle")]; r.Action != "removed" {
		t.Errorf("owned: %+v", r)
	}
	if r := got[foreign]; r.Action != "skipped" {
		t.Errorf("unmarked: %+v", r)
	}
	for _, name := range []string{"huddle", "huddle-sdlc", "huddle-personas"} {
		if _, err := os.Stat(filepath.Join(dir, "claude", name, "SKILL.md")); err != nil {
			t.Errorf("dry run removed %s: %v", name, err)
		}
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
