package autonomous

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decided builds a complete session: three rounds, every persona, every vote.
func decided(t *testing.T, positions map[string]string, conf map[string]float64) Session {
	t.Helper()
	s := room(t, 3, "arasan", "nina", "shaama")
	deliberate(t, &s)
	for _, p := range s.Personas {
		if _, err := s.RecordVote(p, positions[p], conf[p], p+" reason"); err != nil {
			t.Fatalf("vote %s: %v", p, err)
		}
	}
	return s
}

func TestBuildVerdictRefusesAnIncompleteRoom(t *testing.T) {
	s := room(t, 3, "arasan", "nina")
	if _, err := BuildVerdict(s, "do it", "", nil, fixedNow); err == nil ||
		!strings.Contains(err.Error(), "deliberation is not complete") {
		t.Fatalf("a verdict without deliberation must be refused, got %v", err)
	}
	deliberate(t, &s)
	if _, err := s.RecordVote("nina", "A", 0.5, "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildVerdict(s, "do it", "", nil, fixedNow); err == nil ||
		!strings.Contains(err.Error(), "arasan have not voted") {
		t.Fatalf("a verdict without every vote must be refused, got %v", err)
	}
}

func TestBuildVerdictDecidedIsActionable(t *testing.T) {
	s := decided(t,
		map[string]string{"arasan": "cache it", "nina": "cache it", "shaama": "measure first"},
		map[string]float64{"arasan": 0.8, "nina": 0.6, "shaama": 0.9})
	v, err := BuildVerdict(s, "Cache the index behind a flag", "reversible, measure after", nil, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusDecided || !v.Actionable {
		t.Fatalf("status %q actionable %t, want decided/true", v.Status, v.Actionable)
	}
	if v.Tally.Resolved != "cache it" || v.Tally.OwnerOverridden {
		t.Errorf("tally = %+v, want the owner's option", v.Tally)
	}
	if len(v.Tally.Dissents) != 1 || v.Tally.Dissents[0].Persona != "shaama" {
		t.Errorf("dissents = %+v, want shaama recorded", v.Tally.Dissents)
	}
	if len(v.Trail) != 3 || len(v.Trail[0].Turns) != 3 {
		t.Errorf("trail = %d rounds, first round %d turns", len(v.Trail), len(v.Trail[0].Turns))
	}
	md := Markdown(v)
	for _, want := range []string{"Cache the index behind a flag", "## Dissents", "shaama", "## 5-whys rationale trail", "Round 3", "act on it headlessly"} {
		if !strings.Contains(md, want) {
			t.Errorf("verdict.md is missing %q", want)
		}
	}
	if strings.Contains(md, "OWNER-LEVEL FORK") {
		t.Error("a decided verdict must not carry the escalation banner")
	}
}

func TestBuildVerdictEscalatesAnOwnerLevelFork(t *testing.T) {
	s := decided(t,
		map[string]string{"arasan": "ship it", "nina": "ship it", "shaama": "wait"},
		map[string]float64{"arasan": 0.9, "nina": 0.5, "shaama": 0.4})
	v, err := BuildVerdict(s, "Go live with public pricing", "", []string{"spend"}, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusEscalated || v.Actionable {
		t.Fatalf("status %q actionable %t, want escalated/false", v.Status, v.Actionable)
	}
	if !v.Escalation.Required || !contains(v.Escalation.Flags, "spend") || !contains(v.Escalation.Flags, "launch") {
		t.Errorf("escalation = %+v, want the declared spend and the detected launch", v.Escalation)
	}
	if !contains(v.Escalation.Detected, "launch") {
		t.Errorf("detected = %v, want launch", v.Escalation.Detected)
	}
	md := Markdown(v)
	if !strings.Contains(md, "OWNER-LEVEL FORK") || !strings.Contains(md, "do not act") {
		t.Errorf("an escalated verdict.md must say not to act:\n%s", md)
	}
	// The resolved option still stands in the record — the room did its work.
	if v.Tally.Resolved != "ship it" {
		t.Errorf("resolved = %q", v.Tally.Resolved)
	}
}

func TestBuildVerdictDefaultsToTheResolvedOption(t *testing.T) {
	s := decided(t,
		map[string]string{"arasan": "A", "nina": "B", "shaama": "B"},
		map[string]float64{"arasan": 0.2, "nina": 0.9, "shaama": 0.9})
	v, err := BuildVerdict(s, "   ", "", nil, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != "B" || !v.Tally.OwnerOverridden {
		t.Fatalf("decision %q overridden %t, want B with the override recorded", v.Decision, v.Tally.OwnerOverridden)
	}
	if !strings.Contains(Markdown(v), "overridden by a decisively dominant room") {
		t.Error("verdict.md must record the override")
	}
}

func TestStoreNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	s := decided(t,
		map[string]string{"arasan": "A", "nina": "A", "shaama": "B"},
		map[string]float64{"arasan": 0.7, "nina": 0.5, "shaama": 0.4})
	if _, err := Create(root, s); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, s); !errors.Is(err, ErrExists) {
		t.Fatalf("a second create must be ErrExists, got %v", err)
	}
	loaded, err := Load(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Turns) != len(s.Turns) || len(loaded.Votes) != len(s.Votes) || loaded.Owner != s.Owner {
		t.Fatalf("round trip lost state: %d turns, %d votes", len(loaded.Turns), len(loaded.Votes))
	}
	v, err := BuildVerdict(loaded, "Do A", "", nil, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteVerdict(root, loaded, v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteVerdict(root, loaded, v); !errors.Is(err, ErrExists) {
		t.Fatalf("a second verdict must be ErrExists, got %v", err)
	}
	var onDisk Verdict
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Decision != "Do A" || onDisk.Status != StatusDecided {
		t.Errorf("verdict.json = %+v", onDisk)
	}
	if _, err := os.Stat(filepath.Join(Dir(root, s.ID), VerdictMD)); err != nil {
		t.Errorf("verdict.md: %v", err)
	}
	after, err := Load(root, s.ID)
	if err != nil || after.Status != StatusDecided {
		t.Errorf("session status = %q after the verdict (%v)", after.Status, err)
	}
	if _, err := Load(root, "nope"); err == nil || !strings.Contains(err.Error(), "no autonomous session") {
		t.Errorf("missing session: %v", err)
	}
	list, err := List(root)
	if err != nil || len(list) != 1 {
		t.Errorf("List = %d sessions (%v)", len(list), err)
	}
	if empty, err := List(filepath.Join(root, "absent")); err != nil || empty != nil {
		t.Errorf("List of a missing root = %v, %v", empty, err)
	}
}

func TestRecordedTextIsRedacted(t *testing.T) {
	// A synthetic token, built not pasted, so the fixture is never a live value.
	fake := "ghp_" + strings.Repeat("a", 36)
	s, err := New(Options{ID: "s", Question: "why is " + fake + " in the config?", Owner: "arasan", Personas: []string{"arasan", "nina"}, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.Question, fake) {
		t.Errorf("the question kept a token: %q", s.Question)
	}
	if _, err := s.RecordTurn(1, "nina", fake, []string{fake, "b", "c"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.Turns[0].Stance+s.Turns[0].Why[0], fake) {
		t.Errorf("a turn kept a token: %+v", s.Turns[0])
	}
}
