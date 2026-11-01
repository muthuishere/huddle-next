package app

import (
	"fmt"

	"github.com/muthuishere/huddle-next/artifacts"
)

func init() {
	register(Command{Name: "ci", Summary: "Draft CI workflows: claude -p failure triage, intent-merge spec draft", Run: runCI})
}

const ciHelp = `Usage: huddle ci draft <triage|spec> [--out path] [--branch main] [--workflow CI] [--write]

  triage   read-only claude -p job that diagnoses a failed workflow run and comments on the PR
  spec     on merge of intent/**/intent.md, claude -p drafts spec.md into a pull request

Templates only: prints unless --write, never overwrites. huddle never pushes or changes CI.
For the eval workflow run: huddle eval ci-draft
`

func runCI(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, ciHelp)
		return nil
	}
	if args[0] != "draft" {
		return Usagef("unknown verb %q — run: huddle ci --help", args[0])
	}
	a, err := artifacts.ParseArgs(args[1:], []string{"write"}, []string{"out", "branch", "workflow"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 || (a.Pos[0] != "triage" && a.Pos[0] != "spec") {
		return Usagef("huddle ci draft <triage|spec> [--write]")
	}
	name := a.Pos[0]
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	out := a.Flags["out"]
	if out == "" {
		out = artifacts.CIDrafts[name].Path
	}
	workflow := a.Flags["workflow"]
	if workflow == "" {
		workflow = "CI"
	}
	return ciEmit(c, env, name, out, map[string]string{
		"branch":   ciBranch(a.Flags["branch"]),
		"workflow": workflow,
	}, a.Bools["write"], a.Bools["json"], "")
}
