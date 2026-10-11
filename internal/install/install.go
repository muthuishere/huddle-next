// Package install writes the embedded huddle skill family into the skill
// directories agent harnesses read (ADR 1 §4–5, ADR 19 row 1). Each skill is
// copied to a temp sibling and renamed into place, so a reader never sees a
// half-written skill. Every installed dir carries a .huddle-owned marker;
// uninstall removes only marked dirs.
package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/version"
	"github.com/muthuishere/huddle-next/skills"
)

// Marker is the ownership file written into every installed skill dir.
const Marker = ".huddle-owned"

// LegacyName is the skill dir the old install.sh symlinked to a repo checkout.
const LegacyName = "huddle"

// Target is one skills root, such as ~/.claude/skills.
type Target struct {
	Host string `json:"host"`
	Dir  string `json:"dir"`
}

// Result reports one action at one target.
// Action: installed, updated, removed, not-installed, pruned-legacy, skipped.
type Result struct {
	Host   string `json:"host"`
	Skill  string `json:"skill"`
	Path   string `json:"path"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

// MarkerInfo is the marker's JSON content.
type MarkerInfo struct {
	Owner       string    `json:"owner"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installedAt"`
}

// Options tune Install.
type Options struct {
	// Force replaces an existing skill dir that huddle does not own.
	Force bool
}

// Skills returns the embedded skill tree rooted at the skill dirs.
func Skills() fs.FS {
	return skills.FS
}

// SkillNames lists the embedded skills (dirs holding SKILL.md), sorted.
func SkillNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := fs.Stat(fsys, path.Join(e.Name(), "SKILL.md")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// DefaultTargets returns Claude's skills root always, and ~/.agents/skills
// ($AGENTS_SKILLS_DIR) when codex is on PATH or includeAgents.
//
// Claude's root is $CLAUDE_SKILLS_DIR, else $CLAUDE_CONFIG_DIR/skills, else
// ~/.claude/skills. Honouring CLAUDE_CONFIG_DIR matters: a user who relocates
// it reads skills only from there, so installing into ~/.claude/skills would
// leave the name resolving to whatever was already in the real root (ADR 8
// already reads relocated stores; the installer has to agree).
func DefaultTargets(includeAgents bool) ([]Target, error) {
	claude, err := claudeSkillsDir()
	if err != nil {
		return nil, err
	}
	targets := []Target{{Host: "claude", Dir: claude}}
	if !includeAgents {
		if _, err := exec.LookPath("codex"); err == nil {
			includeAgents = true
		}
	}
	if includeAgents {
		agents, err := rootDir("AGENTS_SKILLS_DIR", ".agents", "skills")
		if err != nil {
			return nil, err
		}
		targets = append(targets, Target{Host: "agents", Dir: agents})
	}
	return targets, nil
}

// Install copies every skill in fsys into every target.
func Install(fsys fs.FS, targets []Target, opts Options) ([]Result, error) {
	names, err := SkillNames(fsys)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("no embedded skills found — rebuild huddle")
	}
	var results []Result
	for _, t := range targets {
		for _, name := range names {
			dst := filepath.Join(t.Dir, name)
			r := Result{Host: t.Host, Skill: name, Path: dst}
			action, note, err := installAt(fsys, name, dst, opts)
			if err != nil {
				return results, fmt.Errorf("%s: install %s: %w", t.Host, name, err)
			}
			r.Action, r.Note = action, note
			results = append(results, r)
		}
	}
	return results, nil
}

// Uninstall removes huddle-owned skill dirs (and a legacy repo symlink) from targets.
// Dirs without the marker are left alone and reported as skipped.
func Uninstall(fsys fs.FS, targets []Target) ([]Result, error) {
	names, err := SkillNames(fsys)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, t := range targets {
		for _, name := range names {
			dst := filepath.Join(t.Dir, name)
			r := Result{Host: t.Host, Skill: name, Path: dst}
			action, note, err := uninstallVerdict(dst, name)
			if err != nil {
				return results, err
			}
			switch action {
			case "pruned-legacy":
				if err := os.Remove(dst); err != nil {
					return results, err
				}
			case "removed":
				if err := os.RemoveAll(dst); err != nil {
					return results, err
				}
			}
			r.Action, r.Note = action, note
			results = append(results, r)
		}
	}
	return results, nil
}

