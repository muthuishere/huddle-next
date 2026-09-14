package personas

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const validDef = `---
name: huddle-%[1]s
displayName: %[2]s
title: %[3]s
icon: "🧩"
role: Owns things
domains: [alpha, beta]
capabilities: "stuff, more stuff"
identity: "Did things."
primaryLens: "What breaks?"
communicationStyle: "Short."
principles: "Less is more."
---

## Signature Phrases

- "One."
- "Two."

## Expertise Areas

Things.

## Blind Spots

Misses things. Also other things. And a third.

## When Useful

When things.
`

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testLayers(t *testing.T) []Source {
	t.Helper()
	builtin := fstest.MapFS{
		"shaama/shaama.md":            {Data: []byte(fmt.Sprintf(validDef, "shaama", "Shaama", "Backend"))},
		"shaama/memories/pager.md":    {Data: []byte("---\ntitle: Builtin pager lesson\ntags: [oncall]\n---\nbody")},
		"shaama/memories/rollback.md": {Data: []byte("# Rollback first\n")},
		"nina/nina.md":                {Data: []byte(fmt.Sprintf(validDef, "nina", "Nina", "Tester"))},
	}
	global := t.TempDir()
	repo := t.TempDir()
	// global: metadata-only override of shaama's title; memory overriding builtin pager.md
	writeFile(t, filepath.Join(global, "shaama", "shaama.md"), "---\ntitle: Global Backend\nicon: \"\"\n---\n")
	writeFile(t, filepath.Join(global, "shaama", "memories", "pager.md"), "# Global pager lesson\n")
	writeFile(t, filepath.Join(global, "shaama", "runbook.md"), "no heading here\n")
	// repo: memory-only dir for nina, full new persona rahul, repo overrides pager again
	writeFile(t, filepath.Join(repo, "nina", "memories", "flaky.md"), "# Flaky suites\n")
	writeFile(t, filepath.Join(repo, "shaama", "memories", "pager.md"), "---\ntitle: Repo pager lesson\n---\n")
	writeFile(t, filepath.Join(repo, "shaama", "source", "imported.md"), "raw")
	writeFile(t, filepath.Join(repo, "rahul", "rahul.md"), fmt.Sprintf(validDef, "rahul", "Rahul", "SRE"))
	writeFile(t, filepath.Join(repo, "rahul", "notes.txt"), "ignored")
	return []Source{
		{Layer: Builtin, FS: builtin, Dir: "builtin:personas"},
		DirSource(Global, global),
		DirSource(Repo, repo),
	}
}

func TestMergePrecedence(t *testing.T) {
	r, err := Resolve(testLayers(t))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range r.Personas {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != "nina,rahul,shaama" {
		t.Fatalf("ids = %v", ids)
	}
	s, _ := r.Get("shaama")
	if s.Title != "Global Backend" || s.Provenance["title"] != Global {
		t.Errorf("title = %q from %s, want global override", s.Title, s.Provenance["title"])
	}
	if s.DisplayName != "Shaama" || s.Provenance["displayName"] != Builtin {
		t.Errorf("unset field must fall through: %q from %s", s.DisplayName, s.Provenance["displayName"])
	}
	if s.Icon != "🧩" {
		t.Errorf("empty value must not set a field, icon = %q", s.Icon)
	}
	if s.Definition == nil || s.Definition.Layer != Builtin {
		t.Errorf("front-matter-only global file must not erase builtin body: %+v", s.Definition)
	}
	if strings.Join(personaLayerNames(s.Layers), ",") != "builtin,global,repo" {
		t.Errorf("layers = %v", s.Layers)
	}
	if r2, _ := r.Get("rahul"); r2 == nil || r2.Definition.Layer != Repo || r2.Title != "SRE" {
		t.Errorf("repo-only persona not resolved: %+v", r2)
	}
}

func personaLayerNames(ls []Layer) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = string(l)
	}
	return out
}

