package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/output"
)

func init() {
	k, _ := artifacts.Get(config.Config{}, "eval")
	register(Command{Name: "eval", Summary: k.Summary + "; draft the eval CI workflow", Run: runEval})
}

const evalHelp = `Usage: huddle eval <verb> [options]

  ci-draft [--out .github/workflows/agent-evals.yml] [--threshold 0.9] [--branch main] [--write]
          workflow that runs config eval.command when CLAUDE.md, AGENTS.md, .claude/**, or
          evals/** change; fails below the pass-rate threshold; no authenticated runner ⇒
          result "unverified", never PASS. Prints unless --write; never overwrites.
  new | list | show | status | lint | link    eval cases in evals/<slug>.json
          new "<name>" [--dry-run] [--from <draft>]
`

func runEval(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, evalHelp)
		return nil
	}
	if args[0] == "ci-draft" {
		return evalCIDraft(c, args[1:])
	}
	return artRunNoun(c, "eval", args)
}

// evalCommand reads eval.command (and eval.threshold) from merged config.
func evalCommand(cfg config.Config) (string, float64) {
	raw, ok := cfg.Extra["eval"]
	if !ok {
		return "", 0
	}
	var e struct {
		Command   string  `json:"command"`
		Threshold float64 `json:"threshold"`
	}
	_ = json.Unmarshal(raw, &e)
	return strings.TrimSpace(e.Command), e.Threshold
}

func evalCIDraft(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, []string{"write"}, []string{"out", "threshold", "branch"})
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	command, threshold := evalCommand(env.Cfg)
	if v := a.Flags["threshold"]; v != "" {
		if threshold, err = strconv.ParseFloat(v, 64); err != nil {
			return Usagef("--threshold must be a number between 0 and 1")
		}
	}
	if threshold == 0 {
		threshold = 0.9
	}
	if threshold <= 0 || threshold > 1 {
		return Usagef("--threshold must be in (0, 1]")
	}
	warn := ""
	if command == "" {
		warn = "eval.command is not set — run: huddle config set eval.command '<cmd that writes eval-results.json>'"
		command = `echo "::error::set eval.command: huddle config set eval.command '<cmd>'" && exit 1`
	}
	out := a.Flags["out"]
	if out == "" {
		out = artifacts.CIDrafts["eval"].Path
	}
	return ciEmit(c, env, "eval", out, map[string]string{
		"command":   command,
		"threshold": strconv.FormatFloat(threshold, 'f', -1, 64),
		"branch":    ciBranch(a.Flags["branch"]),
	}, a.Bools["write"], a.Bools["json"], warn)
}

func ciBranch(b string) string {
	if strings.TrimSpace(b) == "" {
		return "main"
	}
	return b
}

// ciEmit prints a workflow draft, or writes it with --write (never overwrites).
func ciEmit(c *Ctx, env artifacts.Env, name, out string, vars map[string]string, write, asJSON bool, warn string) error {
	content, err := artifacts.RenderCI(name, vars)
	if err != nil {
		return err
	}
	rel := filepath.ToSlash(filepath.Clean(out))
	if filepath.IsAbs(out) {
		if r, err := filepath.Rel(env.Repo.Root, out); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		} else {
			return fmt.Errorf("--out must be inside the repo (%s)", env.Repo.Root)
		}
	}
	d := artifacts.Draft{Kind: "ci", ID: "ci-" + name, Path: rel, Content: content, SHA256: artifacts.SHA256([]byte(content))}
	written := false
	if write {
		if err := artifacts.Create(env.Repo.Root, d); err != nil {
			if errors.Is(err, artifacts.ErrExists) {
				return fmt.Errorf("%s already exists — huddle never overwrites a workflow; diff it against: huddle %s", rel, ciCommandFor(name))
			}
			return err
		}
		if _, _, err := artifacts.RecordWrite(env, d.ID, "ci", d.Path, d.SHA256, artifacts.CurrentUser(env.Cfg), "draft"); err != nil {
			return err
		}
		written = true
	}
	if asJSON {
		res := map[string]any{"path": rel, "written": written, "sha256": d.SHA256}
		if !written {
			res["content"] = content
		}
		if warn != "" {
			res["warning"] = warn
		}
		return output.JSON(c.Stdout, res)
	}
	if warn != "" {
		fmt.Fprintln(c.Stderr, "WARNING:", warn)
	}
	if written {
		output.KV(c.Stdout, "WROTE", rel)
		fmt.Fprintln(c.Stdout, "NEXT: review the workflow, then commit it yourself — huddle never pushes or changes CI.")
		return nil
	}
	fmt.Fprintf(c.Stdout, "# DRAFT for %s (not written; add --write)\n%s", rel, content)
	return nil
}

func ciCommandFor(name string) string {
	if name == "eval" {
		return "eval ci-draft"
	}
	return "ci draft " + name
}
