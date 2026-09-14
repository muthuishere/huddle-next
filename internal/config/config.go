// Package config loads huddle's two config levels — global
// (~/.config/.huddle/config.json) and repo (<repo>/.huddle/config.json) —
// and merges them key by key, repo winning.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/huddle/internal/paths"
)

// Hook is one lifecycle hook (ADR 17, 19). The user owns it; huddle just runs it.
type Hook struct {
	On       string            `json:"on"`
	Match    map[string]string `json:"match,omitempty"`
	Run      string            `json:"run,omitempty"`
	Prompt   string            `json:"prompt,omitempty"`
	Blocking bool              `json:"blocking,omitempty"`
	Timeout  string            `json:"timeout,omitempty"`
	Async    bool              `json:"async,omitempty"`
	Scope    string            `json:"-"` // "global" or "repo", set on load
}

// Config is the merged view. Unknown keys are preserved in Extra.
type Config struct {
	User      string                     `json:"user,omitempty"`
	Artifacts map[string]string          `json:"artifacts,omitempty"` // kind -> path pattern override
	Owners    map[string][]string        `json:"owners,omitempty"`    // role -> people
	Hooks     []Hook                     `json:"hooks,omitempty"`
	Extra     map[string]json.RawMessage `json:"-"`
}

// Scope names a config level.
type Scope string

const (
	Global Scope = "global"
	Repo   Scope = "repo"
)

// Path returns the config file for a scope.
func Path(scope Scope, repoRoot string) string {
	if scope == Repo {
		return paths.RepoConfig(repoRoot)
	}
	return paths.GlobalConfig()
}

// Load reads global then repo config and merges them. Missing files are empty.
func Load(repoRoot string) (Config, error) {
	g, err := readRaw(Path(Global, repoRoot))
	if err != nil {
		return Config{}, err
	}
	r, err := readRaw(Path(Repo, repoRoot))
	if err != nil {
		return Config{}, err
	}
	merged := map[string]json.RawMessage{}
	for k, v := range g {
		merged[k] = v
	}
	for k, v := range r {
		if k == "hooks" || k == "owners" || k == "artifacts" {
			continue // merged structurally below
		}
		merged[k] = v
	}
	var cfg Config
	if err := decodeInto(merged, &cfg); err != nil {
		return Config{}, err
	}
	var gc, rc Config
	_ = decodeInto(g, &gc)
	_ = decodeInto(r, &rc)
	cfg.Artifacts = mergeMap(gc.Artifacts, rc.Artifacts)
	cfg.Owners = mergeMap(gc.Owners, rc.Owners)
	cfg.Hooks = nil
	for _, h := range gc.Hooks {
		h.Scope = string(Global)
		cfg.Hooks = append(cfg.Hooks, h)
	}
	for _, h := range rc.Hooks {
		h.Scope = string(Repo)
		cfg.Hooks = append(cfg.Hooks, h)
	}
	cfg.Extra = map[string]json.RawMessage{}
	for k, v := range merged {
		switch k {
		case "user", "artifacts", "owners", "hooks":
		default:
			cfg.Extra[k] = v
		}
	}
	return cfg, nil
}

func mergeMap[V any](lo, hi map[string]V) map[string]V {
	if len(lo) == 0 && len(hi) == 0 {
		return nil
	}
	out := make(map[string]V, len(lo)+len(hi))
	for k, v := range lo {
		out[k] = v
	}
	for k, v := range hi {
		out[k] = v
	}
	return out
}

func readRaw(path string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w — fix it or run: huddle config show", path, err)
	}
	return m, nil
}

func decodeInto(m map[string]json.RawMessage, v any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Set writes one dotted key (e.g. "owners.tech-lead", "user") at a scope,
// preserving every other key in that file. value must be JSON or a bare string.
func Set(scope Scope, repoRoot, key, value string) error {
	path := Path(scope, repoRoot)
	raw, err := readRaw(path)
	if err != nil {
		return err
	}
	var tree map[string]any
	if err := decodeInto(raw, &tree); err != nil {
		return err
	}
	if tree == nil {
		tree = map[string]any{}
	}
	var val any
	if err := json.Unmarshal([]byte(value), &val); err != nil {
		val = value
	}
	parts := strings.Split(key, ".")
	node := tree
	for _, p := range parts[:len(parts)-1] {
		child, ok := node[p].(map[string]any)
		if !ok {
			child = map[string]any{}
			node[p] = child
		}
		node = child
	}
	node[parts[len(parts)-1]] = val
	return WriteJSON(path, tree)
}

// Get reads one dotted key from the merged config as JSON.
func Get(repoRoot, key string) (json.RawMessage, bool, error) {
	cfg, err := Load(repoRoot)
	if err != nil {
		return nil, false, err
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, false, err
	}
	var tree map[string]any
	_ = json.Unmarshal(b, &tree)
	for k, v := range cfg.Extra {
		var x any
		_ = json.Unmarshal(v, &x)
		tree[k] = x
	}
	var cur any = tree
	for _, p := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		if cur, ok = m[p]; !ok {
			return nil, false, nil
		}
	}
	out, err := json.Marshal(cur)
	return out, true, err
}

// WriteJSON writes v atomically (temp file + rename) with 0600 perms.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'))
}

// WriteFileAtomic writes data to path via a temp sibling and rename.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
