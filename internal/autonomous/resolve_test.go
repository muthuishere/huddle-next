package autonomous

import (
	"strings"
	"testing"
)

func votes(spec ...any) []Vote {
	var out []Vote
	for i := 0; i+2 < len(spec); i += 3 {
		out = append(out, Vote{
			Persona:    spec[i].(string),
			Position:   spec[i+1].(string),
			Confidence: spec[i+2].(float64),
			Reason:     "because " + spec[i+1].(string),
		})
	}
	return out
}

func TestResolveOwnerIsTheDecidingVote(t *testing.T) {
	cases := []struct {
		name       string
		votes      []Vote
		resolved   string
		overridden bool
		dissents   int
	}{
		{
			// Owner alone at the top: its call stands.
			name:     "owner leads",
			votes:    votes("arasan", "A", 0.9, "nina", "B", 0.4),
			resolved: "A", dissents: 1,
		},
		{
			// Exact tie at the top — the owner breaks it toward itself.
			name:     "tie broken toward the owner",
			votes:    votes("arasan", "A", 0.6, "nina", "B", 0.3, "shaama", "B", 0.3),
			resolved: "A", dissents: 2,
		},
		{
			// Owner is behind, but two options tie at the top, so the room is
			// not decisive and the owner still decides.
			name:     "owner behind an indecisive room",
			votes:    votes("arasan", "A", 0.2, "nina", "B", 0.7, "shaama", "C", 0.7),
			resolved: "A", dissents: 2,
		},
		{
			// One option strictly dominates: the room overrides, loudly.
			name:     "dominant room overrides the owner",
			votes:    votes("arasan", "A", 0.5, "nina", "B", 0.9, "shaama", "B", 0.8),
			resolved: "B", overridden: true, dissents: 1,
		},
		{
			name:     "unanimous room, no dissent",
			votes:    votes("arasan", "A", 0.5, "nina", "A", 0.5),
			resolved: "A", dissents: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Resolve(tc.votes, "arasan")
			if err != nil {
				t.Fatal(err)
			}
			if res.Resolved != tc.resolved {
				t.Errorf("resolved = %q, want %q (weights %v)", res.Resolved, tc.resolved, res.Weights)
			}
			if res.OwnerOverridden != tc.overridden {
				t.Errorf("ownerOverridden = %t, want %t", res.OwnerOverridden, tc.overridden)
			}
			if len(res.Dissents) != tc.dissents {
				t.Errorf("dissents = %d, want %d (%v)", len(res.Dissents), tc.dissents, res.Dissents)
			}
			if res.OwnerPosition != "A" {
				t.Errorf("ownerPosition = %q, want A", res.OwnerPosition)
			}
			for _, d := range res.Dissents {
				if d.Position == res.Resolved || d.Reason == "" {
					t.Errorf("dissent %+v is not a dissent against %q, or lost its reason", d, res.Resolved)
				}
			}
		})
	}
}

func TestResolveFloatSumsAreNotTiedByRoundingNoise(t *testing.T) {
	// 0.1+0.2 == 0.30000000000000004 in binary floating point; rounding to six
	// decimals must make it an exact tie with a single 0.3 vote, which the
	// owner then wins.
	res, err := Resolve(votes("arasan", "A", 0.3, "nina", "B", 0.1, "shaama", "B", 0.2), "arasan")
	if err != nil {
		t.Fatal(err)
	}
	if res.Resolved != "A" || res.OwnerOverridden {
		t.Fatalf("resolved %q overridden %t — a rounding artefact overrode the owner (weights %v)",
			res.Resolved, res.OwnerOverridden, res.Weights)
	}
}

func TestResolveWeightsTableIsOrderedAndCounted(t *testing.T) {
	res, err := Resolve(votes("arasan", "A", 0.2, "nina", "B", 0.4, "shaama", "B", 0.4, "maya", "C", 0.3), "arasan")
	if err != nil {
		t.Fatal(err)
	}
	if res.Weights[0].Option != "B" || res.Weights[0].Weight != 0.8 || res.Weights[0].Voters != 2 {
		t.Fatalf("top row = %+v, want B weight 0.8 with 2 voters", res.Weights[0])
	}
	if res.TopWeight != 0.8 {
		t.Errorf("topWeight = %v, want 0.8", res.TopWeight)
	}
	if res.Resolved != "B" || !res.OwnerOverridden {
		t.Errorf("resolved %q overridden %t, want B overridden", res.Resolved, res.OwnerOverridden)
	}
}

func TestResolveRefusesWithoutTheOwner(t *testing.T) {
	if _, err := Resolve(nil, "arasan"); err == nil || !strings.Contains(err.Error(), "no votes") {
		t.Fatalf("empty votes: %v", err)
	}
	_, err := Resolve(votes("nina", "A", 0.9), "arasan")
	if err == nil || !strings.Contains(err.Error(), "has not voted") {
		t.Fatalf("a room without the owner's vote cannot be resolved, got %v", err)
	}
}

func TestForkCheckEscalatesEveryOwnerLevelFlag(t *testing.T) {
	for _, flag := range ForkFlagNames() {
		f := ForkCheck("a decision with no fork language", []string{flag})
		if !f.Escalate || len(f.Reasons) != 1 || !strings.HasPrefix(f.Reasons[0], flag+": ") {
			t.Errorf("flag %q: escalate %t reasons %v", flag, f.Escalate, f.Reasons)
		}
		if len(f.Detected) != 0 {
			t.Errorf("flag %q was declared, not detected: %v", flag, f.Detected)
		}
	}
	clean := ForkCheck("rename the internal helper", nil)
	if clean.Escalate || len(clean.Flags) != 0 {
		t.Errorf("a reversible decision must not escalate: %+v", clean)
	}
	unknown := ForkCheck("rename the internal helper", []string{"vibes", "launch"})
	if !unknown.Escalate || len(unknown.Unknown) != 1 || unknown.Unknown[0] != "vibes" {
		t.Errorf("unknown flags must be reported, not honoured: %+v", unknown)
	}
	dup := ForkCheck("rename the internal helper", []string{"spend", "spend", " SPEND "})
	if len(dup.Flags) != 1 {
		t.Errorf("flags should dedupe case-insensitively: %v", dup.Flags)
	}
}

func TestForkCheckDetectsForkLanguageTheAgentForgotToFlag(t *testing.T) {
	cases := map[string]string{
		"Launch the pricing page to customers on Friday": "launch",
		"Spend $400 on the GPU box":                      "spend",
		"Delete production and restore from backup":      "irreversible",
		"Accept the new licence terms":                   "legal",
		"Rotate the API key and store the secret in CI":  "security",
		"Pivot the product to a different market":        "scope",
	}
	for decision, want := range cases {
		f := ForkCheck(decision, nil)
		if !f.Escalate || !contains(f.Flags, want) {
			t.Errorf("%q: flags %v, want %q", decision, f.Flags, want)
		}
		if !contains(f.Detected, want) {
			t.Errorf("%q: %q should be marked as detected, got %v", decision, want, f.Detected)
		}
		if len(f.Reasons) == 0 || !strings.Contains(strings.Join(f.Reasons, " "), "detected in the decision") {
			t.Errorf("%q: reasons should say it was detected: %v", decision, f.Reasons)
		}
	}
	// A declared flag is not re-reported as detected.
	f := ForkCheck("Launch the pricing page", []string{"launch"})
	if len(f.Flags) != 1 || len(f.Detected) != 0 {
		t.Errorf("declared + detected should collapse to one flag: %+v", f)
	}
}
