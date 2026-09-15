// Package paths resolves every location huddle reads or writes outside a repo.
// Nothing else in the module may build these paths by hand.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// Global returns the global root: $HUDDLE_HOME, else ~/.config/.huddle.
func Global() string {
	if v := os.Getenv("HUDDLE_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", ".huddle")
	}
	return filepath.Join(home, ".config", ".huddle")
}

// LegacyRoots returns deprecated roots, newest first, for read-only migration.
func LegacyRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".config", "muthuishere-agent-skills"),
		filepath.Join(home, "config", "muthuishere-agent-skills"),
	}
}

func GlobalConfig() string   { return filepath.Join(Global(), "config.json") }
func GlobalPersonas() string { return filepath.Join(Global(), "personas") }
func Cache() string          { return filepath.Join(Global(), "cache") }

// Migrated records what `huddle migrate` already copied from legacy roots.
func Migrated() string { return filepath.Join(Global(), "migrated.json") }
func Evidence(id string) string {
	return filepath.Join(Global(), "evidence", SafeName(id))
}

// RepoState is the private per-repo state dir: <global>/repos/<name>.
func RepoState(repoName string) string {
	return filepath.Join(Global(), "repos", SafeName(repoName))
}

// Events is the append-only audit log for a repo.
func Events(repoName string) string { return filepath.Join(RepoState(repoName), "events.jsonl") }

// HooksSeen stores the hash of the repo hooks last reviewed via `huddle hooks list`.
func HooksSeen(repoName string) string { return filepath.Join(RepoState(repoName), "hooks-seen.json") }

// Drafts holds subagent drafts awaiting human review.
func Drafts(repoName string) string { return filepath.Join(RepoState(repoName), "drafts") }

// RepoDir is the team-visible, committed huddle dir inside a repo.
func RepoDir(root string) string      { return filepath.Join(root, ".huddle") }
func RepoConfig(root string) string   { return filepath.Join(RepoDir(root), "config.json") }
func RepoPersonas(root string) string { return filepath.Join(RepoDir(root), "personas") }

// SafeName makes a string safe as one path segment (branch names, repo names, ids).
func SafeName(s string) string {
	s = strings.TrimLeft(strings.ReplaceAll(s, "/", "-"), ".")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, s)
	if s == "" {
		return "unknown"
	}
	return s
}

// Usage is the local-only CLI usage log (ADR 20); it rotates to Usage()+".1".
func Usage() string { return filepath.Join(Global(), "usage.jsonl") }
