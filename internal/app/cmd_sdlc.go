package app

import (
	"fmt"
	"strings"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/sdlc"
)

func init() {
	register(Command{Name: "sdlc", Summary: "Where each change is in the SDLC, and the single next action", Run: runSdlc})
}

const sdlcHelp = `Usage: huddle sdlc <verb> [--json]

  status   per intent: intent/spec/plan status, pending gates, verdict (+stale), open spikes
  next     the lowest missing stage and the exact command to run
`

func runSdlc(c *Ctx, args []string) error {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(c.Stdout, sdlcHelp)
		return nil
	}
	verb := "status"
	if len(args) > 0 && !strings.HasPrefix(args[0], "--") {
		verb, args = args[0], args[1:]
	}
	a, err := artifacts.ParseArgs(args, nil, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	if verb != "status" && verb != "next" {
		return Usagef("unknown verb %q — run: huddle sdlc --help", verb)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	r, err := sdlc.Status(env.Repo, env.Cfg)
	if err != nil {
		return err
	}
	if verb == "next" {
		if a.Bools["json"] {
			return output.JSON(c.Stdout, r.Next)
		}
		output.KV(c.Stdout, "STAGE", r.Next.Stage, "WHY", r.Next.Reason, "RUN", r.Next.Command)
		return nil
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, r)
	}
	sdlcPrint(c, r)
	return nil
}

func sdlcPrint(c *Ctx, r sdlc.Report) {
	w := c.Stdout
	fmt.Fprintf(w, "%s @ %s\n\n", r.Repo, r.Branch)
	yes := func(p sdlc.Presence) string {
		if !p.Exists {
			return "missing"
		}
		if p.Status != "" && p.Status != "-" {
			return p.Status
		}
		return "present"
	}
	guard := yes(r.Guardrails)
	if r.Guardrails.Exists && !r.Guardrails.Block {
		guard += " (no huddle block)"
	}
	output.KV(w, "CLAUDE.md", guard, "AGENTS.md", yes(r.AgentsMD), "REVIEW.md", yes(r.ReviewPolicy),
		"bands.yaml", yes(r.Bands), "evals", fmt.Sprint(r.Evals))
	fmt.Fprintln(w)
	if len(r.Changes) == 0 {
		fmt.Fprintln(w, "No intents yet.")
	} else {
		rows := make([][]string, len(r.Changes))
		for i, ch := range r.Changes {
			st := func(s sdlc.Stage) string {
				if !s.Exists {
					return "·"
				}
				return s.Status
			}
			verdict := "-"
			if ch.Verdict != nil {
				verdict = ch.Verdict.Verdict
				if ch.Verdict.Stale {
					verdict += " (stale)"
				}
			}
			rows[i] = []string{ch.Slug, st(ch.Intent), st(ch.Spec), st(ch.Plan), fmt.Sprint(len(ch.PendingGates)), verdict}
		}
		output.Table(w, []string{"CHANGE", "INTENT", "SPEC", "PLAN", "GATES", "VERDICT"}, rows)
	}
	if len(r.PendingGates) > 0 {
		ids := make([]string, len(r.PendingGates))
		for i, g := range r.PendingGates {
			ids[i] = g.ID + " " + g.ArtifactID + " (" + g.Role + ")"
		}
		fmt.Fprintf(w, "\nPENDING GATES: %s\n", strings.Join(ids, "; "))
	}
	if len(r.OpenSpikes) > 0 {
		fmt.Fprintf(w, "OPEN SPIKES: %s\n", strings.Join(r.OpenSpikes, ", "))
	}
	fmt.Fprintf(w, "\nNEXT: %s\n  %s\n", r.Next.Reason, r.Next.Command)
}
