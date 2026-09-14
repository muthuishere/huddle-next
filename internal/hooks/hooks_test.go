package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/huddle/internal/config"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hook scripts are POSIX sh")
	}
}

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func cfgOf(hooks ...config.Hook) config.Config { return config.Config{Hooks: hooks} }

func TestNoMatchAllows(t *testing.T) {
	r := Fire(cfgOf(config.Hook{On: "gate.request", Run: "exit 1", Blocking: true}), t.TempDir(), "stage.enter", nil)
	if r.Decision != Allow || r.Ran != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestSelectMatchAndOrder(t *testing.T) {
	hooks := []config.Hook{
		{On: "e", Run: "repo1", Scope: "repo"},
		{On: "e", Run: "global1", Scope: "global", Match: map[string]string{"stage": "deploy"}},
		{On: "e", Run: "wrongstage", Scope: "global", Match: map[string]string{"stage": "plan"}},
		{On: "other", Run: "x", Scope: "global"},
		{On: "e", Run: "global2", Scope: "global"},
	}
	got := Select(hooks, "e", Payload{"stage": "deploy"})
	var names []string
	for _, h := range got {
		names = append(names, h.Run)
	}
	if strings.Join(names, ",") != "global1,global2,repo1" {
		t.Fatal(names)
	}
}

func TestStdinEnvAndOutput(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	s := script(t, dir, "h.sh", `in=$(cat)
case "$in" in *'"event":"artifact.written"'*) ;; *) echo "bad stdin $in" >&2; exit 3;; esac
printf '{"context":"saw %s in %s","links":[{"system":"jira","id":"PAY-1"}],"ingest":[{"text":"password=hunter2hunter2"}],"log":[{"type":"action","text":"t"}]}' "$HUDDLE_EVENT" "$(basename "$HUDDLE_REPO_ROOT")"`)
	r := Fire(cfgOf(config.Hook{On: "artifact.written", Run: s, Match: map[string]string{"kind": "intent"}}), dir, "artifact.written", Payload{"kind": "intent"})
	if r.Decision != Allow || r.Ran != 1 || len(r.Errors) != 0 {
		t.Fatalf("%+v", r)
	}
	if len(r.Context) != 1 || r.Context[0] != "saw artifact.written in "+filepath.Base(dir) {
		t.Fatalf("context %q", r.Context)
	}
	if len(r.Links) != 1 || r.Links[0].ID != "PAY-1" || len(r.Log) != 1 {
		t.Fatalf("%+v", r)
	}
	if txt := r.Ingest[0]["text"].(string); strings.Contains(txt, "hunter2") {
		t.Fatalf("ingest not redacted: %q", txt)
	}
}

func TestBlockingDeny(t *testing.T) {
	skipWindows(t)
	r := Fire(cfgOf(config.Hook{On: "gate.request", Run: `echo '{"decision":"deny","reason":"PR not approved"}'`, Blocking: true}), t.TempDir(), "gate.request", nil)
	if r.Decision != Deny || len(r.Reasons) != 1 || !strings.Contains(r.Reasons[0], "PR not approved") {
		t.Fatalf("%+v", r)
	}
}

func TestNonBlockingCannotDeny(t *testing.T) {
	skipWindows(t)
	r := Fire(cfgOf(config.Hook{On: "gate.request", Run: `echo '{"decision":"deny","reason":"nope"}'`}), t.TempDir(), "gate.request", nil)
	if r.Decision != Allow {
		t.Fatalf("%+v", r)
	}
	r = Fire(cfgOf(config.Hook{On: "gate.request", Run: `exit 7`}), t.TempDir(), "gate.request", nil)
	if r.Decision != Allow || len(r.Errors) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestBlockingFailureInvalidJSONTimeoutAsk(t *testing.T) {
	skipWindows(t)
	for name, h := range map[string]config.Hook{
		"exit":    {Run: "echo boom >&2; exit 1"},
		"invalid": {Run: "echo not json"},
		"timeout": {Run: "sleep 5", Timeout: "200ms"},
	} {
		t.Run(name, func(t *testing.T) {
			h.On, h.Blocking = "gate.request", true
			start := time.Now()
			r := Fire(cfgOf(h), t.TempDir(), "gate.request", nil)
			if r.Decision != Ask || len(r.Errors) != 1 {
				t.Fatalf("%+v", r)
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("timeout not enforced")
			}
		})
	}
}

func TestTimeoutKillsProcessGroup(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "survived")
	r := Fire(cfgOf(config.Hook{On: "e", Run: "(sleep 1; touch " + marker + ") & sleep 5", Timeout: "200ms", Blocking: true}), dir, "e", nil)
	if r.Decision != Ask {
		t.Fatalf("%+v", r)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("grandchild survived group kill")
	}
}

func TestDenyBeatsAsk(t *testing.T) {
	skipWindows(t)
	r := Fire(cfgOf(
		config.Hook{On: "e", Run: "exit 1", Blocking: true, Scope: "global"},
		config.Hook{On: "e", Run: `echo '{"decision":"deny"}'`, Blocking: true, Scope: "repo"},
		config.Hook{On: "e", Run: `echo '{"decision":"ask"}'`, Blocking: true, Scope: "repo"},
	), t.TempDir(), "e", nil)
	if r.Decision != Deny || r.Ran != 3 {
		t.Fatalf("%+v", r)
	}
}

func TestAsyncDoesNotWait(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "async.out")
	start := time.Now()
	r := Fire(cfgOf(config.Hook{On: "e", Run: "sleep 1; cat > " + out, Async: true, Blocking: true}), dir, "e", Payload{"x": "y"})
	if time.Since(start) > 700*time.Millisecond {
		t.Fatal("async hook was awaited")
	}
	if r.Decision != Allow || r.Ran != 1 {
		t.Fatalf("%+v", r)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(out); err == nil && strings.Contains(string(b), `"x":"y"`) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("async hook never received stdin")
}

func TestPromptHookNotExecuted(t *testing.T) {
	r := Fire(cfgOf(config.Hook{On: "e", Prompt: "create a Jira epic", Blocking: true}), t.TempDir(), "e", nil)
	if r.Ran != 0 || r.Decision != Allow || len(r.Context) != 1 || r.Context[0] != "agent hook: create a Jira epic" {
		t.Fatalf("%+v", r)
	}
}

func TestOutputRedacted(t *testing.T) {
	skipWindows(t)
	r := Fire(cfgOf(config.Hook{On: "e", Run: "echo token=abcdefgh12345678"}), t.TempDir(), "e", nil)
	if len(r.Context) != 1 || strings.Contains(r.Context[0], "abcdefgh12345678") {
		t.Fatalf("%+v", r)
	}
}

func TestTimeoutParse(t *testing.T) {
	for in, want := range map[string]time.Duration{"": DefaultTimeout, "20s": 20 * time.Second, "3": 3 * time.Second, "junk": DefaultTimeout} {
		if got := Timeout(config.Hook{Timeout: in}); got != want {
			t.Errorf("%q: %v", in, got)
		}
	}
}
