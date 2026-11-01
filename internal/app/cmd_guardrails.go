package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/record"
)

func init() {
	k, _ := artifacts.Get(config.Config{}, "guardrails")
	register(Command{Name: "guardrails", Summary: k.Summary, Run: runGuardrails})
}

const guardrailsAdoptHelp = `  adopt [--dry-run] [--from <draft>] [--author <name>]
                                                      put huddle's block into a CLAUDE.md
                                                      huddle did not write; everything
                                                      outside the markers is left alone

` + "`new`" + ` writes the whole file and refuses to touch an existing one. ` + "`adopt`" + ` is for the
repo that already has a CLAUDE.md: it inserts (or, with --from, refreshes) only the
` + artifacts.GuardrailsBegin + ` … ` + artifacts.GuardrailsEnd + ` block.
`

func runGuardrails(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		artNounHelp(c.Stdout, "guardrails")
		fmt.Fprint(c.Stdout, "\n"+guardrailsAdoptHelp)
		return nil
	}
	if args[0] == "adopt" {
		for _, a := range args[1:] {
			if a == "--help" || a == "-h" || a == "help" {
				fmt.Fprint(c.Stdout, guardrailsAdoptHelp)
				return nil
			}
		}
		return guardrailsAdopt(c, args[1:])
	}
	err := artRunNoun(c, "guardrails", args)
	if args[0] == "new" && errors.Is(err, artifacts.ErrExists) {
		return guardrailsExistsHint(err, guardrailsPath(c))
	}
	return err
}

// guardrailsPath is the configured guardrails file, for messages only.
func guardrailsPath(c *Ctx) string {
	cfg := config.Config{}
	if env, err := artEnv(c); err == nil {
		cfg = env.Cfg
	}
	k, _ := artifacts.Get(cfg, "guardrails")
	return k.Pattern
}

// guardrailsAdopt edits an existing CLAUDE.md in the one way ADR 12 §7 allows:
// huddle's own block is inserted or refreshed and every other byte is kept.
func guardrailsAdopt(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, []string{"dry-run"}, []string{"author", "from"})
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	k, _ := artifacts.Get(env.Cfg, "guardrails")
	rel := filepath.ToSlash(k.Pattern)
	abs := filepath.Join(env.Repo.Root, filepath.FromSlash(rel))
	raw, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no %s yet — adopt edits a file that already exists; run: huddle guardrails new", rel)
		}
		return err
	}
	author := a.Flags["author"]
	if author == "" {
		author = record.CurrentUser(env.Cfg)
	}
	tpl, err := artifacts.Render(env.Repo.Root, env.Cfg, "guardrails", artifacts.NewOptions{Author: author})
	if err != nil {
		return err
	}
	tplFM, tplBody := artifacts.SplitFrontMatter([]byte(tpl.Content))

	// The block huddle will own: the template's, or a reviewed draft's.
	block, err := artifacts.WrapManagedBlock(string(tplBody))
	if err != nil {
		return err
	}
	if from := a.Flags["from"]; from != "" {
		draft, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		_, body := artifacts.SplitFrontMatter(draft)
		if block, err = artifacts.WrapManagedBlock(string(body)); err != nil {
			return fmt.Errorf("%s: %w", from, err)
		}
	}

	fm, body := artifacts.SplitFrontMatter(raw)
	had, hasBlock, err := artifacts.ManagedBlock(string(body))
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	if hasBlock && a.Flags["from"] == "" {
		// Already managed and no new content given: never overwrite what a
		// human wrote inside the block.
		if a.Bools["json"] {
			return output.JSON(c.Stdout, map[string]any{"path": rel, "managed": true, "changed": false})
		}
		output.KV(c.Stdout, "MANAGED", rel, "CHANGED", "no")
		fmt.Fprintf(c.Stdout, "NOTE: %s already has huddle's block (%d bytes) — edit it in place, or replace it with: huddle guardrails adopt --from <draft>\n", rel, len(had))
		return nil
	}
	newBody, err := artifacts.SetManagedBlock(string(body), block)
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}

	// Front matter is huddle's own metadata (lint requires id, kind, status,
	// author); fill only what the file is missing, keep the rest verbatim.
	prefix := string(raw[:len(raw)-len(body)])
	changedFM := false
	if !fm.Present {
		fm, changedFM = tplFM, true
	} else {
		for _, kv := range [][2]string{{"id", tpl.ID}, {"kind", "guardrails"}, {"status", "draft"}, {"author", author}} {
			if fm.Get(kv[0]) == "" {
				fm.Set(kv[0], kv[1])
				changedFM = true
			}
		}
	}
	if changedFM {
		prefix = string(fm.Render())
	}
	out := prefix + newBody

	verb := "insert"
	if hasBlock {
		verb = "refresh"
	}
	if a.Bools["dry-run"] {
		if a.Bools["json"] {
			return output.JSON(c.Stdout, map[string]any{"dryRun": true, "path": rel, "action": verb,
				"frontMatter": changedFM, "sha256": artifacts.SHA256([]byte(out)), "content": out})
		}
		fmt.Fprintf(c.Stdout, "DRY RUN: would %s huddle's block in %s (%d bytes kept outside the markers%s)\n\n%s\n",
			verb, rel, len(body)-len(had), map[bool]string{true: ", front matter added"}[changedFM], block)
		return nil
	}
	if err := artifacts.WriteAtomic(abs, []byte(out)); err != nil {
		return err
	}
	x, err := artifacts.Load(env.Repo.Root, k, rel)
	if err != nil {
		return err
	}
	ev, res, err := record.RecordWrite(env, x.ID, x.Kind, rel, x.SHA256, author, "adopt")
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"path": rel, "id": x.ID, "action": verb,
			"sha256": x.SHA256, "event": ev.ID, "hooks": res})
	}
	output.KV(c.Stdout, "ADOPTED", rel, "ID", x.ID, "STATUS", artDash(x.Status))
	fmt.Fprintln(c.Stdout, "NOTE: only the "+artifacts.GuardrailsBegin+" block is huddle's; mirror shared content into AGENTS.md for non-Claude harnesses (Huddle convention).")
	fmt.Fprintf(c.Stdout, "NEXT: fill the block in %s, then: huddle guardrails lint && huddle gate request %s --role %s\n", rel, x.ID, k.GateRole)
	return nil
}

// guardrailsExistsHint rewrites the ErrExists failure of `new` so it names the
// verb that can do the job instead of asking for the markers by hand.
func guardrailsExistsHint(err error, rel string) error {
	if !errors.Is(err, artifacts.ErrExists) {
		return err
	}
	return fmt.Errorf("%w: %s — huddle owns only its %s block there; insert it with: huddle guardrails adopt [--dry-run]",
		artifacts.ErrExists, strings.TrimSpace(rel), artifacts.GuardrailsBegin)
}
