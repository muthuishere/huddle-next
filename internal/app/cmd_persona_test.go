package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func personaRun(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	if stdin != "" {
		orig := personaStdin
		personaStdin = strings.NewReader(stdin)
		defer func() { personaStdin = orig }()
	}
	var out, errb bytes.Buffer
	err := Run(append([]string{"persona"}, args...), &out, &errb)
	return out.String() + errb.String(), err
}

func TestPersonaListResolveStyleLint(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	out, err := personaRun(t, "", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Personas []struct {
			ID         string `json:"id"`
			Selectable bool   `json:"selectable"`
		} `json:"personas"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil || len(list.Personas) < 21 {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if out, err = personaRun(t, "", "resolve", "shaama", "--explain", "--json"); err != nil || !strings.Contains(out, `"provenance"`) {
		t.Fatalf("resolve --explain: %v\n%s", err, out)
	}
	if out, err = personaRun(t, "", "style", "nina", "--for", "review"); err != nil || !strings.Contains(out, "Task focus: review") || len(out) > 1600 {
		t.Fatalf("style: %v (%d bytes)\n%s", err, len(out), out)
	}
	if out, err = personaRun(t, "", "brief", "shaama,nina", "--task", "review the retry logic"); err != nil || !strings.Contains(out, "## Task") {
		t.Fatalf("brief: %v\n%s", err, out)
	}
	if out, err = personaRun(t, "", "find", "security threat"); err != nil || !strings.HasPrefix(strings.Split(out, "\n")[1], "senthil") {
		t.Fatalf("find: %v\n%s", err, out)
	}
	if out, err = personaRun(t, "", "lint"); err != nil || !strings.Contains(out, "ok:") {
		t.Fatalf("lint: %v\n%s", err, out)
	}
	if _, err = personaRun(t, "", "style", "ghost"); err == nil {
		t.Fatal("unknown persona must error")
	}
}

func TestPersonaNewImportIngest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUDDLE_HOME", home)
	dir := t.TempDir()

	bad := filepath.Join(dir, "bad.md")
	os.WriteFile(bad, []byte("---\nname: huddle-sre\ntitle: SRE\n---\n## Signature Phrases\n- \"page me\"\n"), 0o644)
	out, err := personaRun(t, "", "new", "rahul-sre", "--scope", "global", "--from", bad)
	if err == nil || !strings.Contains(out, `missing required front-matter key "displayName"`) {
		t.Fatalf("invalid draft must fail with exact errors: %v\n%s", err, out)
	}
	target := filepath.Join(home, "personas", "rahul-sre", "rahul-sre.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("invalid draft must not write without --template")
	}
	if out, err = personaRun(t, "", "new", "rahul-sre", "--scope", "global", "--from", bad, "--template"); err != nil {
		t.Fatalf("--template: %v\n%s", err, out)
	}
	b, _ := os.ReadFile(target)
	if !strings.Contains(string(b), "title: SRE") || !strings.Contains(string(b), `"page me"`) {
		t.Fatalf("scaffold not pre-filled:\n%s", b)
	}
	if _, err = personaRun(t, "", "new", "rahul-sre", "--scope", "global", "--template"); err == nil {
		t.Fatal("existing file must not be overwritten without --force")
	}
	if out, err = personaRun(t, "", "lint", "rahul-sre"); err != nil {
		t.Fatalf("scaffold should lint structurally: %v\n%s", err, out)
	}
	if out, err = personaRun(t, "", "new", "x-dry", "--scope", "global", "--template", "--dry-run"); err != nil || !strings.Contains(out, "dry-run") {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "personas", "x-dry")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write")
	}

	out, err = personaRun(t, "---\nname: agent\n---\nYou are a reviewer.", "import", "-", "--scope", "global", "--id", "imported-one")
	if err != nil || !strings.Contains(out, "persona new imported-one") {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if b, err := os.ReadFile(filepath.Join(home, "personas", "imported-one", "source", "imported.md")); err != nil || !strings.Contains(string(b), "You are a reviewer.") {
		t.Fatalf("import store: %v\n%s", err, b)
	}

	out, err = personaRun(t, `{"text":"nit: rename this","author":"alice"}`+"\n", "ingest", "alice", "--label", "gh reviews", "--json")
	if err != nil || !strings.Contains(out, `"records": 1`) {
		t.Fatalf("ingest: %v\n%s", err, out)
	}
	ev := filepath.Join(home, "evidence", "alice", "evidence.ndjson")
	if st, err := os.Stat(ev); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("evidence file: %v %v", err, st)
	}
	if out, err = personaRun(t, "", "ingest", "--schema"); err != nil || !strings.Contains(out, `"required": ["text"]`) {
		t.Fatalf("schema: %v\n%s", err, out)
	}
	events, _ := filepath.Glob(filepath.Join(home, "repos", "*", "events.jsonl"))
	if len(events) != 1 {
		t.Fatalf("expected one event log, got %v", events)
	}
	log, _ := os.ReadFile(events[0])
	if !strings.Contains(string(log), `"persona.written"`) || !strings.Contains(string(log), `"persona.ingested"`) {
		t.Fatalf("events not logged:\n%s", log)
	}
}
