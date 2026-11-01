package room_test

import (
	"fmt"
	"time"

	"github.com/muthuishere/huddle-next/room"
)

// A host drives a whole room in-process: it owns the model calls, this package
// owns the shape and the arithmetic. The turns below stand in for whatever the
// host's persona agents answered — nothing here calls a model.
func ExampleBuildVerdict() {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s, err := room.New(room.Options{
		ID:       "r1",
		Question: "Do we cache the persona index?",
		Owner:    "arasan",
		Personas: []string{"arasan", "nina"},
		Rounds:   room.MinRounds,
		Now:      now,
	})
	if err != nil {
		panic(err)
	}

	// Each persona speaks every round, and every round must dig at least as
	// deep as that persona's previous one — the host cannot skip or shallow out.
	for round := 1; round <= s.Rounds; round++ {
		for _, p := range s.SpeakingOrder {
			whys := make([]string, 0, round+2)
			for i := 0; i < round+2; i++ {
				whys = append(whys, fmt.Sprintf("why %d", i+1))
			}
			if _, err := s.RecordTurn(round, p, "cache it", whys); err != nil {
				panic(err)
			}
		}
	}
	if !s.DeliberationComplete() {
		panic("room is not done deliberating")
	}

	for _, p := range s.Personas {
		if _, err := s.RecordVote(p, "cache it", 0.8, "startup cost dominates"); err != nil {
			panic(err)
		}
	}

	v, err := room.BuildVerdict(s, "Cache the index behind a flag", "reversible", nil, now)
	if err != nil {
		panic(err)
	}
	fmt.Println(v.Status, "|", v.Decision)
	// Output: decided | Cache the index behind a flag
}

// SpeakingOrder is deterministic and puts the owner last, so the deciding
// voice answers the room rather than anchoring it.
func ExampleSpeakingOrder() {
	fmt.Println(room.SpeakingOrder([]string{"arasan", "nina", "suren"}, "arasan"))
	// Output: [nina suren arasan]
}
