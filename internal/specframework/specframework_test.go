package specframework

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRepo makes a repo root; withFramework adds an openspec/ layout.
func fakeRepo(t *testing.T, withFramework bool) string {
	t.Helper()
	root := t.TempDir()
	if withFramework {
		if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Absence is the default and must be completely quiet: no detection, no
// client, no error, and a line that does not read as a defect.
func TestAbsentFrameworkIsNormal(t *testing.T) {
	root := fakeRepo(t, false)
	if d := Detect(root); d != nil {
		t.Fatalf("detected %+v in a repo with no framework", d)
	}
	line := Line(nil)
	if !strings.Contains(line, "native") {
		t.Errorf("absent line should point at the native layout: %q", line)
	}
	for _, bad := range []string{"missing", "not found", "error", "warn"} {
		if strings.Contains(strings.ToLower(line), bad) {
			t.Errorf("absent line reads as a defect (%q): %q", bad, line)
		}
	}
	var c *Client
	if c = New(root, nil); c != nil {
		t.Fatal("New with no detection must return nil")
	}
	changes, err := c.Changes(context.Background())
	if err != nil || changes != nil {
		t.Fatalf("nil client: %v %v", changes, err)
	}
}

// A directory without the marker file is not a framework; the marker makes it
// one, and a missing CLI still yields no client (nothing to read).
func TestDetect(t *testing.T) {
	root := fakeRepo(t, false)
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o700); err != nil {
		t.Fatal(err)
	}
	if d := Detect(root); d != nil {
		t.Fatalf("bare openspec/ dir with no config.yaml detected: %+v", d)
	}
	root = fakeRepo(t, true)
	d := Detect(root)
	if d == nil || d.Name != "openspec" || d.Dir != "openspec" {
		t.Fatalf("detection: %+v", d)
	}
	// PATH emptied: detected, but no CLI and therefore no client.
	t.Setenv("PATH", filepath.Join(root, "nowhere"))
	d = Detect(root)
	if d == nil || d.CLI != "" {
		t.Fatalf("CLI should be empty with an empty PATH: %+v", d)
	}
	if New(root, d) != nil {
		t.Fatal("no CLI must mean no client")
	}
	if l := Line(d); !strings.Contains(l, "not on PATH") || !strings.Contains(l, "native") {
		t.Errorf("line: %q", l)
	}
}

const listJSON = `{"changes":[{"name":"add-login"},{"name":"--oops"},{"name":"../escape"}],"root":{"path":"/x"}}`

func statusJSON(root string) string {
	// Slash form, always: root goes into a JSON string literal, and on Windows
	// its backslashes are JSON escapes — "D:\a\huddle-next" made the whole
	// document unparseable, Changes returned nothing, and the test read as a
	// product failure. Go's filepath accepts forward slashes on Windows.
	root = filepath.ToSlash(root)
	return `{"changeName":"add-login","changeRoot":"` + root + `/openspec/changes/add-login",
	"artifactPaths":{
	  "proposal":{"outputPath":"proposal.md","resolvedOutputPath":"` + root + `/openspec/changes/add-login/proposal.md","existingOutputPaths":["` + root + `/openspec/changes/add-login/proposal.md"]},
	  "specs":{"outputPath":"specs/**/*.md","resolvedOutputPath":"` + root + `/openspec/changes/add-login/specs/**/*.md","existingOutputPaths":[]},
	  "design":{"outputPath":"design.md","resolvedOutputPath":"` + root + `/openspec/changes/add-login/design.md","existingOutputPaths":[]},
	  "tasks":{"outputPath":"tasks.md","resolvedOutputPath":"/etc/passwd","existingOutputPaths":[]}},
	"isComplete":false}`
}

// The present case, with the CLI stubbed: huddle locates files and nothing
// more. Unsafe ids are dropped, paths outside the repo are dropped, and the
// huddle stage labels are descriptive only.
func TestChangesStubbed(t *testing.T) {
	root := fakeRepo(t, true)
	var calls [][]string
	c := &Client{Root: root, Det: Detection{Name: "openspec", Dir: "openspec", CLI: "/nonexistent/openspec"},
		Run: func(_ context.Context, _, dir string, args []string) ([]byte, error) {
			if dir != root {
				t.Errorf("cwd %q, want repo root %q", dir, root)
			}
			calls = append(calls, args)
			if args[0] == "list" {
				return []byte(listJSON), nil
			}
			return []byte(statusJSON(root)), nil
		}}
	changes, err := c.Changes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Join(calls[1], " ") != "status --change add-login --json" {
		t.Fatalf("calls: %v", calls)
	}
	if len(changes) != 1 || changes[0].ID != "add-login" || changes[0].Dir != "openspec/changes/add-login" {
		t.Fatalf("changes: %+v", changes)
	}
	got := map[string]Artifact{}
	for _, a := range changes[0].Artifacts {
		got[a.Kind] = a
	}
	if len(got) != 3 {
		t.Fatalf("want proposal/specs/design and no out-of-repo tasks: %+v", changes[0].Artifacts)
	}
	if a := got["proposal"]; a.Native != "intent" || !a.Exists || a.Path != "openspec/changes/add-login/proposal.md" {
		t.Errorf("proposal: %+v", a)
	}
	if a := got["specs"]; a.Native != "spec" || a.Exists {
		t.Errorf("specs: %+v", a)
	}
	if a := got["design"]; a.Native != "adr" {
		t.Errorf("design: %+v", a)
	}
	if _, ok := got["tasks"]; ok {
		t.Error("an absolute path outside the repo must be dropped")
	}
}

// Every failure mode of the external tool degrades to "no changes": huddle
// must never break because someone else's CLI did.
func TestChangesDegradeGracefully(t *testing.T) {
	root := fakeRepo(t, true)
	det := Detection{Name: "openspec", CLI: "/nonexistent/openspec"}
	cases := map[string]Runner{
		"exec fails":  func(context.Context, string, string, []string) ([]byte, error) { return nil, errors.New("boom") },
		"garbage":     func(context.Context, string, string, []string) ([]byte, error) { return []byte("not json"), nil },
		"empty":       func(context.Context, string, string, []string) ([]byte, error) { return []byte(`{"changes":[]}`), nil },
		"status dies": nil,
	}
	cases["status dies"] = func(_ context.Context, _, _ string, args []string) ([]byte, error) {
		if args[0] == "list" {
			return []byte(listJSON), nil
		}
		return nil, errors.New("status exploded")
	}
	for name, run := range cases {
		c := &Client{Root: root, Det: det, Run: run}
		changes, err := c.Changes(context.Background())
		if len(changes) != 0 {
			t.Errorf("%s: got changes %+v", name, changes)
		}
		if name == "exec fails" || name == "garbage" {
			if err == nil {
				t.Errorf("%s: want an error the caller may ignore", name)
			}
		} else if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// An unknown framework is never shelled out to.
	c := &Client{Root: root, Det: Detection{Name: "spec-kit", CLI: "/nonexistent/x"},
		Run: func(context.Context, string, string, []string) ([]byte, error) {
			t.Fatal("must not exec an unsupported framework")
			return nil, nil
		}}
	if _, err := c.Changes(context.Background()); !errors.Is(err, errUnsupported) {
		t.Errorf("unsupported: %v", err)
	}
}

// Output is data: control characters out, secrets masked, ids that could act
// as flags or paths rejected.
func TestUntrustedOutputIsSanitised(t *testing.T) {
	if got := clean("a\x00b\nc"); got != "a b  c" && !strings.Contains(got, "a b") {
		t.Errorf("control chars: %q", got)
	}
	fake := "password=" + strings.Repeat("z", 14)
	if got := clean(fake); strings.Contains(got, strings.Repeat("z", 14)) {
		t.Errorf("secret not masked: %q", got)
	}
	if got := clean(strings.Repeat("x", 900)); len(got) > 512 {
		t.Errorf("length %d", len(got))
	}
	for _, bad := range []string{"--json", "-x", "../up", "a/b", "", strings.Repeat("a", 65), "a b"} {
		if safeID(bad) {
			t.Errorf("safeID(%q) = true", bad)
		}
	}
	for _, ok := range []string{"add-login", "a", "v1.2_x"} {
		if !safeID(ok) {
			t.Errorf("safeID(%q) = false", ok)
		}
	}
}

// The real exec path, without needing openspec installed: a tiny script stands
// in for the CLI, proving timeout, cwd, bounded env and capped read work.
func TestExecRunnerAgainstScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stub")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "openspec")
	script := "#!/bin/sh\necho \"{\\\"changes\\\":[{\\\"name\\\":\\\"from-script\\\"}],\\\"cwd\\\":\\\"$PWD\\\",\\\"secret\\\":\\\"${MY_SECRET:-unset}\\\"}\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MY_SECRET", "must-not-leak")
	root := fakeRepo(t, true)
	out, err := execRunner(context.Background(), bin, root, []string{"list", "--json"})
	if err != nil {
		t.Fatal(err, string(out))
	}
	if !strings.Contains(string(out), "from-script") {
		t.Fatalf("stdout: %s", out)
	}
	if strings.Contains(string(out), "must-not-leak") {
		t.Error("the environment was handed to the external process")
	}
	c := &Client{Root: root, Det: Detection{Name: "openspec", CLI: bin}, Run: execRunner}
	// The stub answers every subcommand with the same payload, so the change is
	// listed and its (absent) artifact paths simply yield nothing.
	changes, err := c.Changes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].ID != "from-script" || len(changes[0].Artifacts) != 0 {
		t.Fatalf("changes: %+v", changes)
	}
}

