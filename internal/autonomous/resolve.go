package autonomous

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Dissent is a persona that landed against the resolved option. Dissents are
// recorded, never erased — a recorded dissent is cheaper than a silent one.
type Dissent struct {
	Persona  string `json:"persona"`
	Position string `json:"position"`
	Reason   string `json:"reason"`
}

// OptionWeight is one option and the summed confidence behind it.
type OptionWeight struct {
	Option string  `json:"option"`
	Weight float64 `json:"weight"`
	Voters int     `json:"voters"`
}

// Resolution is the outcome of the owner-weighted vote.
type Resolution struct {
	Resolved        string         `json:"resolved"`
	OwnerPosition   string         `json:"ownerPosition"`
	OwnerOverridden bool           `json:"ownerOverridden"`
	Weights         []OptionWeight `json:"weights"`
	TopWeight       float64        `json:"topWeight"`
	Dissents        []Dissent      `json:"dissents"`
}

// Resolve resolves the vote through the owner's lens. Pure and deterministic.
//
// weight(option) = sum of its voters' confidences. The resolved option is the
// owner's position unless a *single* non-owner option strictly dominates it —
// unique top of the table and heavier than the owner's option by more than the
// tie epsilon. Only then does the room override the owner, and the override is
// recorded. A tie at the top is broken toward the owner.
func Resolve(votes []Vote, owner string) (Resolution, error) {
	if len(votes) == 0 {
		return Resolution{}, errors.New("no votes recorded yet")
	}
	ownerPos := ""
	for _, v := range votes {
		if v.Persona == owner {
			ownerPos = v.Position
			break
		}
	}
	if ownerPos == "" {
		return Resolution{}, fmt.Errorf("owner %q has not voted — the deciding vote cannot be inferred", owner)
	}
	weight := map[string]float64{}
	voters := map[string]int{}
	var order []string
	for _, v := range votes {
		if _, ok := weight[v.Position]; !ok {
			order = append(order, v.Position)
		}
		weight[v.Position] = round6(weight[v.Position] + v.Confidence)
		voters[v.Position]++
	}
	top := math.Inf(-1)
	for _, w := range weight {
		if w > top {
			top = w
		}
	}
	var atTop []string
	for _, opt := range order {
		if math.Abs(weight[opt]-top) <= tieEpsilon {
			atTop = append(atTop, opt)
		}
	}
	resolved, overridden := ownerPos, false
	switch {
	case contains(atTop, ownerPos):
		// Owner is at the top (alone or tied) — the owner's call stands.
	case len(atTop) == 1 && top-weight[ownerPos] > tieEpsilon:
		resolved, overridden = atTop[0], true // the room is decisively against the owner
	}
	table := make([]OptionWeight, 0, len(order))
	for _, opt := range order {
		table = append(table, OptionWeight{Option: opt, Weight: weight[opt], Voters: voters[opt]})
	}
	sort.SliceStable(table, func(i, j int) bool {
		if table[i].Weight != table[j].Weight {
			return table[i].Weight > table[j].Weight
		}
		return table[i].Option < table[j].Option
	})
	dissents := []Dissent{}
	for _, v := range votes {
		if v.Position != resolved {
			dissents = append(dissents, Dissent{Persona: v.Persona, Position: v.Position, Reason: v.Reason})
		}
	}
	return Resolution{
		Resolved:        resolved,
		OwnerPosition:   ownerPos,
		OwnerOverridden: overridden,
		Weights:         table,
		TopWeight:       top,
		Dissents:        dissents,
	}, nil
}

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ForkFlags are the owner-level forks. A decision carrying any of them is the
// real human's to make: the huddle stages it instead of acting.
var ForkFlags = map[string]string{
	"launch":       "customer- or public-facing go-live",
	"spend":        "money is committed",
	"irreversible": "hard or impossible to undo",
	"legal":        "legal or compliance exposure",
	"security":     "trust-boundary or security exposure",
	"scope":        "materially changes direction or a product's identity",
}

// ForkFlagNames lists the flags in a stable order for help text and errors.
func ForkFlagNames() []string {
	out := make([]string, 0, len(ForkFlags))
	for f := range ForkFlags {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// forkWords maps a flag to the lowercase substrings that, in a decision's own
// wording, mean the decision is that kind of fork. Detection is a safety net
// for an agent that forgot its flags, and it only ever adds escalation: a false
// escalation costs a message to a human, a missed one costs a launch.
var forkWords = map[string][]string{
	"launch":       {"launch", "go live", "go-live", "ship to customers", "public release", "announce publicly"},
	"spend":        {"spend", "pay ", "purchase", "buy ", "budget", "invoice", "subscribe", "$"},
	"irreversible": {"irreversible", "cannot be undone", "can't be undone", "one-way door", "delete production", "drop the database", "unrecoverable"},
	"legal":        {"legal", "contract", "licence", "license terms", "gdpr", "compliance", "terms of service"},
	"security":     {"security", "credential", "secret", "api key", "auth bypass", "disable the gate", "permission model"},
	"scope":        {"pivot", "change direction", "rebrand", "out of scope", "new product line", "deprecate the product"},
}

// Fork is the result of the owner-level fork check.
type Fork struct {
	Escalate bool     `json:"escalate"`
	Flags    []string `json:"flags"`
	Detected []string `json:"detected,omitempty"`
	Reasons  []string `json:"reasons"`
	Unknown  []string `json:"unknownFlags,omitempty"`
}

// ForkCheck applies the escalation policy to the flags an agent declared plus
// the flags the decision's own wording implies.
func ForkCheck(decision string, flags []string) Fork {
	f := Fork{Flags: []string{}, Reasons: []string{}}
	seen := map[string]bool{}
	for _, raw := range flags {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if _, ok := ForkFlags[name]; !ok {
			f.Unknown = append(f.Unknown, name)
			continue
		}
		if !seen[name] {
			seen[name] = true
			f.Flags = append(f.Flags, name)
		}
	}
	for _, name := range DetectForks(decision) {
		if !seen[name] {
			seen[name] = true
			f.Flags = append(f.Flags, name)
			f.Detected = append(f.Detected, name)
		}
	}
	sort.Strings(f.Flags)
	sort.Strings(f.Detected)
	for _, name := range f.Flags {
		reason := name + ": " + ForkFlags[name]
		if contains(f.Detected, name) {
			reason += " (detected in the decision's wording)"
		}
		f.Reasons = append(f.Reasons, reason)
	}
	f.Escalate = len(f.Flags) > 0
	return f
}

// DetectForks returns the fork flags implied by a decision's wording.
func DetectForks(decision string) []string {
	text := strings.ToLower(decision)
	var out []string
	for _, name := range ForkFlagNames() {
		for _, w := range forkWords[name] {
			if strings.Contains(text, w) {
				out = append(out, name)
				break
			}
		}
	}
	return out
}
