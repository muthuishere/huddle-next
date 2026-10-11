package autonomous

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/redact"
)

// Escalation records why a verdict is the real human's to close.
type Escalation struct {
	Required bool     `json:"required"`
	Flags    []string `json:"flags"`
	Detected []string `json:"detected,omitempty"`
	Reasons  []string `json:"reasons"`
}

// Verdict is the actionable (or staged) output of one autonomous huddle.
type Verdict struct {
	Session    string       `json:"session"`
	Question   string       `json:"question"`
	Owner      string       `json:"owner"`
	Personas   []string     `json:"personas"`
	Rounds     int          `json:"rounds"`
	Decision   string       `json:"decision"`
	Summary    string       `json:"summary,omitempty"`
	Status     string       `json:"status"` // decided | escalated
	Actionable bool         `json:"actionable"`
	Tally      Resolution   `json:"tally"`
	Votes      []Vote       `json:"votes"`
	Trail      []RoundTrail `json:"rationaleTrail"`
	Escalation Escalation   `json:"escalation"`
	Decided    time.Time    `json:"decided"`
}

// BuildVerdict assembles the verdict from the session. Pure: it writes nothing,
// so `--dry-run` and the real write share one code path.
func BuildVerdict(s Session, decision, summary string, flags []string, now time.Time) (Verdict, error) {
	if !s.DeliberationComplete() {
		return Verdict{}, fmt.Errorf("deliberation is not complete (%d of %d turns) — every persona speaks in every round before a verdict",
			len(s.Turns), s.Rounds*len(s.Personas))
	}
	if missing := s.DidNotVote(); len(missing) > 0 {
		return Verdict{}, fmt.Errorf("%s have not voted — every persona votes before the verdict", strings.Join(missing, ", "))
	}
	res, err := Resolve(s.Votes, s.Owner)
	if err != nil {
		return Verdict{}, err
	}
	decision = redact.String(strings.TrimSpace(decision))
	if decision == "" {
		decision = res.Resolved
	}
	fork := ForkCheck(decision, flags)
	if now.IsZero() {
		now = time.Now()
	}
	status := StatusDecided
	if fork.Escalate {
		status = StatusEscalated
	}
	return Verdict{
		Session:    s.ID,
		Question:   s.Question,
		Owner:      s.Owner,
		Personas:   s.Personas,
		Rounds:     s.Rounds,
		Decision:   decision,
		Summary:    redact.String(strings.TrimSpace(summary)),
		Status:     status,
		Actionable: status == StatusDecided,
		Tally:      res,
		Votes:      s.Votes,
		Trail:      s.Trail(),
		Escalation: Escalation{Required: fork.Escalate, Flags: fork.Flags, Detected: fork.Detected, Reasons: fork.Reasons},
		Decided:    now.UTC(),
	}, nil
}

// Markdown renders the human-readable verdict.
func Markdown(v Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Verdict — %s\n\n", v.Question)
	fmt.Fprintf(&b, "- Decision: %s\n", v.Decision)
	fmt.Fprintf(&b, "- Resolved option: %s\n", v.Tally.Resolved)
	owner := v.Owner
	if v.Tally.OwnerOverridden {
		owner += " — overridden by a decisively dominant room"
	}
	fmt.Fprintf(&b, "- Owner (decider): %s\n", owner)
	next := "staged for the human owner — do not act"
	if v.Actionable {
		next = "act on it headlessly"
	}
	fmt.Fprintf(&b, "- Status: %s (%s)\n", v.Status, next)
	fmt.Fprintf(&b, "- Session: %s · %d personas · %d rounds\n", v.Session, len(v.Personas), v.Rounds)
	if v.Summary != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", v.Summary)
	}
	if v.Escalation.Required {
		b.WriteString("\n> OWNER-LEVEL FORK — escalate to the human owner. Do not act.\n")
		for _, r := range v.Escalation.Reasons {
			fmt.Fprintf(&b, "> - %s\n", r)
		}
	}
	b.WriteString("\n## Vote tally\n\n")
	for _, w := range v.Tally.Weights {
		mark := ""
		if w.Option == v.Tally.Resolved {
			mark = " ← resolved"
		}
		fmt.Fprintf(&b, "- %s — weight %s, %d voter(s)%s\n", w.Option, strconv.FormatFloat(w.Weight, 'f', -1, 64), w.Voters, mark)
	}
	b.WriteString("\n## Dissents\n\n")
	if len(v.Tally.Dissents) == 0 {
		b.WriteString("- none — the room aligned\n")
	}
	for _, d := range v.Tally.Dissents {
		fmt.Fprintf(&b, "- %s (%s): %s\n", d.Persona, d.Position, d.Reason)
	}
	b.WriteString("\n## 5-whys rationale trail\n")
	for _, r := range v.Trail {
		fmt.Fprintf(&b, "\n### Round %d\n\n", r.Round)
		for _, t := range r.Turns {
			fmt.Fprintf(&b, "- %s [%s]: %s\n", t.Persona, t.Stance, strings.Join(t.Why, " → "))
		}
	}
	return b.String()
}
