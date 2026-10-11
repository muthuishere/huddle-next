package app

import (
	"fmt"

	"github.com/muthuishere/huddle-next/internal/install"
	"github.com/muthuishere/huddle-next/internal/output"
)

func init() {
	register(
		Command{Name: "install", Summary: "Install the huddle skill family into agent skill dirs", Run: runInstall},
		Command{Name: "uninstall", Summary: "Remove huddle-owned skill dirs (never touches state)", Run: runUninstall},
	)
}

const installHelp = `
Usage:
  huddle install --skills [--agents] [--force] [--json]
  huddle uninstall --skills [--agents] [--json]

Installs the embedded skills (huddle, huddle-sdlc, huddle-personas) into
Claude's skills root -- $CLAUDE_SKILLS_DIR, else $CLAUDE_CONFIG_DIR/skills,
else ~/.claude/skills -- and, when codex is on PATH or --agents is given,
~/.agents/skills ($AGENTS_SKILLS_DIR). Each dir is replaced atomically
and carries a .huddle-owned marker. Existing dirs without the marker are left
alone unless --force. An old install.sh symlink named "huddle" that points at a
repo checkout is replaced and reported.

uninstall removes only dirs with the marker (plus that legacy symlink).
Huddle state under ~/.config/.huddle is never touched.
`

func runInstall(c *Ctx, args []string) error {
	return installCmd(c, "install", args)
}

func runUninstall(c *Ctx, args []string) error {
	return installCmd(c, "uninstall", args)
}

func installCmd(c *Ctx, verb string, args []string) error {
	bools := []string{"skills", "agents", "json"}
	if verb == "install" {
		bools = append(bools, "force")
	}
	a, err := parseCoreArgs(verb, args, nil, bools)
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, installHelp)
	}
	if !a.bools["skills"] || len(a.pos) > 0 {
		return Usagef("run: huddle %s --skills [--agents]", verb)
	}
	targets, err := install.DefaultTargets(a.bools["agents"])
	if err != nil {
		return err
	}
	var res []install.Result
	if verb == "install" {
		res, err = install.Install(install.Skills(), targets, install.Options{Force: a.bools["force"]})
	} else {
		res, err = install.Uninstall(install.Skills(), targets)
	}
	if a.bools["json"] {
		if jerr := output.JSON(c.Stdout, map[string]any{"results": res}); jerr != nil {
			return jerr
		}
		return err
	}
	rows := make([][]string, 0, len(res))
	for _, r := range res {
		rows = append(rows, []string{r.Host, r.Skill, r.Action, r.Path, r.Note})
	}
	if len(rows) > 0 {
		output.Table(c.Stdout, []string{"HOST", "SKILL", "ACTION", "PATH", "NOTE"}, rows)
	}
	if err == nil && verb == "install" {
		fmt.Fprintln(c.Stdout, "Restart your agent session to load the skills. Check with: huddle doctor")
	}
	return err
}
