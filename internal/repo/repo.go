// Package repo identifies the repository (or plain folder) huddle runs in,
// reading .git files directly instead of spawning git — this is on the hot path.
package repo

import (
	"os"
	"path/filepath"
	"strings"
)

// Info identifies a working tree.
type Info struct {
	Root   string `json:"root"`
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Head   string `json:"head,omitempty"`
	IsGit  bool   `json:"isGit"`
}

// Identify walks up from dir to the nearest .git (dir or worktree file).
// Outside git it returns the folder itself as a local-mode repo.
func Identify(dir string) (Info, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Info{}, err
	}
	for cur := abs; ; cur = filepath.Dir(cur) {
		gitPath := filepath.Join(cur, ".git")
		if st, err := os.Stat(gitPath); err == nil {
			gitDir := gitPath
			name := filepath.Base(cur)
			if !st.IsDir() {
				gitDir = readGitdirFile(gitPath, cur)
				if main := mainRepoRoot(gitDir); main != "" {
					name = filepath.Base(main)
				}
			}
			branch, head := readHead(gitDir)
			return Info{Root: cur, Name: name, Branch: branch, Head: head, IsGit: true}, nil
		}
		if parent := filepath.Dir(cur); parent == cur {
			break
		}
	}
	return Info{Root: abs, Name: filepath.Base(abs), Branch: "local", IsGit: false}, nil
}

func readGitdirFile(path, base string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return path
	}
	line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
	if !filepath.IsAbs(line) {
		line = filepath.Join(base, line)
	}
	return line
}

// mainRepoRoot maps <main>/.git/worktrees/<name> back to <main>, so every
// worktree of a repo shares one identity.
func mainRepoRoot(gitDir string) string {
	parent := filepath.Dir(gitDir)
	if filepath.Base(parent) != "worktrees" {
		return ""
	}
	dotGit := filepath.Dir(parent)
	if filepath.Base(dotGit) != ".git" {
		return ""
	}
	return filepath.Dir(dotGit)
}

func readHead(gitDir string) (branch, head string) {
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "unknown", ""
	}
	s := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(s, "ref: "); ok {
		return strings.TrimPrefix(ref, "refs/heads/"), ""
	}
	if len(s) >= 12 {
		return "detached", s
	}
	return "unknown", s
}
