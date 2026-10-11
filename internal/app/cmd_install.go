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
  huddle install [--agents] [--force] [--dry-run] [--json]
  huddle uninstall [--agents] [--dry-run] [--json]

Installs the embedded skills (huddle, huddle-sdlc, huddle-personas) -- the
skill family is the product, so this is what a bare "huddle install" does;
--skills is still accepted and means the same thing -- into
Claude's skills root -- $CLAUDE_SKILLS_DIR, else $CLAUDE_CONFIG_DIR/skills,
else ~/.claude/skills -- and, when codex is on PATH or --agents is given,
~/.agents/skills ($AGENTS_SKILLS_DIR). Each dir is replaced atomically
and carries a .huddle-owned marker. Existing dirs without the marker are left
alone unless --force. An old install.sh symlink named "huddle" that points at a
repo checkout is replaced and reported.

uninstall removes only dirs with the marker (plus that legacy symlink).
Huddle state under ~/.config/.huddle is never touched.

--dry-run writes nothing and reports the action each destination would get:
installed, updated, pruned-legacy, not-installed or skipped.
`

func runInstall(c *Ctx, args []string) error {
	return installCmd(c, "install", args)
}

func runUninstall(c *Ctx, args []string) error {
	return installCmd(c, "uninstall", args)
}

func installCmd(c *Ctx, verb string, args []string) error {
	bools := []string{"skills", "agents", "json", "dry-run"}
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
	// The skill family IS the product, so installing huddle installs it: bare
	// `huddle install` does the same thing as `huddle install --skills`. The flag
	// stays accepted so every published postinstall hook and the brew cask keep
	// working unchanged, but it is no longer something a human has to know.
	if len(a.pos) > 0 {
		return Usagef("run: huddle %s [--agents]", verb)
	}
	targets, err := install.DefaultTargets(a.bools["agents"])
	if err != nil {
		return err
	}
	var res []install.Result
	dry := a.bools["dry-run"]
	switch {
	case verb == "install" && dry:
		res, err = install.Plan(install.Skills(), targets, install.Options{Force: a.bools["force"]})
	case verb == "install":
		res, err = install.Install(install.Skills(), targets, install.Options{Force: a.bools["force"]})
	case dry:
		res, err = install.PlanUninstall(install.Skills(), targets)
	default:
		res, err = install.Uninstall(install.Skills(), targets)
	}
	if a.bools["json"] {
		if jerr := output.JSON(c.Stdout, map[string]any{"dryRun": dry, "results": res}); jerr != nil {
			return jerr
		}
		return err
	}
	rows := make([][]string, 0, len(res))
	for _, r := range res {
		rows = append(rows, []string{r.Host, r.Skill, r.Action, r.Path, r.Note})
	}
	if dry {
		fmt.Fprintf(c.Stdout, "DRY RUN: nothing written — rerun without --dry-run to %s.\n", verb)
	}
	if len(rows) > 0 {
		output.Table(c.Stdout, []string{"HOST", "SKILL", "ACTION", "PATH", "NOTE"}, rows)
	}
	if err == nil && verb == "install" && !dry {
		fmt.Fprintln(c.Stdout, "Restart your agent session to load the skills. Check with: huddle doctor")
	}
	return err
}
