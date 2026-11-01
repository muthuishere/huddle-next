// Package room holds the deterministic substrate for a huddle room — the
// session structure, the 5-whys depth rules, the owner-weighted decision math,
// the owner-level fork policy and the verdict writer. It backs the CLI's
// `huddle autonomous` verbs, and it is the package an embedding host drives
// directly.
//
// The caller is declarative and this package is deterministic: the caller
// supplies the persona reasoning (stances, why-chains, votes) from whatever
// model it likes, this package owns the shape and the arithmetic. Nothing here
// calls a model or touches the network — that boundary is the point, and it is
// what makes the room embeddable.
package room

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/redact"
)

// Limits on a session. They are decisions, not preferences: fewer than three
// rounds is not deliberation, more than five is an agent padding a transcript,
// and a why-chain shorter than three steps has not reached a root cause.
const (
	MinRounds   = 3
	MaxRounds   = 5
	MinWhys     = 3
	MinPersonas = 2
	// tieEpsilon is the float slack within which two option weights count as
	// tied; weights are rounded to 6 decimals before comparison.
	tieEpsilon = 1e-9
)

// Session statuses.
const (
	StatusOpen         = "open"
	StatusDeliberating = "deliberating"
	StatusVoting       = "voting"
	StatusDecided      = "decided"
	StatusEscalated    = "escalated"
)

// ErrExists reports a session or verdict that is already on disk; like every
// other huddle writer this package never overwrites one.
var ErrExists = errors.New("already exists")

// Turn is one persona's 5-whys contribution in one round.
type Turn struct {
	Round   int      `json:"round"`
	Persona string   `json:"persona"`
	Stance  string   `json:"stance"`
	Why     []string `json:"why"`
	Depth   int      `json:"depth"`
	Root    string   `json:"root"`
}

