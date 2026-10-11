package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guardrails block in this repo's CLAUDE.md promises every write path takes
// --dry-run; install used to answer "unknown flag".
func TestInstallDryRunWritesNothing(t *testing.T) {
	root := coreEnv(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_SKILLS_DIR", filepath.Join(dir, "claude"))
	t.Setenv("AGENTS_SKILLS_DIR", filepath.Join(dir, "agents"))
	t.Setenv("PATH", t.TempDir()) // no codex

	out, err := runCore(t, root, "install", "--skills", "--dry-run")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"DRY RUN", "installed", "huddle-sdlc"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Restart your agent session") {
		t.Errorf("dry run claims the skills are installed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "claude")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote to the skills root: %v", err)
	}

	out = mustRunCore(t, root, "install", "--skills", "--dry-run", "--json")
	var doc struct {
		DryRun  bool `json:"dryRun"`
		Results []struct {
			Host, Skill, Action, Path string
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !doc.DryRun || len(doc.Results) != 3 {
		t.Fatalf("%+v", doc)
	}
	for _, r := range doc.Results {
		if r.Host != "claude" || r.Action != "installed" {
			t.Errorf("%+v", r)
		}
	}

	// After a real install the plan reports updates, still without writing.
	mustRunCore(t, root, "install", "--skills")
	marker := filepath.Join(dir, "claude", "huddle-sdlc", ".huddle-owned")
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if out = mustRunCore(t, root, "install", "--skills", "--dry-run"); !strings.Contains(out, "updated") {
		t.Errorf("second plan:\n%s", out)
	}
	if out = mustRunCore(t, root, "uninstall", "--skills", "--dry-run"); !strings.Contains(out, "removed") {
		t.Errorf("uninstall plan:\n%s", out)
	}
	after, err := os.ReadFile(marker)
	if err != nil || string(after) != string(before) {
		t.Fatalf("a dry run changed the installed copy: %v", err)
	}
}

func mustRunCore(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := runCore(t, root, args...)
	if err != nil {
		t.Fatalf("huddle %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}
