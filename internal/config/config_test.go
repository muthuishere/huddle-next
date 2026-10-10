package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muthuishere/huddle-next/internal/paths"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMerge(t *testing.T) {
	cases := []struct {
		name      string
		global    string
		repo      string
		wantUser  string
		wantOwner map[string]string // role -> first person
		wantHooks []string          // "scope:on"
		wantExtra []string
	}{
		{
			name:     "missing files are empty",
			wantUser: "",
		},
		{
			name:      "repo overrides global key by key, hooks concatenate with scope",
			global:    `{"user":"ada","owners":{"product":["pat"],"tech-lead":["tom"]},"hooks":[{"on":"gate.request","run":"g"}],"theme":"dark"}`,
			repo:      `{"user":"grace","owners":{"tech-lead":["tia"]},"hooks":[{"on":"session.start","run":"r"}],"extra":1}`,
			wantUser:  "grace",
			wantOwner: map[string]string{"product": "pat", "tech-lead": "tia"},
			wantHooks: []string{"global:gate.request", "repo:session.start"},
			wantExtra: []string{"theme", "extra"},
		},
		{
			name:      "global only",
			global:    `{"user":"ada","artifacts":{"spec":"docs/specs"}}`,
			wantUser:  "ada",
			wantOwner: map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HUDDLE_HOME", t.TempDir())
			root := t.TempDir()
			if tc.global != "" {
				write(t, paths.GlobalConfig(), tc.global)
			}
			if tc.repo != "" {
				write(t, paths.RepoConfig(root), tc.repo)
			}
			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.User != tc.wantUser {
				t.Errorf("user = %q, want %q", cfg.User, tc.wantUser)
			}
			for role, who := range tc.wantOwner {
				if got := cfg.Owners[role]; len(got) == 0 || got[0] != who {
					t.Errorf("owners[%s] = %v, want %s", role, got, who)
				}
			}
			if len(cfg.Hooks) != len(tc.wantHooks) {
				t.Fatalf("hooks = %+v, want %v", cfg.Hooks, tc.wantHooks)
			}
			for i, h := range cfg.Hooks {
				if h.Scope+":"+h.On != tc.wantHooks[i] {
					t.Errorf("hook %d = %s:%s, want %s", i, h.Scope, h.On, tc.wantHooks[i])
				}
			}
			for _, k := range tc.wantExtra {
				if _, ok := cfg.Extra[k]; !ok {
					t.Errorf("extra key %q lost", k)
				}
			}
		})
	}
}

func TestSetPreservesKeysAndGet(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	root := t.TempDir()
	write(t, paths.RepoConfig(root), `{"custom":{"keep":true}}`)
	if err := Set(Repo, root, "owners.code", `["sam"]`); err != nil {
		t.Fatal(err)
	}
	if err := Set(Global, root, "user", "plain string"); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := Get(root, "custom.keep"); err != nil || !ok || string(v) != "true" {
		t.Fatalf("custom.keep = %s %v %v", v, ok, err)
	}
	if v, ok, _ := Get(root, "user"); !ok || string(v) != `"plain string"` {
		t.Fatalf("user = %s", v)
	}
	cfg, _ := Load(root)
	if cfg.Owners["code"][0] != "sam" {
		t.Fatalf("owners = %v", cfg.Owners)
	}
}

func TestCorruptConfigNamesFile(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	root := t.TempDir()
	write(t, paths.RepoConfig(root), `{not json`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected error for corrupt config")
	}
}