func TestMemoryDedupeAndOrder(t *testing.T) {
	r, err := Resolve(testLayers(t))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := r.Get("shaama")
	got := map[string]Knowledge{}
	var order []string
	for _, k := range s.Knowledge {
		if _, dup := got[k.Rel]; dup {
			t.Fatalf("duplicate relative path %s", k.Rel)
		}
		got[k.Rel] = k
		order = append(order, string(k.Layer)+":"+k.Rel)
	}
	if k := got["memories/pager.md"]; k.Layer != Repo || k.Title != "Repo pager lesson" || k.Kind != "memory" {
		t.Errorf("pager.md should be the repo copy: %+v", k)
	}
	if k := got["memories/rollback.md"]; k.Layer != Builtin || k.Title != "Rollback first" {
		t.Errorf("rollback.md: %+v", k)
	}
	if k := got["runbook.md"]; k.Kind != "knowledge" || k.Title != "runbook" {
		t.Errorf("runbook.md title falls back to file name: %+v", k)
	}
	if _, ok := got["source/imported.md"]; ok {
		t.Error("source/ imports must not be indexed as knowledge")
	}
	want := []string{"builtin:memories/rollback.md", "global:runbook.md", "repo:memories/pager.md"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestMemoryOnlyDirKeepsBuiltinBody(t *testing.T) {
	r, err := Resolve(testLayers(t))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := r.Get("nina")
	if n.Definition == nil || n.Definition.Layer != Builtin {
		t.Fatalf("nina body should stay builtin: %+v", n.Definition)
	}
	body, err := n.Body()
	if err != nil || !strings.Contains(body, "## Signature Phrases") {
		t.Fatalf("body = %q, err %v", body, err)
	}
	if len(n.Knowledge) != 1 || n.Knowledge[0].Layer != Repo {
		t.Errorf("knowledge = %+v", n.Knowledge)
	}
	issues, _ := r.Lint("nina")
	if HasErrors(issues) {
		t.Errorf("nina lint: %v", issues)
	}
}

func TestBuiltinsPassLint(t *testing.T) {
	r, err := Resolve([]Source{BuiltinSource()})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Personas) != 21 {
		t.Fatalf("want 21 builtins, got %d", len(r.Personas))
	}
	issues, err := r.Lint("")
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range issues {
		if is.Severity != SevWaived {
			t.Errorf("%s", is)
		}
	}
	for _, id := range []string{"sreyash", "hari", "harshvardhan"} {
		if p, _ := r.Get(id); p.Selectable {
			t.Errorf("%s must be selectable:false (ADR 11)", id)
		}
	}
	if p, _ := r.Get("elango"); !p.Silent {
		t.Error("elango must be silent")
	}
}

func TestStyleCardsWithinBudget(t *testing.T) {
	r, err := Resolve([]Source{BuiltinSource()})
	if err != nil {
		t.Fatal(err)
	}
	tasks := append([]string{""}, TaskKeys()...)
	for _, p := range r.Personas {
		for _, task := range tasks {
			card, err := Style(p, task)
			if err != nil {
				t.Fatalf("%s/%s: %v", p.ID, task, err)
			}
			if n := len(card.Markdown()); n > MaxStyleCardBytes {
				t.Errorf("%s --for %q card is %d bytes > %d", p.ID, task, n, MaxStyleCardBytes)
			}
			if len(card.SignaturePhrases) > 4 || len(card.BlindSpots) > 2 {
				t.Errorf("%s: phrases %d blind spots %d", p.ID, len(card.SignaturePhrases), len(card.BlindSpots))
			}
		}
	}
	shaama, _ := r.Get("shaama")
	card, _ := Style(shaama, "review")
	if len(card.SignaturePhrases) != 4 || card.Focus == "" || card.PrimaryLens == "" {
		t.Errorf("shaama card incomplete: %+v", card)
	}
	if _, err := Style(shaama, "poem"); err == nil {
		t.Error("unknown --for must error")
	}
}

func TestBriefAndFind(t *testing.T) {
	r, err := Resolve(testLayers(t))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), "x")
	br, err := MakeBrief(r, []string{"shaama", "nina"}, "fix the flaky pager alert", "review", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(br.Cards) != 2 || len(br.Guardrails) != 1 || !strings.HasSuffix(br.Guardrails[0], "AGENTS.md") {
		t.Fatalf("brief = %+v", br)
	}
	var titles []string
	for _, m := range br.Memories {
		titles = append(titles, m.Title)
	}
	sort.Strings(titles)
	if strings.Join(titles, "|") != "Flaky suites|Repo pager lesson" {
		t.Errorf("memories = %v", titles)
	}
	if _, err := MakeBrief(r, []string{"ghost"}, "x", "", root); err == nil {
		t.Error("unknown id must error")
	}
	m := Find(r, "SRE")
	if len(m) == 0 || m[0].ID != "rahul" {
		t.Errorf("find SRE = %+v", m)
	}
	if got := Find(r, "flaky"); len(got) != 1 || got[0].ID != "nina" {
		t.Errorf("find by knowledge title = %+v", got)
	}
}

