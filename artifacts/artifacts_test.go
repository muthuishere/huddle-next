package artifacts

import (
	"strings"
	"testing"
)

func TestFrontMatterRewritePreservesHandEdits(t *testing.T) {
	src := "---\nid: plan-x\n# a comment the user wrote\nstatus: draft\nauthor: alice\nlinks:\n  intent: intent-x\ncustom: keep me\n---\n# Body\n\ntext\n"
	fm, body := SplitFrontMatter([]byte(src))
	if fm.Get("status") != "draft" || fm.Links()["intent"][0] != "intent-x" {
		t.Fatalf("parse: %q %v", fm.Get("status"), fm.Links())
	}
	fm.Set("status", "approved")
	fm.Set("approvedBy", "bob")
	fm.AddLink("adrs", "adr-3")
	fm.AddLink("adrs", "adr-4")
	out := string(fm.Render()) + string(body)
	for _, want := range []string{"# a comment the user wrote", "custom: keep me", "status: approved", "approvedBy: bob", "adrs: [adr-3, adr-4]", "intent: intent-x", "# Body\n\ntext\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	again, _ := SplitFrontMatter([]byte(out))
	if got := again.Links()["adrs"]; len(got) != 2 {
		t.Errorf("round trip links: %v", got)
	}
}

func TestParseYAMLSubset(t *testing.T) {
	doc, err := ParseYAML([]byte("metric: p95 # comment\nbaseline:\n  window: 20\n  mean: 1.5\nrules:\n  - direction: up\n    min_points: 4\n  - plain\ntags: [a, \"b c\"]\nok: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if doc["metric"] != "p95" || doc["baseline"].(map[string]any)["mean"] != 1.5 || doc["ok"] != true {
		t.Fatalf("doc: %#v", doc)
	}
	rules := doc["rules"].([]any)
	if len(rules) != 2 || rules[0].(map[string]any)["min_points"] != 4.0 || rules[1] != "plain" {
		t.Fatalf("rules: %#v", rules)
	}
	if tags := doc["tags"].([]any); len(tags) != 2 || tags[1] != "b c" {
		t.Fatalf("tags: %#v", tags)
	}
	if _, err := ParseYAML([]byte("a: 1\n   b: 2\n")); err == nil {
		t.Fatal("bad indentation accepted")
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("  Can pgvector handle 50M rows?  "); got != "can-pgvector-handle-50m-rows" {
		t.Fatal(got)
	}
}

// Statuses hands out a copy. It used to be an exported slice, which meant any
// importer — including an embedding host — could rewrite the vocabulary that
// lint validates `status:` against, for the whole process.
func TestStatusesCannotBeMutatedByCallers(t *testing.T) {
	got := Statuses()
	if len(got) == 0 {
		t.Fatal("no statuses")
	}
	got[0] = "whatever"
	if again := Statuses(); again[0] == "whatever" {
		t.Fatal("mutating the returned slice changed the package's own vocabulary")
	}
}
