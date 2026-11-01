package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/record"
)

// gateInteractive reports whether a human is at the terminal; tests swap it.
var gateInteractive = func(c *Ctx) bool {
	return output.IsTTY(os.Stdin) && output.IsTTY(c.Stdout)
}

func init() {
	register(Command{Name: "gate", Summary: "Human approval gates with separation of duties", Run: runGate})
}

const gateHelp = `Usage: huddle gate <verb> [options]

  request <artifact-id> [--role <owner-role>]           open a gate (default role per kind)
          [--name deploy|production|release|<free>]     gate name (default: the artifact kind)
  approve <gate-id> [--reason r] [--as <owner>]         approve (human owner, not the author)
          [--override --reason r]                       override a hook deny/ask (logged)
  reject  <gate-id> --reason r [--as <owner>]           reject
  status  [--all] [--json]                              requested gates; STALE when the file changed
                                                        --json lists every gate: [{id, name, artifact,
                                                        role, state, stale, approvedBy, ts}]

Agent sessions (HUDDLE_AGENT, CLAUDECODE, CODEX_THREAD_ID, … set) cannot approve or reject, even with --as. Without a terminal, --as is required.
`

func runGate(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, gateHelp)
		return nil
	}
	switch args[0] {
	case "request":
		return gateRequest(c, args[1:])
	case "approve":
		return gateDecide(c, args[1:], true)
	case "reject":
		return gateDecide(c, args[1:], false)
	case "status", "list":
		return gateStatus(c, args[1:])
	}
	return Usagef("unknown verb %q — run: huddle gate --help", args[0])
}

func gateRequest(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"role", "name"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 {
		return Usagef("huddle gate request <artifact-id> [--role <owner-role>] [--name <gate-name>]")
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	g, err := gates.Request(gates.Deps{Root: env.Repo.Root, Env: env}, a.Pos[0], a.Flags["role"], a.Flags["name"], record.CurrentUser(env.Cfg))
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, g)
	}
	output.KV(c.Stdout, "GATE", g.ID, "NAME", g.Name, "ARTIFACT", g.ArtifactID, "ROLE", g.Role, "OWNERS", strings.Join(env.Cfg.Owners[g.Role], ", "))
	fmt.Fprintf(c.Stdout, "NEXT: a %s owner who did not write %s runs: huddle gate approve %s\n", g.Role, g.ArtifactID, g.ID)
	return nil
}

func gateDecide(c *Ctx, args []string, approve bool) error {
	a, err := artifacts.ParseArgs(args, []string{"override"}, []string{"reason", "as"})
	if err != nil {
		return Usagef("%v", err)
	}
	verb := map[bool]string{true: "approve", false: "reject"}[approve]
	if len(a.Pos) != 1 {
		return Usagef("huddle gate %s <gate-id> [--reason r] [--as <owner>]", verb)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	g, err := gates.Decide(gates.Deps{Root: env.Repo.Root, Env: env}, gates.DecideOptions{
		GateID: a.Pos[0], Approve: approve, Reason: a.Flags["reason"], Override: a.Bools["override"],
		Identity: gates.Identity{
			Agent:       gates.AgentSession(os.Getenv),
			Interactive: gateInteractive(c),
			As:          a.Flags["as"],
			User:        record.CurrentUser(env.Cfg),
		},
	})
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, g)
	}
	output.KV(c.Stdout, "GATE", g.ID, "STATE", g.State, "BY", g.DecidedBy, "ARTIFACT", g.ArtifactID)
	if g.Override {
		fmt.Fprintln(c.Stdout, "OVERRIDE: hook result overridden — logged as gate.override")
	}
	return nil
}

func gateStatus(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, []string{"all"}, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	all, err := gates.Load(env.Repo.Name)
	if err != nil {
		return err
	}
	byPath := map[string]artifacts.Artifact{}
	for _, g := range all {
		if _, seen := byPath[g.Path]; seen {
			continue
		}
		if k, ok := artifacts.Get(env.Cfg, g.Kind); ok {
			if x, err := artifacts.Load(env.Repo.Root, k, g.Path); err == nil {
				byPath[g.Path] = x
			}
		}
	}
	gates.Annotate(all, byPath)
	list := make([]gates.Gate, 0, len(all))
	for _, g := range gates.Sorted(all) {
		// --json always lists every gate: hooks need approved ones to open a deploy.
		if a.Bools["all"] || a.Bools["json"] || g.State == gates.StateRequested || g.Stale {
			list = append(list, g)
		}
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, list)
	}
	if len(list) == 0 {
		fmt.Fprintln(c.Stdout, "no requested gates")
		return nil
	}
	rows := make([][]string, len(list))
	for i, g := range list {
		state := strings.ToUpper(g.State)
		if g.Stale {
			state += " STALE"
		}
		if g.Missing {
			state += " MISSING"
		}
		rows[i] = []string{g.ID, g.Name, state, g.ArtifactID, g.Role, artDash(g.FileStatus), artDash(g.DecidedBy)}
	}
	output.Table(c.Stdout, []string{"GATE", "NAME", "STATE", "ARTIFACT", "ROLE", "FILE STATUS", "DECIDED BY"}, rows)
	return nil
}
