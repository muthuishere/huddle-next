package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/artifacts"
)

// The repo huddle is actually installed into: a hand-written CLAUDE.md, no
// huddle markers, and another tool's managed block in it.
const handWrittenClaudeMD = `# Acme API

Read the runbook first.

<!-- crossmem:managed -->
crossmem owns this block
<!-- /crossmem:managed -->

## House rules

- Money is BigDecimal.
`

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// outsideBlock returns everything but huddle's own block.
func outsideBlock(t *testing.T, content string) string {
	t.Helper()
	start, end, err := artifacts.ManagedSpan(content)
	if err != nil || start < 0 {
		t.Fatalf("no managed block in:\n%s\n(%v)", content, err)
	}
	return content[:start] + content[end:]
}

func TestGuardrailsAdoptExistingClaudeMD(t *testing.T) {
	root := artTestEnv(t)
	path := filepath.Join(root, "CLAUDE.md")
	artWrite(t, path, handWrittenClaudeMD)

	// `new` must not claim the only way in is hand-writing the markers.
	_, err := artRun(t, root, "guardrails", "new")
	if !errors.Is(err, artifacts.ErrExists) {
		t.Fatalf("new on an existing CLAUDE.md: %v", err)
	}
	if !strings.Contains(err.Error(), "huddle guardrails adopt") || strings.Contains(err.Error(), "by hand") {
		t.Errorf("new should name adopt, not hand editing: %v", err)
	}

	// --dry-run writes nothing.
	out := artMust(t, root, "guardrails", "adopt", "--dry-run")
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, artifacts.GuardrailsBegin) {
		t.Errorf("dry run output:\n%s", out)
	}
	if got := readFile(t, path); got != handWrittenClaudeMD {
		t.Fatalf("dry run touched the file:\n%s", got)
	}

	artMust(t, root, "guardrails", "adopt")
	got := readFile(t, path)

	// Everything outside huddle's markers survives byte for byte; front
	// matter (huddle's own metadata, required by lint) is the only addition.
	outside := outsideBlock(t, got)
	fm, bodyB := artifacts.SplitFrontMatter([]byte(got))
	body := string(bodyB)
	if !fm.Present {
		t.Error("adopt should add huddle front matter")
	}
	// The hand-written file is still there byte for byte, in front of the
	// appended block; only trailing blank lines separate the two.
	if !strings.HasPrefix(body, handWrittenClaudeMD) {
		t.Errorf("content outside the markers changed:\n%q\nwant it to start with:\n%q", body, handWrittenClaudeMD)
	}
	if strings.TrimSpace(strings.TrimPrefix(outsideBlock(t, body), handWrittenClaudeMD)) != "" {
		t.Errorf("adopt added text outside its own block:\n%q", outsideBlock(t, body))
	}
	if !strings.Contains(outside, "<!-- crossmem:managed -->\ncrossmem owns this block") {
		t.Error("another tool's managed block was not kept intact")
	}
	if !strings.HasPrefix(body, "# Acme API") {
		t.Errorf("body no longer starts with the user's heading:\n%s", body)
	}

	// The result is a valid, lintable guardrails artifact.
	if out, err := artRun(t, root, "guardrails", "lint"); err != nil {
		t.Fatalf("guardrails lint after adopt: %v\n%s", err, out)
	}
	if out := artMust(t, root, "guardrails", "status"); !strings.Contains(out, "CLAUDE.md") {
		t.Errorf("status:\n%s", out)
	}

	// Re-running never clobbers what a human wrote inside the block.
	artWrite(t, path, strings.Replace(got, "## Commands", "## Commands\n\n- task build", 1))
	mine := readFile(t, path)
	out = artMust(t, root, "guardrails", "adopt")
	if !strings.Contains(out, "CHANGED: no") {
		t.Errorf("second adopt:\n%s", out)
	}
	if readFile(t, path) != mine {
		t.Error("second adopt rewrote the block")
	}
}

func TestGuardrailsAdoptFromDraftRefreshesOnlyTheBlock(t *testing.T) {
	root := artTestEnv(t)
	path := filepath.Join(root, "CLAUDE.md")
	artWrite(t, path, handWrittenClaudeMD)
	artMust(t, root, "guardrails", "adopt")
	before := readFile(t, path)

	draft := filepath.Join(root, "draft.md")
	artWrite(t, draft, "## Commands\n\n- task build\n\n## Conventions\n\n- stdlib only\n\n## Architecture\n\nOne binary.\n\n## Things Claude gets wrong\n\n- says done without building\n")
	artMust(t, root, "guardrails", "adopt", "--from", draft)
	after := readFile(t, path)

	if outsideBlock(t, after) != outsideBlock(t, before) {
		t.Errorf("refresh changed content outside the markers:\n%s", after)
	}
	if !strings.Contains(after, "- says done without building") {
		t.Errorf("draft was not written into the block:\n%s", after)
	}
	if out, err := artRun(t, root, "guardrails", "lint"); err != nil {
		t.Fatalf("lint after refresh: %v\n%s", err, out)
	}
}

func TestGuardrailsAdoptRefusesMalformedMarkers(t *testing.T) {
	root := artTestEnv(t)
	path := filepath.Join(root, "CLAUDE.md")
	cases := map[string]string{
		"never closed":  "# x\n" + artifacts.GuardrailsBegin + "\nhi\n",
		"never opened":  "# x\n" + artifacts.GuardrailsEnd + "\n",
		"out of order":  "# x\n" + artifacts.GuardrailsEnd + "\nhi\n" + artifacts.GuardrailsBegin + "\n",
		"two blocks":    "# x\n" + artifacts.GuardrailsBegin + "\na\n" + artifacts.GuardrailsEnd + "\n" + artifacts.GuardrailsBegin + "\nb\n" + artifacts.GuardrailsEnd + "\n",
		"nested blocks": "# x\n" + artifacts.GuardrailsBegin + "\n" + artifacts.GuardrailsBegin + "\na\n" + artifacts.GuardrailsEnd + "\n" + artifacts.GuardrailsEnd + "\n",
	}
	for name, body := range cases {
		artWrite(t, path, body)
		_, err := artRun(t, root, "guardrails", "adopt")
		if !errors.Is(err, artifacts.ErrManagedBlock) {
			t.Errorf("%s: want a refusal, got %v", name, err)
		}
		if readFile(t, path) != body {
			t.Errorf("%s: file was changed despite the refusal", name)
		}
	}
}

func TestGuardrailsAdoptNeedsAnExistingFile(t *testing.T) {
	root := artTestEnv(t)
	_, err := artRun(t, root, "guardrails", "adopt")
	if err == nil || !strings.Contains(err.Error(), "huddle guardrails new") {
		t.Fatalf("adopt with no CLAUDE.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("adopt created the file it refused to adopt")
	}
}
