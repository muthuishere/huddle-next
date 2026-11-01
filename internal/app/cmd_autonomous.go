package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/repo"
	"github.com/muthuishere/huddle-next/room"
)

func init() {
	register(Command{
		Name:    "autonomous",
		Summary: "Headless owner-decided huddle: 5-whys rounds, owner-weighted vote, verdict",
		Run:     runAutonomous,
	})
}

// DefaultOwner is the owner-aligned decider persona used when none is named.
const DefaultOwner = "arasan"

var autonomousHelp = `
Usage: huddle autonomous <verb> [options]

A headless huddle that reaches a verdict with no human stop. The agent supplies
the persona reasoning; this command owns the session, the 5-whys rules, the
owner-weighted decision math and the verdict. Owner-level forks still escalate.

Verbs:
  new --question "Q" --personas a,b,c [--owner ` + DefaultOwner + `] [--rounds 3] [--id <sid>] [--dry-run]
        open a room: >= 2 personas, the owner among them, rounds ` + strconv.Itoa(room.MinRounds) + `..` + strconv.Itoa(room.MaxRounds) + `
  turn <sid> --round N --persona <id> --why "first; deeper; root" [--stance "one line"]
        record one 5-whys turn; >= ` + strconv.Itoa(room.MinWhys) + ` steps, in round order, never shallower than before
  trail <sid>                              the rationale trail so far, by round
  vote <sid> --persona <id> --position "<option>" --confidence 0..1 [--reason "why"]
  tally <sid>                              resolve through the owner's lens (no side effects)
  fork-check --decision "<text>" [--flags launch,spend,...]
        owner-level fork policy: ` + strings.Join(room.ForkFlagNames(), ", ") + `
  verdict <sid> --decision "<text>" [--flags ...] [--summary "<one line>"] [--dry-run]
        write verdict.json + verdict.md; status decided (act) or escalated (stage)
  list | show <sid>                        sessions and one session's manifest

Add --json to any verb for machine output.
`

func runAutonomous(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return coreHelp(c, autonomousHelp)
	}
	switch args[0] {
	case "new":
		return autoNew(c, args[1:])
	case "turn":
		return autoTurn(c, args[1:])
	case "trail":
		return autoTrail(c, args[1:])
	case "vote":
		return autoVote(c, args[1:])
	case "tally":
		return autoTally(c, args[1:])
	case "fork-check":
		return autoForkCheck(c, args[1:])
	case "verdict":
		return autoVerdict(c, args[1:])
	case "list":
		return autoList(c, args[1:])
	case "show":
		return autoShow(c, args[1:])
	}
	return Usagef("unknown autonomous verb %q — run: huddle autonomous --help", args[0])
}

// autoRoot is the per-repo session root. Every global path comes from
// internal/paths; this only names the subdirectory.
func autoRoot(c *Ctx) (repo.Info, string, error) {
	info, err := c.Repo()
	if err != nil {
		return repo.Info{}, "", err
	}
	return info, filepath.Join(paths.RepoState(info.Name), "autonomous"), nil
}

func autoSessionID(a coreArgs) string {
	if len(a.pos) > 0 {
		return strings.TrimSpace(a.pos[0])
	}
	return ""
}

func autoNew(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args,
		[]string{"question", "owner", "personas", "rounds", "id"}, []string{"json", "dry-run"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	rounds, err := a.intValue("rounds", room.MinRounds)
	if err != nil {
		return err
	}
	question := strings.TrimSpace(a.values["question"])
	if question == "" {
		return Usagef(`huddle autonomous new --question "<the decision>" --personas a,b,c [--owner %s]`, DefaultOwner)
	}
	owner := strings.TrimSpace(a.values["owner"])
	if owner == "" {
		owner = DefaultOwner
	}
	personas := coreSplitCSV(a.values["personas"])
	if len(personas) == 0 {
		personas = []string{owner}
	}
	if !autoHas(personas, owner) {
		personas = append(personas, owner)
	}
	info, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(a.values["id"])
	if id == "" {
		id = time.Now().UTC().Format("20060102T150405") + "-" + artifacts.Slugify(question)
	}
	s, err := room.New(room.Options{
		ID: paths.SafeName(id), Question: question, Owner: owner,
		Personas: personas, Rounds: rounds, Branch: info.Branch, Now: time.Now(),
	})
	if err != nil {
		return err
	}
	out := map[string]any{
		"session": s.ID, "owner": s.Owner, "rounds": s.Rounds,
		"personas": s.Personas, "speakingOrder": s.SpeakingOrder, "plan": s.Plan(),
		"dryRun": a.bools["dry-run"],
	}
	if !a.bools["dry-run"] {
		dir, err := room.Create(root, s)
		if err != nil {
			if errors.Is(err, room.ErrExists) {
				return fmt.Errorf("%w — pick another --id; a room is never silently reset", err)
			}
			return err
		}
		out["dir"] = dir
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, out)
	}
	output.KV(c.Stdout, "SESSION", s.ID, "OWNER", s.Owner, "ROUNDS", strconv.Itoa(s.Rounds),
		"ORDER", strings.Join(s.SpeakingOrder, " → "), "PLAN", s.Plan())
	if a.bools["dry-run"] {
		output.KV(c.Stdout, "DRY RUN", "nothing written")
	}
	return nil
}

