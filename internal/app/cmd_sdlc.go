package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/sdlc"
	"github.com/muthuishere/huddle-next/internal/specframework"
)

func init() {
	register(Command{Name: "sdlc", Summary: "Where each change is in the SDLC, and the single next action", Run: runSdlc})
}

const sdlcHelp = `Usage: huddle sdlc <verb> [--json]

  status   per intent: intent/spec/plan status, pending gates, verdict (+stale), open spikes
  next     the lowest missing stage and the exact command to run

status also reports an external spec framework (e.g. an openspec/ directory)
when one is present, and where its files are. Huddle has no dependency on it:
with none present, status reports huddle's native paths and nothing changes.
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
	// Reading the external framework's layout is the only place `huddle sdlc`
	// starts a process, and `sdlc status` is human-invoked. `huddle start`
	// never reaches here (ADR 23).
	ext, _ := specframework.New(env.Repo.Root, r.SpecFramework).Changes(context.Background())
	if a.Bools["json"] {
		return output.JSON(c.Stdout, sdlcJSON{Report: r, SpecFrameworkChanges: ext})
	}
	sdlcPrint(c, r, ext)
	return nil
}

// sdlcJSON is the report plus the external framework's changes, which are read
// by exec and so are not part of sdlc.Report itself.
type sdlcJSON struct {
	sdlc.Report
	SpecFrameworkChanges []specframework.Change `json:"specFrameworkChanges,omitempty"`
}

func sdlcPrint(c *Ctx, r sdlc.Report, ext []specframework.Change) {
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
	if r.SpecFramework != nil {
		fmt.Fprintf(w, "\nSPEC FRAMEWORK: %s\n", specframework.Line(r.SpecFramework))
		for _, ch := range ext {
			state := "in progress"
			if ch.Complete {
				state = "complete"
			}
			var have, want []string
			for _, art := range ch.Artifacts {
				label := art.Kind
				if art.Native != "" {
					label += "\u2192" + art.Native
				}
				if art.Exists {
					have = append(have, label)
				} else {
					want = append(want, label)
				}
			}
			fmt.Fprintf(w, "  %s (%s) %s — has: %s; missing: %s\n", ch.ID, state, ch.Dir,
				orNone(have), orNone(want))
		}
	}
	fmt.Fprintf(w, "\nNEXT: %s\n  %s\n", r.Next.Reason, r.Next.Command)
}

func orNone(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ", ")
}