func TestParserTolerance(t *testing.T) {
	cases := []string{
		"",
		"---",
		"---\n",
		"no front matter",
		"---\nname: x\n", // unclosed
		"---\n: nokey\n- orphan\n[bad]\n---\nbody", // garbage lines
		"---\nname: \"unterminated\n---\n",
		"---\ndomains: [a, \"b, c\", 'd'\n---\n", // unclosed list
		"---\ntitle: a # comment\nicon: \"🔥\"\nlist:\n  - one\n  - \"two\"\nfolded: >\n  line one\n  line two\n---\n",
		"\xef\xbb\xbf---\nname: bom\n---\n# Body",
		"---\r\nname: crlf\r\n---\r\nbody",
		strings.Repeat("-", 5000),
		"---\n" + strings.Repeat("k: v\n", 5000),
	}
	for i, c := range cases {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("case %d panicked: %v", i, p)
				}
			}()
			fm := ParseFrontMatter([]byte(c))
			if fm.BodyAt < 0 || fm.BodyAt > len(c) {
				t.Errorf("case %d: BodyAt %d out of range", i, fm.BodyAt)
			}
			_ = LintDefinition("x", "x.md", []byte(c))
			_ = Template("x", []byte(c))
		}()
	}
	fm := ParseFrontMatter([]byte(cases[8]))
	if fm.Fields["title"].Str != "a" || fm.Fields["icon"].Str != "🔥" {
		t.Errorf("scalars: %+v", fm.Fields)
	}
	if l := fm.Fields["list"]; !l.IsList || strings.Join(l.List, ",") != "one,two" {
		t.Errorf("block list: %+v", l)
	}
	if fm.Fields["folded"].Str != "line one line two" {
		t.Errorf("folded: %q", fm.Fields["folded"].Str)
	}
	if d := ParseFrontMatter([]byte("---\ndomains: [a, \"b, c\", 'd']\n---\n")).Fields["domains"]; strings.Join(d.List, "|") != "a|b, c|d" {
		t.Errorf("inline list: %+v", d)
	}
	bom := ParseFrontMatter([]byte(cases[9]))
	if bom.Fields["name"].Str != "bom" || cases[9][bom.BodyAt:] != "# Body" {
		t.Errorf("bom: %+v", bom)
	}
	if crlf := ParseFrontMatter([]byte(cases[10])); crlf.Fields["name"].Str != "crlf" || !crlf.Closed {
		t.Errorf("crlf: %+v", crlf)
	}
	if p := ParseFrontMatter([]byte(cases[5])); len(p.Problems) == 0 {
		t.Error("garbage lines must be reported as problems")
	}
}

func TestLintDefinitionErrorsAndTemplate(t *testing.T) {
	issues := LintDefinition("rahul", "d.md", []byte("---\nname: x\n---\n## Signature Phrases\n- hi\n"))
	msgs := fmt.Sprint(issues)
	for _, want := range []string{`"displayName"`, `"principles"`, `## Expertise Areas`, `## When Useful`} {
		if !strings.Contains(msgs, want) {
			t.Errorf("missing error for %s in %s", want, msgs)
		}
	}
	tpl := Template("rahul-sre", []byte("---\ntitle: SRE\n---\n"))
	if !strings.Contains(tpl, "title: SRE") || !strings.Contains(tpl, "displayName: Rahul") {
		t.Errorf("template not pre-filled:\n%s", tpl)
	}
	// A filled template is valid apart from TODO text.
	if HasErrors(LintDefinition("rahul-sre", "t.md", []byte(tpl))) {
		t.Errorf("template must be structurally valid: %v", LintDefinition("rahul-sre", "t.md", []byte(tpl)))
	}
}