func autoHas(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func autoTurn(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args,
		[]string{"round", "persona", "why", "stance"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" || a.values["persona"] == "" || a.values["why"] == "" {
		return Usagef(`huddle autonomous turn <sid> --round N --persona <id> --why "first; deeper; root"`)
	}
	round, err := a.intValue("round", 0)
	if err != nil {
		return err
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	t, err := s.RecordTurn(round, a.values["persona"], a.values["stance"], strings.Split(a.values["why"], ";"))
	if err != nil {
		return err
	}
	if err := room.Save(root, s); err != nil {
		return err
	}
	done := fmt.Sprintf("%d/%d", len(s.Turns), s.Rounds*len(s.Personas))
	if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"recorded": t, "turns": done, "complete": s.DeliberationComplete()})
	}
	output.KV(c.Stdout, "TURN", fmt.Sprintf("round %d · %s · depth %d", t.Round, t.Persona, t.Depth),
		"ROOT", t.Root, "TURNS", done)
	return nil
}

func autoTrail(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" {
		return Usagef("huddle autonomous trail <sid>")
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	trail := s.Trail()
	if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{
			"session": s.ID, "question": s.Question, "owner": s.Owner,
			"rounds": s.Rounds, "trail": trail, "complete": s.DeliberationComplete(),
		})
	}
	fmt.Fprintf(c.Stdout, "Trail — %s (owner %s)\n", s.Question, s.Owner)
	for _, r := range trail {
		fmt.Fprintf(c.Stdout, "\nRound %d\n", r.Round)
		for _, t := range r.Turns {
			fmt.Fprintf(c.Stdout, "  %-14s [%s] %s\n", t.Persona, t.Stance, strings.Join(t.Why, " → "))
		}
	}
	fmt.Fprintf(c.Stdout, "\nCOMPLETE: %t (%d/%d turns)\n", s.DeliberationComplete(), len(s.Turns), s.Rounds*len(s.Personas))
	return nil
}

func autoVote(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args,
		[]string{"persona", "position", "confidence", "reason"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" || a.values["persona"] == "" || a.values["position"] == "" {
		return Usagef(`huddle autonomous vote <sid> --persona <id> --position "<option>" --confidence 0.8`)
	}
	conf, err := strconv.ParseFloat(strings.TrimSpace(a.values["confidence"]), 64)
	if err != nil {
		return Usagef("--confidence must be a number 0..1, got %q", a.values["confidence"])
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	v, err := s.RecordVote(a.values["persona"], a.values["position"], conf, a.values["reason"])
	if err != nil {
		return err
	}
	if err := room.Save(root, s); err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"recorded": v, "votes": len(s.Votes), "didNotVote": s.DidNotVote()})
	}
	output.KV(c.Stdout, "VOTE", v.Persona+" → "+v.Position,
		"CONFIDENCE", strconv.FormatFloat(v.Confidence, 'f', -1, 64),
		"PENDING", strings.Join(s.DidNotVote(), ", "))
	return nil
}

func autoTally(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" {
		return Usagef("huddle autonomous tally <sid>")
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	res, err := room.Resolve(s.Votes, s.Owner)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"session": s.ID, "owner": s.Owner, "didNotVote": s.DidNotVote(), "tally": res})
	}
	output.KV(c.Stdout, "RESOLVED", res.Resolved, "OWNER", s.Owner+" voted "+res.OwnerPosition,
		"OWNER_OVERRIDDEN", strconv.FormatBool(res.OwnerOverridden))
	rows := make([][]string, 0, len(res.Weights))
	for _, w := range res.Weights {
		rows = append(rows, []string{w.Option, strconv.FormatFloat(w.Weight, 'f', -1, 64), strconv.Itoa(w.Voters)})
	}
	output.Table(c.Stdout, []string{"OPTION", "WEIGHT", "VOTERS"}, rows)
	for _, d := range res.Dissents {
		fmt.Fprintf(c.Stdout, "DISSENT: %s (%s) %s\n", d.Persona, d.Position, d.Reason)
	}
	return nil
}

