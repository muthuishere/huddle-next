package app

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/eventlog"
)

// autoWhys returns a chain of n deepening steps.
func autoWhys(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "why"
	}
	return strings.Join(parts, "; ")
}

// autoDeliberate runs every persona through every round via the CLI.
func autoDeliberate(t *testing.T, root, sid string, rounds int, personas ...string) {
	t.Helper()
	for r := 1; r <= rounds; r++ {
		for _, p := range personas {
			if _, err := runCore(t, root, "autonomous", "turn", sid,
				"--round", strconv.Itoa(r), "--persona", p, "--stance", "stance", "--why", autoWhys(2+r)); err != nil {
				t.Fatalf("turn r%d %s: %v", r, p, err)
			}
		}
	}
}

// A headless run: open the room, deliberate, vote, verdict — and the verdict
// is actionable, written once, and recorded in the audit log.
func TestAutonomousHeadlessRunToAnActionableVerdict(t *testing.T) {
	root := coreEnv(t)
	out, err := runCore(t, root, "autonomous", "new", "--question", "Do we cache the index?",
		"--personas", "arasan,nina,shaama", "--rounds", "3", "--id", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "OWNER: arasan") || !strings.Contains(out, "nina → shaama → arasan") {
		t.Fatalf("new should name the owner and put it last:\n%s", out)
	}
	autoDeliberate(t, root, "s1", 3, "nina", "shaama", "arasan")
	if out, err := runCore(t, root, "autonomous", "trail", "s1"); err != nil || !strings.Contains(out, "COMPLETE: true") {
		t.Fatalf("trail = %q (%v)", out, err)
	}
	for _, v := range [][3]string{{"nina", "cache", "0.6"}, {"shaama", "measure", "0.9"}, {"arasan", "cache", "0.8"}} {
		if _, err := runCore(t, root, "autonomous", "vote", "s1", "--persona", v[0],
			"--position", v[1], "--confidence", v[2], "--reason", "r"); err != nil {
			t.Fatalf("vote %s: %v", v[0], err)
		}
	}
	tally, err := runCore(t, root, "autonomous", "tally", "s1")
	if err != nil || !strings.Contains(tally, "RESOLVED: cache") || !strings.Contains(tally, "DISSENT: shaama") {
		t.Fatalf("tally = %q (%v)", tally, err)
	}
	dry, err := runCore(t, root, "autonomous", "verdict", "s1", "--decision", "Cache behind a flag", "--dry-run")
	if err != nil || !strings.Contains(dry, "DRY RUN") || !strings.Contains(dry, "# Verdict") {
		t.Fatalf("dry run = %q (%v)", dry, err)
	}
	if list, _ := runCore(t, root, "autonomous", "list"); strings.Contains(list, "decided") {
		t.Fatalf("a dry run must not change the session:\n%s", list)
	}
	final, err := runCore(t, root, "autonomous", "verdict", "s1", "--decision", "Cache behind a flag", "--summary", "reversible")
	if err != nil || !strings.Contains(final, "STATUS: decided") || !strings.Contains(final, "ACTIONABLE: true") {
		t.Fatalf("verdict = %q (%v)", final, err)
	}
	if _, err := runCore(t, root, "autonomous", "verdict", "s1", "--decision", "again"); err == nil ||
		!strings.Contains(err.Error(), "written once") {
		t.Fatalf("a second verdict must be refused, got %v", err)
	}
	evs, err := eventlog.List("proj", eventlog.Filter{Types: map[string]bool{"autonomous.decided": true}})
	if err != nil || len(evs) != 1 || evs[0].Door != "two-way" {
		t.Fatalf("audit log = %+v (%v)", evs, err)
	}
	if show, _ := runCore(t, root, "autonomous", "show", "s1"); !strings.Contains(show, "STATUS: decided") {
		t.Errorf("show = %q", show)
	}
}

// An owner-level fork stages instead of deciding, and says so in the log.
func TestAutonomousForkEscalatesInsteadOfActing(t *testing.T) {
	root := coreEnv(t)
	if _, err := runCore(t, root, "autonomous", "new", "--question", "Ship pricing?",
		"--personas", "nina", "--id", "s2"); err != nil {
		t.Fatal(err) // the owner is added for us
	}
	autoDeliberate(t, root, "s2", 3, "nina", "arasan")
	for _, p := range []string{"nina", "arasan"} {
		if _, err := runCore(t, root, "autonomous", "vote", "s2", "--persona", p,
			"--position", "ship", "--confidence", "0.9", "--reason", "r"); err != nil {
			t.Fatal(err)
		}
	}
	out, err := runCore(t, root, "autonomous", "verdict", "s2", "--decision", "Go live with public pricing")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "STATUS: escalated") || !strings.Contains(out, "ACTIONABLE: false") ||
		!strings.Contains(out, "do not act") {
		t.Fatalf("an owner-level fork must stage for the human:\n%s", out)
	}
	evs, err := eventlog.List("proj", eventlog.Filter{Types: map[string]bool{"autonomous.escalated": true}})
	if err != nil || len(evs) != 1 || evs[0].Door != "one-way" {
		t.Fatalf("an escalation is a one-way door in the log: %+v (%v)", evs, err)
	}
}

func TestAutonomousUsageErrors(t *testing.T) {
	root := coreEnv(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"autonomous", "new", "--personas", "arasan,nina"}, "--question"},
		{[]string{"autonomous", "new", "--question", "q", "--personas", "nina,maya", "--owner", "arasan", "--rounds", "9"}, "rounds must be 3..5"},
		{[]string{"autonomous", "new", "--question", "q", "--personas", "arasan", "--rounds", "3"}, "at least 2 personas"},
		{[]string{"autonomous", "turn", "nope", "--round", "1", "--persona", "nina", "--why", "a;b;c"}, "no autonomous session"},
		{[]string{"autonomous", "wat"}, "unknown autonomous verb"},
	}
	for _, tc := range cases {
		if _, err := runCore(t, root, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v → %v, want %q", tc.args, err, tc.want)
		}
	}
	// fork-check is stateless: it needs no session and no repo state.
	out, err := runCore(t, root, "autonomous", "fork-check", "--decision", "Spend $500 on ads", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Escalate bool     `json:"escalate"`
		Flags    []string `json:"flags"`
	}
	if err := json.Unmarshal([]byte(out), &f); err != nil {
		t.Fatal(err)
	}
	if !f.Escalate || len(f.Flags) == 0 {
		t.Errorf("fork-check = %+v", f)
	}
	if help, err := runCore(t, root, "autonomous", "--help"); err != nil || !strings.Contains(help, "5-whys") {
		t.Errorf("help = %q (%v)", help, err)
	}
}
