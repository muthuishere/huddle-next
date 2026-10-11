// Automatic skill sync. The skill family is the product; a binary without it
// installed is a half-finished install. Ensure runs on the huddle start path so
// every distribution channel converges on the same state — npm, brew, apt, dnf,
// winget, go install or a downloaded tarball — without each needing its own
// post-install hook. apt and dnf run as root and winget portables have no hook
// at all, so hooks alone could never cover them.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/version"
)

// EnvNoSync disables automatic skill sync when set to 1/true/yes/on.
const EnvNoSync = "HUDDLE_NO_SKILL_SYNC"

// SyncDisabled reports whether automatic sync is switched off, by env var or by
// a top-level "skillSync": false in repo or global config (repo wins).
func SyncDisabled(repoRoot string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvNoSync))) {
	case "1", "true", "yes", "on":
		return true
	}
	if repoRoot != "" {
		if v, ok := skillSyncKey(paths.RepoConfig(repoRoot)); ok {
			return !v
		}
	}
	if v, ok := skillSyncKey(paths.GlobalConfig()); ok {
		return !v
	}
	return false
}

func skillSyncKey(path string) (bool, bool) {
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte(`"skillSync"`)) {
		return false, false
	}
	var m struct {
		SkillSync *bool `json:"skillSync"`
	}
	if json.Unmarshal(b, &m) != nil || m.SkillSync == nil {
		return false, false
	}
	return *m.SkillSync, true
}

// Ensure brings the installed skills into step with this binary and reports only
// what it changed or could not change.
//
// It writes when the target is absent, when huddle owns a stale copy, and when
// the target is the legacy install.sh symlink into a skill repo — that last case
// is the upgrade: the old single-skill checkout is replaced by the new family.
// Replacing the symlink removes a link, never the repo behind it, and never any
// state: memories, personas and the event log live under the config roots, which
// install never touches and which the caller migrates by copying.
//
// Left strictly alone and reported: a dir with no marker, and a symlink huddle
// did not create and that does not point at a skill repo.
func Ensure(fsys fs.FS, targets []Target) ([]Result, error) {
	names, err := SkillNames(fsys)
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, t := range targets {
		for _, name := range names {
			dst := filepath.Join(t.Dir, name)
			act, note, write := ensureVerdict(dst, name)
			if !write {
				if act != "" {
					out = append(out, Result{Host: t.Host, Skill: name, Path: dst, Action: act, Note: note})
				}
				continue
			}
			action, n, err := installAt(fsys, name, dst, Options{})
			if err != nil {
				// A sync failure must never fail the caller's real work.
				out = append(out, Result{Host: t.Host, Skill: name, Path: dst, Action: "skipped", Note: err.Error()})
				continue
			}
			out = append(out, Result{Host: t.Host, Skill: name, Path: dst, Action: action, Note: n})
		}
	}
	return out, nil
}

// Plan reports what Install would do at every destination without writing
// anything (`huddle install --skills --dry-run`). It asks ensureVerdict for the
// verdict on each destination — the same decision the real write makes — and
// only translates it into the action installAt would report.
func Plan(fsys fs.FS, targets []Target, opts Options) ([]Result, error) {
	names, err := SkillNames(fsys)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("no embedded skills found — rebuild huddle")
	}
	var out []Result
	for _, t := range targets {
		for _, name := range names {
			dst := filepath.Join(t.Dir, name)
			r := Result{Host: t.Host, Skill: name, Path: dst}
			r.Action, r.Note = planVerdict(dst, name, opts)
			out = append(out, r)
		}
	}
	return out, nil
}

// planVerdict maps ensureVerdict's decision onto the action `install` reports.
// Install rewrites a destination it owns even when the marker is current, so an
// owned dir is always "updated"; only a foreign dir or symlink is skipped, and
// --force turns that into a replacement.
func planVerdict(dst, name string, opts Options) (action, note string) {
	act, note, _ := ensureVerdict(dst, name)
	if act == "skipped" {
		if !opts.Force {
			return "skipped", strings.Replace(note, "replace it with: huddle install --skills --force", "rerun with --force to replace", 1)
		}
		return "updated", "would replace a destination huddle does not own (--force)"
	}
	st, err := os.Lstat(dst)
	switch {
	case os.IsNotExist(err):
		return "installed", ""
	case err != nil:
		return "skipped", err.Error()
	case st.Mode()&os.ModeSymlink != 0: // ensureVerdict only lets the legacy link through
		return "pruned-legacy", "would replace old install.sh symlink to " + linkTarget(dst)
	}
	return "updated", note
}

// PlanUninstall reports what Uninstall would do, without removing anything.
func PlanUninstall(fsys fs.FS, targets []Target) ([]Result, error) {
	names, err := SkillNames(fsys)
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, t := range targets {
		for _, name := range names {
			dst := filepath.Join(t.Dir, name)
			r := Result{Host: t.Host, Skill: name, Path: dst}
			r.Action, r.Note, _ = uninstallVerdict(dst, name)
			out = append(out, r)
		}
	}
	return out, nil
}

// ensureVerdict decides what to do about one destination without touching it.
// A returned empty action means "current, say nothing".
func ensureVerdict(dst, name string) (action, note string, write bool) {
	st, err := os.Lstat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", true // absent: install it
		}
		return "skipped", err.Error(), false
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if name == LegacyName && isLegacyRepoLink(dst) {
			// The upgrade path: installAt reports this as pruned-legacy.
			return "", "", true
		}
		return "skipped", "symlink not created by huddle — replace it with: huddle install --skills --force", false
	}
	if !Owned(dst) {
		return "skipped", "existing dir has no " + Marker + " marker — replace it with: huddle install --skills --force", false
	}
	m, err := ReadMarker(dst)
	if err != nil {
		return "", "", true // unreadable marker on an owned dir: refresh it
	}
	if m.Version == version.Version {
		return "", "", false // up to date
	}
	return "", "", true // version drift: refresh
}