// Vote is one persona's final position.
type Vote struct {
	Persona    string  `json:"persona"`
	Position   string  `json:"position"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Session is the manifest of one autonomous huddle. It is the only mutable
// state; the verdict is derived from it.
type Session struct {
	ID            string    `json:"id"`
	Question      string    `json:"question"`
	Owner         string    `json:"owner"`
	Personas      []string  `json:"personas"`
	SpeakingOrder []string  `json:"speakingOrder"`
	Rounds        int       `json:"rounds"`
	Status        string    `json:"status"`
	Created       time.Time `json:"created"`
	Branch        string    `json:"branch,omitempty"`
	Turns         []Turn    `json:"turns"`
	Votes         []Vote    `json:"votes"`
}

// Options are the inputs to New.
type Options struct {
	ID       string
	Question string
	Owner    string
	Personas []string
	Rounds   int
	Branch   string
	Now      time.Time
}

// New validates a room and returns its session manifest. The owner must be a
// member of the room: a decider who did not sit in the deliberation is not a
// decider, it is an oracle.
func New(o Options) (Session, error) {
	if strings.TrimSpace(o.Question) == "" {
		return Session{}, errors.New("a question is required — the room needs something answerable to decide")
	}
	ids := make([]string, 0, len(o.Personas))
	seen := map[string]bool{}
	for _, p := range o.Personas {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if seen[p] {
			return Session{}, fmt.Errorf("persona %q is listed twice — one seat per persona", p)
		}
		seen[p] = true
		ids = append(ids, p)
	}
	if len(ids) < MinPersonas {
		return Session{}, fmt.Errorf("need at least %d personas (an owner and one other voice), got %d", MinPersonas, len(ids))
	}
	owner := strings.TrimSpace(o.Owner)
	if owner == "" {
		return Session{}, errors.New("an owner persona is required — it casts the deciding vote")
	}
	if !seen[owner] {
		return Session{}, fmt.Errorf("owner %q is not in the room (%s) — add it to --personas", owner, strings.Join(ids, ", "))
	}
	rounds := o.Rounds
	if rounds == 0 {
		rounds = MinRounds
	}
	if rounds < MinRounds || rounds > MaxRounds {
		return Session{}, fmt.Errorf("rounds must be %d..%d, got %d", MinRounds, MaxRounds, rounds)
	}
	if strings.TrimSpace(o.ID) == "" {
		return Session{}, errors.New("a session id is required")
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	return Session{
		ID:            strings.TrimSpace(o.ID),
		Question:      redact.String(strings.TrimSpace(o.Question)),
		Owner:         owner,
		Personas:      ids,
		SpeakingOrder: SpeakingOrder(ids, owner),
		Rounds:        rounds,
		Status:        StatusOpen,
		Created:       now.UTC(),
		Branch:        o.Branch,
		Turns:         []Turn{},
		Votes:         []Vote{},
	}, nil
}

// SpeakingOrder puts the owner last: it hears every voice before it weighs in.
func SpeakingOrder(personas []string, owner string) []string {
	out := make([]string, 0, len(personas))
	for _, p := range personas {
		if p != owner {
			out = append(out, p)
		}
	}
	return append(out, owner)
}

// Plan is the one-line deliberation plan printed after New.
func (s Session) Plan() string {
	return fmt.Sprintf("%d personas, %d rounds of 5-whys on %q; owner %q speaks last and decides.",
		len(s.Personas), s.Rounds, s.Question, s.Owner)
}

func (s Session) inRoom(persona string) bool {
	for _, p := range s.Personas {
		if p == persona {
			return true
		}
	}
	return false
}

// deepest returns the greatest depth persona reached before round r, and the
// highest round it has already spoken in.
func (s Session) deepest(persona string, r int) (depth, lastRound int) {
	for _, t := range s.Turns {
		if t.Persona != persona {
			continue
		}
		if t.Round > lastRound {
			lastRound = t.Round
		}
		if t.Round < r && t.Depth > depth {
			depth = t.Depth
		}
	}
	return depth, lastRound
}

// RecordTurn appends one 5-whys turn and enforces the deliberation rules:
// the persona is in the room, the round is in range and is this persona's next
// round, it has not spoken in that round, the chain reaches a root, and the
// round digs at least as deep as the persona's own previous rounds.
func (s *Session) RecordTurn(round int, persona, stance string, whys []string) (Turn, error) {
	persona = strings.TrimSpace(persona)
	if !s.inRoom(persona) {
		return Turn{}, fmt.Errorf("persona %q is not in this room (%s)", persona, strings.Join(s.Personas, ", "))
	}
	if round < 1 || round > s.Rounds {
		return Turn{}, fmt.Errorf("round must be 1..%d, got %d", s.Rounds, round)
	}
	chain := make([]string, 0, len(whys))
	for _, w := range whys {
		if w = strings.TrimSpace(w); w != "" {
			chain = append(chain, redact.String(w))
		}
	}
	if len(chain) < MinWhys {
		return Turn{}, fmt.Errorf("a 5-whys turn needs at least %d deepening steps (why → why → root), got %d", MinWhys, len(chain))
	}
	prevDepth, lastRound := s.deepest(persona, round)
	if lastRound >= round {
		return Turn{}, fmt.Errorf("%q already spoke in round %d — a round is recorded once", persona, round)
	}
	// Rounds are sequential per persona. The Python original only compared
	// against earlier rounds, so recording a deep round 3 and then a shallow
	// round 2 slipped past the monotonicity rule; requiring the next round
	// closes that hole.
	if lastRound != round-1 {
		return Turn{}, fmt.Errorf("%q is on round %d next, not %d — rounds deepen in order", persona, lastRound+1, round)
	}
	if len(chain) < prevDepth {
		return Turn{}, fmt.Errorf("round %d for %q is shallower (%d whys) than its round %d (%d) — each round must dig deeper",
			round, persona, len(chain), round-1, prevDepth)
	}
	t := Turn{
		Round:   round,
		Persona: persona,
		Stance:  redact.String(strings.TrimSpace(stance)),
		Why:     chain,
		Depth:   len(chain),
		Root:    chain[len(chain)-1],
	}
	s.Turns = append(s.Turns, t)
	if s.Status == StatusOpen {
		s.Status = StatusDeliberating
	}
	return t, nil
}

// RecordVote appends one final vote. One vote per persona, confidence in 0..1.
func (s *Session) RecordVote(persona, position string, confidence float64, reason string) (Vote, error) {
	persona = strings.TrimSpace(persona)
	if !s.inRoom(persona) {
		return Vote{}, fmt.Errorf("persona %q is not in this room (%s)", persona, strings.Join(s.Personas, ", "))
	}
	position = strings.TrimSpace(position)
	if position == "" {
		return Vote{}, errors.New("a vote needs --position (the option being backed)")
	}
	if confidence < 0 || confidence > 1 {
		return Vote{}, fmt.Errorf("confidence must be 0..1, got %g", confidence)
	}
	for _, v := range s.Votes {
		if v.Persona == persona {
			return Vote{}, fmt.Errorf("%q already voted for %q — a vote is cast once", persona, v.Position)
		}
	}
	if !s.DeliberationComplete() {
		return Vote{}, fmt.Errorf("deliberation is not complete (%d of %d turns recorded) — run every round before voting",
			len(s.Turns), s.Rounds*len(s.Personas))
	}
	v := Vote{Persona: persona, Position: redact.String(position), Confidence: confidence, Reason: redact.String(strings.TrimSpace(reason))}
	s.Votes = append(s.Votes, v)
	s.Status = StatusVoting
	return v, nil
}

// DeliberationComplete reports whether every persona spoke in every round.
func (s Session) DeliberationComplete() bool {
	return len(s.Turns) == s.Rounds*len(s.Personas)
}

// DidNotVote lists room members with no vote recorded.
func (s Session) DidNotVote() []string {
	voted := map[string]bool{}
	for _, v := range s.Votes {
		voted[v.Persona] = true
	}
	var out []string
	for _, p := range s.Personas {
		if !voted[p] {
			out = append(out, p)
		}
	}
	return out
}

// RoundTrail is one round of the rationale trail, in speaking order.
type RoundTrail struct {
	Round int    `json:"round"`
	Turns []Turn `json:"turns"`
}

// Trail groups the recorded turns by round, each round in speaking order.
func (s Session) Trail() []RoundTrail {
	order := map[string]int{}
	for i, p := range s.SpeakingOrder {
		order[p] = i
	}
	byRound := map[int][]Turn{}
	var rounds []int
	for _, t := range s.Turns {
		if _, ok := byRound[t.Round]; !ok {
			rounds = append(rounds, t.Round)
		}
		byRound[t.Round] = append(byRound[t.Round], t)
	}
	sort.Ints(rounds)
	out := make([]RoundTrail, 0, len(rounds))
	for _, r := range rounds {
		turns := byRound[r]
		sort.SliceStable(turns, func(i, j int) bool { return order[turns[i].Persona] < order[turns[j].Persona] })
		out = append(out, RoundTrail{Round: r, Turns: turns})
	}
	return out
}