func autoForkCheck(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args, []string{"decision", "flags"}, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	decision := strings.TrimSpace(a.values["decision"])
	if decision == "" && len(a.pos) > 0 {
		decision = strings.Join(a.pos, " ")
	}
	if decision == "" {
		return Usagef(`huddle autonomous fork-check --decision "<text>" [--flags launch,spend]`)
	}
	f := room.ForkCheck(decision, coreSplitCSV(a.values["flags"]))
	if a.bools["json"] {
		return output.JSON(c.Stdout, f)
	}
	output.KV(c.Stdout, "ESCALATE", strconv.FormatBool(f.Escalate), "FLAGS", strings.Join(f.Flags, ", "))
	for _, r := range f.Reasons {
		fmt.Fprintln(c.Stdout, "REASON: "+r)
	}
	if len(f.Unknown) > 0 {
		fmt.Fprintf(c.Stdout, "UNKNOWN FLAGS: %s (known: %s)\n", strings.Join(f.Unknown, ", "), strings.Join(room.ForkFlagNames(), ", "))
	}
	return nil
}

func autoVerdict(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args,
		[]string{"decision", "flags", "summary"}, []string{"json", "dry-run"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" {
		return Usagef(`huddle autonomous verdict <sid> --decision "<the actionable decision>"`)
	}
	info, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	v, err := room.BuildVerdict(s, a.values["decision"], a.values["summary"], coreSplitCSV(a.values["flags"]), time.Now())
	if err != nil {
		return err
	}
	out := map[string]any{"verdict": v, "dryRun": a.bools["dry-run"]}
	if !a.bools["dry-run"] {
		path, err := room.WriteVerdict(root, s, v)
		if err != nil {
			if errors.Is(err, room.ErrExists) {
				return fmt.Errorf("%w — a verdict is written once; open a new session to decide again", err)
			}
			return err
		}
		door, evType := "two-way", "autonomous.decided"
		if v.Escalation.Required {
			door, evType = "one-way", "autonomous.escalated"
		}
		ev, err := eventlog.Append(info.Name, eventlog.Event{
			Type: evType, By: v.Owner, Branch: info.Branch, Door: door, Text: v.Decision,
			Data: map[string]any{
				"session": v.Session, "resolved": v.Tally.Resolved, "ownerOverridden": v.Tally.OwnerOverridden,
				"dissents": len(v.Tally.Dissents), "flags": v.Escalation.Flags, "actionable": v.Actionable,
			},
		})
		if err != nil {
			return err
		}
		out["event"], out["path"] = ev.ID, path
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, out)
	}
	output.KV(c.Stdout, "STATUS", v.Status, "DECISION", v.Decision, "RESOLVED", v.Tally.Resolved,
		"ACTIONABLE", strconv.FormatBool(v.Actionable), "DISSENTS", strconv.Itoa(len(v.Tally.Dissents)))
	if v.Escalation.Required {
		fmt.Fprintf(c.Stdout, "ESCALATE: owner-level fork — stage this for the human owner, do not act\n")
		for _, r := range v.Escalation.Reasons {
			fmt.Fprintln(c.Stdout, "REASON: "+r)
		}
	}
	if a.bools["dry-run"] {
		fmt.Fprintf(c.Stdout, "DRY RUN: nothing written — rerun without --dry-run\n\n%s", room.Markdown(v))
	} else {
		output.KV(c.Stdout, "VERDICT", out["path"].(string), "EVENT", out["event"].(string))
	}
	return nil
}

func autoList(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	sessions, err := room.List(root)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		if sessions == nil {
			sessions = []room.Session{}
		}
		return output.JSON(c.Stdout, sessions)
	}
	if len(sessions) == 0 {
		fmt.Fprintf(c.Stdout, "No autonomous sessions — run: huddle autonomous new --question \"...\" --personas %s,shaama\n", DefaultOwner)
		return nil
	}
	rows := make([][]string, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, []string{s.ID, s.Status, s.Owner, strconv.Itoa(len(s.Personas)), coreTruncate(s.Question, 60)})
	}
	output.Table(c.Stdout, []string{"SESSION", "STATUS", "OWNER", "ROOM", "QUESTION"}, rows)
	return nil
}

func autoShow(c *Ctx, args []string) error {
	a, err := parseCoreArgs("autonomous", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, autonomousHelp)
	}
	id := autoSessionID(a)
	if id == "" {
		return Usagef("huddle autonomous show <sid>")
	}
	_, root, err := autoRoot(c)
	if err != nil {
		return err
	}
	s, err := room.Load(root, id)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, s)
	}
	output.KV(c.Stdout, "SESSION", s.ID, "STATUS", s.Status, "QUESTION", s.Question,
		"OWNER", s.Owner, "ROOM", strings.Join(s.SpeakingOrder, " → "),
		"ROUNDS", strconv.Itoa(s.Rounds),
		"TURNS", fmt.Sprintf("%d/%d", len(s.Turns), s.Rounds*len(s.Personas)),
		"VOTES", fmt.Sprintf("%d/%d", len(s.Votes), len(s.Personas)))
	return nil
}
