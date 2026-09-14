package gates

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/repo"
)

func TestDeciderRefusesAgents(t *testing.T) {
	cases := []struct {
		id   Identity
		want string
		err  bool
	}{
		{Identity{Agent: "HUDDLE_AGENT", As: "bob", Interactive: true}, "", true},
		{Identity{Interactive: false, User: "bob"}, "", true},
		{Identity{Interactive: false, As: "bob"}, "bob", false},
		{Identity{Interactive: true, User: "bob"}, "bob", false},
		{Identity{Interactive: true, User: "unknown"}, "", true},
	}
	for _, c := range cases {
		got, err := Decider(c.id)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%+v: got %q, %v", c.id, got, err)
		}
	}
	if _, err := Decider(Identity{Agent: "CLAUDECODE"}); !errors.Is(err, ErrAgent) {
		t.Error("agent error not ErrAgent")
	}
}

func TestAgentSessionEachMarker(t *testing.T) {
	if got := AgentSession(func(string) string { return "" }); got != "" {
		t.Fatalf("empty env detected as agent %q", got)
	}
	for _, name := range AgentEnvVars {
		t.Run(name, func(t *testing.T) {
			env := map[string]string{name: "1", "HUDDLE_UNRELATED": "x"}
			got := AgentSession(func(k string) string { return env[k] })
			if got != name {
				t.Fatalf("got %q", got)
			}
			_, err := Decider(Identity{Agent: got, As: "bob", Interactive: true})
			if !errors.Is(err, ErrAgent) || !strings.Contains(err.Error(), "gates are approved by a human in a terminal: run `huddle gate approve <id>` yourself") {
				t.Fatalf("%s set with --as must be refused, got %v", name, err)
			}
		})
	}
}

func TestGitFingerprintMovesWithTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	f := filepath.Join(root, "a.txt")
	os.WriteFile(f, []byte("one\n"), 0o644)
	git("add", ".")
	git("commit", "-qm", "init")
	info, _ := repo.Identify(root)
	fp1, mode, err := Fingerprint(root, info, config.Config{})
	if err != nil || mode != "git" {
		t.Fatalf("%v %s", err, mode)
	}
	if HeadCommit(root) == "" {
		t.Error("HeadCommit empty in a real repo")
	}
	os.WriteFile(f, []byte("two\n"), 0o644)
	fp2, _, _ := Fingerprint(root, info, config.Config{})
	if fp1 == fp2 {
		t.Fatal("fingerprint did not change with a working tree edit")
	}
}