// A repo root and the path an external tool prints can name the same directory
// by different routes: on macOS every mktemp dir lives under /var, a symlink to
// /private/var, and the tool resolves it while huddle does not. The containment
// guard then dropped every legitimate path and the user saw an empty list with
// no error. A genuine escape must still be dropped.
func TestRelAcceptsSymlinkedRootAndStillRejectsEscapes(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	inside := filepath.Join(real, "openspec", "changes", "add-login", "proposal.md")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("# x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Root reached by the symlink, path reported resolved: still inside.
	c := &Client{Root: link}
	if got := c.rel(inside); got != "openspec/changes/add-login/proposal.md" {
		t.Fatalf("resolved path under symlinked root = %q, want the relative path", got)
	}

	// And the reverse: root resolved, path via the symlink.
	c = &Client{Root: real}
	if got := c.rel(filepath.Join(link, "openspec", "changes", "add-login", "proposal.md")); got == "" {
		t.Fatal("path via a symlink to the root was dropped")
	}

	// An artifact the tool reports as *missing* has no file on disk, and
	// EvalSymlinks fails on a path that does not exist — resolving both sides
	// would drop exactly the entries the caller needs to be told about.
	c = &Client{Root: link}
	absent := filepath.Join(real, "openspec", "changes", "add-login", "tasks.md")
	if got := c.rel(absent); got != "openspec/changes/add-login/tasks.md" {
		t.Fatalf("missing artifact under a symlinked root = %q, want its relative path", got)
	}

	// A real escape is still refused, however it is spelled.
	outside := filepath.Join(t.TempDir(), "secrets.md")
	if err := os.WriteFile(outside, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c = &Client{Root: link}
	for _, p := range []string{outside, "/etc/passwd", filepath.Join(real, "..", "elsewhere.md")} {
		if got := c.rel(p); got != "" {
			t.Fatalf("rel(%q) = %q, want it dropped", p, got)
		}
	}
}

// A path the tool prints without a root used to be trusted for being relative:
// rel returned filepath.Clean(p), which keeps a leading "..". So every one of
// these escaped the repo and was reported as a path inside it, on every
// platform — Windows only made it visible, because there a rooted path with no
// drive letter is not IsAbs and reached the same branch.
func TestRelRejectsRelativeEscapesAndRootedPaths(t *testing.T) {
	root := t.TempDir()
	c := &Client{Root: root}

	for _, p := range []string{
		"../../etc/passwd",
		"../elsewhere.md",
		"openspec/../../x.md",
		"openspec/changes/../../../x.md",
		"..",
	} {
		if got := c.rel(p); got != "" {
			t.Errorf("rel(%q) = %q, want it dropped — it leaves the repo", p, got)
		}
	}

	// Rooted but not IsAbs. This shape exists only on Windows, where a path
	// with no drive letter is still rooted and so is not repo-relative; on Unix
	// a backslash or a "C:" prefix is an ordinary character in a filename and
	// such a path really is inside the repo, so there is nothing to reject.
	if runtime.GOOS == "windows" {
		for _, p := range []string{"/etc/passwd", `\windows\system32`, "C:relative"} {
			if got := c.rel(p); got != "" {
				t.Errorf("rel(%q) = %q, want it dropped — rooted paths are not repo-relative", p, got)
			}
		}
	}

	// A genuinely relative path inside the repo still works, and still comes
	// back in slash form.
	if got := c.rel(filepath.Join("openspec", "changes", "add-login", "proposal.md")); got != "openspec/changes/add-login/proposal.md" {
		t.Errorf("relative path inside the repo = %q, want it kept", got)
	}
}