func TestIngestRedactsAndIsPrivate(t *testing.T) {
	t.Setenv("HUDDLE_HOME", t.TempDir())
	calls := 0
	orig := redactString
	redactString = func(s string) string {
		calls++
		return strings.ReplaceAll(s, "sk-secret", "[REDACTED]")
	}
	defer func() { redactString = orig }()

	in := `{"text":"token sk-secret here","author":"alice","tags":["sk-secret"]}
{"text":"second","meta":{"k":"sk-secret"}}
`
	res, err := Ingest("alice-reviewer", "dump.ndjson", []byte(in), "label", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 2 || res.Format != "ndjson" || calls == 0 {
		t.Fatalf("res %+v calls %d", res, calls)
	}
	b, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sk-secret") || strings.Count(string(b), "\n") != 2 {
		t.Errorf("stored evidence not redacted:\n%s", b)
	}
	st, _ := os.Stat(res.Path)
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("evidence perms = %v, want 0600", st.Mode().Perm())
	}
	if _, err := Ingest("alice-reviewer", "notes.md", []byte("# Notes\nplain markdown"), "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := Ingest("alice-reviewer", "bad.ndjson", []byte("{\"author\":\"x\"}\nnot json\n"), "", time.Now()); err == nil ||
		!strings.Contains(err.Error(), "line 1") || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("bad ndjson error = %v", err)
	}
	b2, _ := os.ReadFile(res.Path)
	if strings.Count(string(b2), "\n") != 3 {
		t.Errorf("invalid input must store nothing; lines = %d", strings.Count(string(b2), "\n"))
	}
}

func TestCapsReportOverflow(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < MaxDirsPerLayer+2; i++ {
		os.MkdirAll(filepath.Join(dir, fmt.Sprintf("p%03d", i)), 0o755)
	}
	for i := 0; i < MaxFilesPerDir+1; i++ {
		writeFile(t, filepath.Join(dir, "p000", "memories", fmt.Sprintf("m%03d.md", i)), "# m")
	}
	r, err := Resolve([]Source{DirSource(Global, dir)})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Personas) != MaxDirsPerLayer || len(r.Overflow) != 2 {
		t.Errorf("personas %d overflow %v", len(r.Personas), r.Overflow)
	}
}

// syntheticLayer builds n persona dirs, each with a definition and two memories.
func syntheticLayer(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("persona%03d", i)
		writeFile(tb, filepath.Join(dir, id, id+".md"), fmt.Sprintf(validDef, id, id, "Title"))
		writeFile(tb, filepath.Join(dir, id, "memories", "one.md"), "---\ntitle: One\n---\n")
		writeFile(tb, filepath.Join(dir, id, "memories", "two.md"), "# Two\n")
	}
	return dir
}

func BenchmarkResolve(b *testing.B) {
	srcs := []Source{BuiltinSource(), DirSource(Global, syntheticLayer(b, 200))}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Resolve(srcs); err != nil {
			b.Fatal(err)
		}
	}
}

func TestResolveBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing budget skipped under -short")
	}
	srcs := []Source{BuiltinSource(), DirSource(Global, syntheticLayer(t, 200))}
	const runs = 21
	d := make([]time.Duration, runs)
	for i := range d {
		start := time.Now()
		r, err := Resolve(srcs)
		d[i] = time.Since(start)
		if err != nil || len(r.Personas) != 221 {
			t.Fatalf("resolve: %v (%d personas)", err, len(r.Personas))
		}
	}
	sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
	p50 := d[runs/2]
	t.Logf("Resolve 200 synthetic + 21 builtin personas: p50 %v", p50)
	if p50 > 20*time.Millisecond {
		t.Errorf("Resolve p50 %v > 20ms budget", p50)
	}
}