// uninstallVerdict decides what Uninstall does about one destination without
// touching it: removed, pruned-legacy, not-installed, or skipped. Uninstall and
// `--dry-run` share this one decision.
func uninstallVerdict(dst, name string) (action, note string, err error) {
	st, lerr := os.Lstat(dst)
	switch {
	case errors.Is(lerr, os.ErrNotExist):
		return "not-installed", "", nil
	case lerr != nil:
		return "", "", lerr
	case st.Mode()&os.ModeSymlink != 0:
		if name == LegacyName && isLegacyRepoLink(dst) {
			return "pruned-legacy", "old install.sh symlink to " + linkTarget(dst), nil
		}
		return "skipped", "symlink not created by huddle", nil
	case Owned(dst):
		return "removed", "", nil
	}
	return "skipped", "no " + Marker + " marker — not removed", nil
}

// Owned reports whether dir carries the huddle marker.
func Owned(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, Marker))
	return err == nil
}

// ReadMarker returns the marker in dir.
func ReadMarker(dir string) (MarkerInfo, error) {
	var m MarkerInfo
	b, err := os.ReadFile(filepath.Join(dir, Marker))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

func installAt(fsys fs.FS, name, dst string, opts Options) (action, note string, err error) {
	action = "installed"
	if st, lerr := os.Lstat(dst); lerr == nil {
		action = "updated"
		switch {
		case st.Mode()&os.ModeSymlink != 0:
			if name == LegacyName && isLegacyRepoLink(dst) {
				note = "replaced old install.sh symlink to " + linkTarget(dst)
				action = "pruned-legacy"
			} else if !opts.Force {
				return "skipped", "symlink not created by huddle — rerun with --force to replace", nil
			}
		case !Owned(dst) && !opts.Force:
			return "skipped", "existing dir has no " + Marker + " marker — rerun with --force to replace", nil
		}
	} else if !errors.Is(lerr, os.ErrNotExist) {
		return "", "", lerr
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp(parent, "."+name+".tmp-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmp) // no-op after a successful rename
	if err := copyTree(fsys, name, tmp); err != nil {
		return "", "", err
	}
	marker, _ := json.MarshalIndent(MarkerInfo{Owner: "huddle", Version: version.Version, InstalledAt: time.Now().UTC()}, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, Marker), append(marker, '\n'), 0o644); err != nil {
		return "", "", err
	}
	// Move the old copy aside first so a failed rename can be rolled back.
	var backup string
	if _, err := os.Lstat(dst); err == nil {
		backup = filepath.Join(parent, "."+name+".old-"+fmt.Sprint(time.Now().UnixNano()))
		if err := os.Rename(dst, backup); err != nil {
			return "", "", fmt.Errorf("move aside %s: %w", dst, err)
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		if backup != "" {
			_ = os.Rename(backup, dst)
		}
		return "", "", fmt.Errorf("activate %s: %w", dst, err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup) // RemoveAll on a symlink removes only the link
	}
	return action, note, nil
}

func copyTree(fsys fs.FS, root, dst string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		if path.Base(rel) == ".keep" {
			return nil
		}
		out := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

// isLegacyRepoLink reports whether link points at a huddle repo checkout
// (what the retired install.sh created).
func isLegacyRepoLink(link string) bool {
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return false
	}
	for _, marker := range []string{"install.sh", filepath.Join("references", "activation-routing.xml"), ".git"} {
		if _, err := os.Stat(filepath.Join(target, marker)); err == nil {
			if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err == nil {
				return true
			}
		}
	}
	return false
}

func linkTarget(link string) string {
	t, err := os.Readlink(link)
	if err != nil {
		return "?"
	}
	return t
}

// claudeSkillsDir resolves the skills root Claude actually reads.
func claudeSkillsDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_SKILLS_DIR")); dir != "" {
		return dir, nil
	}
	if cfg := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); cfg != "" {
		return filepath.Join(cfg, "skills"), nil
	}
	return rootDir("", ".claude", "skills")
}

// rootDir returns $envKey when set, else ~/ joined with fallback. An empty
// envKey skips the environment entirely.
func rootDir(envKey string, fallback ...string) (string, error) {
	if envKey != "" {
		if dir := strings.TrimSpace(os.Getenv(envKey)); dir != "" {
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for %s: %w", filepath.Join(fallback...), err)
	}
	return filepath.Join(append([]string{home}, fallback...)...), nil
}
