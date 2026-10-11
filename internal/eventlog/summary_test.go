package eventlog

import "testing"

// Events the CLI appends itself carry no free text — their meaning is in
// Artifact, Data and Refs. Summary must render a line for every one of them,
// or `huddle log list` and `huddle timeline` show the user a blank column.
func TestSummaryRendersStructuredEvents(t *testing.T) {
	cases := []struct {
		name string
		e    Event
		want string
	}{
		{"free text wins", Event{Type: "decision", Text: "keep the\n event log append-only"}, "keep the event log append-only"},
		{
			"artifact.written",
			Event{Type: "artifact.written", Refs: []string{"intent:v1-readiness"},
				Artifact: &ArtifactRef{Kind: "intent", Path: "intent/v1-readiness/intent.md", SHA256: "deadbeef"},
				Data:     map[string]any{"id": "intent:v1-readiness", "action": "new"}},
			"intent intent/v1-readiness/intent.md · action=new",
		},
		{
			"gate.request",
			Event{Type: "gate.request", Refs: []string{"spec:login"},
				Artifact: &ArtifactRef{Kind: "spec", Path: "intent/login/spec.md"},
				Data:     map[string]any{"gate": "g7", "name": "spec", "artifactId": "spec:login", "role": "tech-lead"}},
			"spec intent/login/spec.md · gate=g7 · name=spec · role=tech-lead",
		},
		{
			"verify",
			Event{Type: "verify", Artifact: &ArtifactRef{Kind: "plan", Path: "intent/x/plan.md"},
				Data: map[string]any{"verdict": "PASS", "mode": "full", "fingerprint": "abc123", "findingsCount": float64(0)}},
			"plan intent/x/plan.md · verdict=PASS · mode=full · findingsCount=0",
		},
		{"refs only", Event{Type: "persona.created", Refs: []string{"vidya"}}, "refs vidya"},
		{"nothing to say", Event{Type: "milestone"}, ""},
	}
	for _, tc := range cases {
		if got := tc.e.Summary(); got != tc.want {
			t.Errorf("%s: Summary() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Composite Data values are left out rather than dumped into a one-line summary.
func TestSummarySkipsCompositeValues(t *testing.T) {
	e := Event{Type: "verify", Data: map[string]any{
		"findings": []any{map[string]any{"id": "f1"}},
		"nested":   map[string]any{"a": 1},
		"verdict":  "FAIL",
	}}
	if got := e.Summary(); got != "verdict=FAIL" {
		t.Fatalf("Summary() = %q", got)
	}
}
