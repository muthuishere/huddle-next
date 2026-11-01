package room

import (
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func room(t *testing.T, rounds int, personas ...string) Session {
	t.Helper()
	s, err := New(Options{
		ID: "s1", Question: "Do we cache the index?", Owner: personas[0],
		Personas: personas, Rounds: rounds, Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// deliberate records every persona in every round with a chain that deepens by
// one why per round, which is what a well-behaved agent produces.
func deliberate(t *testing.T, s *Session) {
	t.Helper()
	for r := 1; r <= s.Rounds; r++ {
		for _, p := range s.SpeakingOrder {
			whys := make([]string, 0, MinWhys+r)
			for i := 0; i < MinWhys+r-1; i++ {
				whys = append(whys, "why "+p)
			}
			if _, err := s.RecordTurn(r, p, "stance", whys); err != nil {
				t.Fatalf("RecordTurn r%d %s: %v", r, p, err)
			}
		}
	}
}

func TestNewValidatesTheRoom(t *testing.T) {
	base := Options{ID: "s", Question: "q", Owner: "arasan", Personas: []string{"arasan", "nina"}, Rounds: 3}
	cases := []struct {
		name string
		mut  func(*Options)
		want string
	}{
		{"owner outside the room", func(o *Options) { o.Owner = "maya" }, "not in the room"},
		{"no owner", func(o *Options) { o.Owner = "" }, "owner persona is required"},
		{"room too small", func(o *Options) { o.Personas = []string{"arasan"} }, "at least 2 personas"},
		{"duplicate seat", func(o *Options) { o.Personas = []string{"arasan", "nina", "nina"} }, "listed twice"},
		{"too few rounds", func(o *Options) { o.Rounds = 2 }, "rounds must be 3..5"},
		{"too many rounds", func(o *Options) { o.Rounds = 6 }, "rounds must be 3..5"},
		{"no question", func(o *Options) { o.Question = "  " }, "question is required"},
		{"no id", func(o *Options) { o.ID = "" }, "session id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			tc.mut(&o)
			if _, err := New(o); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	s, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.SpeakingOrder; got[len(got)-1] != "arasan" {
		t.Errorf("speaking order = %v, owner must speak last", got)
	}
	if s.Status != StatusOpen || s.Rounds != 3 {
		t.Errorf("status %q rounds %d", s.Status, s.Rounds)
	}
	zero := base
	zero.Rounds = 0
	if s, err := New(zero); err != nil || s.Rounds != MinRounds {
		t.Errorf("rounds 0 should default to %d: %d %v", MinRounds, s.Rounds, err)
	}
}

func TestRecordTurnEnforcesDepthAndOrder(t *testing.T) {
	s := room(t, 3, "arasan", "nina")
	if _, err := s.RecordTurn(1, "nina", "s", []string{"a", "b"}); err == nil ||
		!strings.Contains(err.Error(), "at least 3 deepening steps") {
		t.Fatalf("a 2-why turn must be rejected, got %v", err)
	}
	if _, err := s.RecordTurn(1, "maya", "s", []string{"a", "b", "c"}); err == nil ||
		!strings.Contains(err.Error(), "not in this room") {
		t.Fatalf("an outsider must be rejected, got %v", err)
	}
	if _, err := s.RecordTurn(4, "nina", "s", []string{"a", "b", "c"}); err == nil ||
		!strings.Contains(err.Error(), "round must be 1..3") {
		t.Fatalf("round 4 of 3 must be rejected, got %v", err)
	}
	if _, err := s.RecordTurn(2, "nina", "s", []string{"a", "b", "c"}); err == nil ||
		!strings.Contains(err.Error(), "on round 1 next") {
		t.Fatalf("skipping round 1 must be rejected, got %v", err)
	}
	if _, err := s.RecordTurn(1, "nina", "s", []string{"a", " ", "b", "c"}); err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if _, err := s.RecordTurn(1, "nina", "s", []string{"a", "b", "c"}); err == nil ||
		!strings.Contains(err.Error(), "already spoke in round 1") {
		t.Fatalf("a second turn in one round must be rejected, got %v", err)
	}
	if s.Status != StatusDeliberating {
		t.Errorf("status = %q after a turn", s.Status)
	}
}

func TestRoundsMustDeepenMonotonically(t *testing.T) {
	s := room(t, 3, "arasan", "nina")
	if _, err := s.RecordTurn(1, "nina", "s", []string{"a", "b", "c", "d"}); err != nil {
		t.Fatal(err)
	}
	// Shallower than its own round 1 — the whole point of the rule.
	_, err := s.RecordTurn(2, "nina", "s", []string{"a", "b", "c"})
	if err == nil || !strings.Contains(err.Error(), "must dig deeper") {
		t.Fatalf("a shallower round 2 must be rejected, got %v", err)
	}
	// Equal depth is allowed (the content deepens even when the chain does not).
	if _, err := s.RecordTurn(2, "nina", "s", []string{"a", "b", "c", "d"}); err != nil {
		t.Fatalf("equal depth must be allowed: %v", err)
	}
	// The Python original let a deep round 3 be followed by a shallow round 2,
	// because it only looked at rounds < r. Sequential rounds close that hole.
	s2 := room(t, 3, "arasan", "nina")
	if _, err := s2.RecordTurn(1, "nina", "s", []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.RecordTurn(3, "nina", "s", []string{"a", "b", "c", "d", "e"}); err == nil {
		t.Fatal("round 3 before round 2 must be rejected")
	}
}

func TestVotingNeedsCompleteDeliberation(t *testing.T) {
	s := room(t, 3, "arasan", "nina")
	if _, err := s.RecordVote("nina", "A", 0.5, "r"); err == nil ||
		!strings.Contains(err.Error(), "deliberation is not complete") {
		t.Fatalf("voting before the rounds must be rejected, got %v", err)
	}
	deliberate(t, &s)
	if !s.DeliberationComplete() {
		t.Fatalf("deliberation should be complete after %d turns", len(s.Turns))
	}
	if _, err := s.RecordVote("nina", "A", 1.4, "r"); err == nil || !strings.Contains(err.Error(), "confidence must be 0..1") {
		t.Fatalf("confidence 1.4 must be rejected, got %v", err)
	}
	if _, err := s.RecordVote("nina", "  ", 0.5, "r"); err == nil || !strings.Contains(err.Error(), "--position") {
		t.Fatalf("an empty position must be rejected, got %v", err)
	}
	if _, err := s.RecordVote("nina", "A", 0.5, "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVote("nina", "B", 0.5, "r"); err == nil || !strings.Contains(err.Error(), "already voted") {
		t.Fatalf("a second vote must be rejected, got %v", err)
	}
	if got := s.DidNotVote(); len(got) != 1 || got[0] != "arasan" {
		t.Errorf("DidNotVote = %v, want [arasan]", got)
	}
}

func TestTrailGroupsByRoundInSpeakingOrder(t *testing.T) {
	s := room(t, 3, "arasan", "nina", "shaama")
	deliberate(t, &s)
	trail := s.Trail()
	if len(trail) != 3 {
		t.Fatalf("trail has %d rounds, want 3", len(trail))
	}
	for _, r := range trail {
		var got []string
		for _, turn := range r.Turns {
			got = append(got, turn.Persona)
		}
		if strings.Join(got, ",") != strings.Join(s.SpeakingOrder, ",") {
			t.Errorf("round %d order = %v, want %v", r.Round, got, s.SpeakingOrder)
		}
	}
	if trail[2].Turns[0].Depth <= trail[0].Turns[0].Depth {
		t.Errorf("round 3 depth %d not deeper than round 1 depth %d", trail[2].Turns[0].Depth, trail[0].Turns[0].Depth)
	}
}
