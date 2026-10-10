package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/hooks"
	"github.com/muthuishere/huddle-next/internal/output"
)

// artFire runs hooks for artifact writes and gates; tests swap it.
var artFire artifacts.FireFunc = hooks.Fire

func init() {
	for _, name := range []string{"intent", "spec", "plan", "adr", "spike", "guardrails", "review-policy", "skill", "lessons"} {
		name := name
		k, _ := artifacts.Get(config.Config{}, name)
		register(Command{Name: name, Summary: k.Summary, Run: func(c *Ctx, args []string) error {
			return artRunNoun(c, name, args)
		}})
	}
}

// artEnv loads repo + merged config once for an artifact command.
func artEnv(c *Ctx) (artifacts.Env, error) {
	info, err := c.Repo()
	if err != nil {
		return artifacts.Env{}, err
	}
	cfg, err := config.Load(info.Root)
	if err != nil {
		return artifacts.Env{}, err
	}
	return artifacts.Env{Repo: info, Cfg: cfg, Fire: artFire}, nil
}

func artNounHelp(w io.Writer, noun string) {
	k, _ := artifacts.Get(config.Config{}, noun)
	extra := ""
	switch noun {
	case "spike":
		extra = "      [--timebox <2d>]                               fill the Timebox section\n"
	case "lessons":
		extra = "      [--eval <id>] [--intent <id>]                  link the eval and intent\n"
	}
	fmt.Fprintf(w, `Usage: huddle %[1]s <verb> [options]

%[2]s

Verbs:
  new <slug|"title"> [--dry-run] [--author <name>]   write the template (never overwrites)
      [--from <draft>]                               write a reviewed draft instead (status: draft)
%[3]s  list [--status <s>]                                list files and their status
  show <id|slug>                                     print the file
  status [<id>]                                      status + lint summary
  lint [<id>]                                        check required headings
  supersede <id> --by <id>                           mark superseded
  link <id> --to <id|url>                            add a front matter link

Status is read from the file's front matter; hand edits win. Add --json for machine output.
`, noun, k.Summary, extra)
}

func artRunNoun(c *Ctx, noun string, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		artNounHelp(c.Stdout, noun)
		return nil
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "new":
		return artNew(c, noun, rest)
	case "list":
		return artList(c, noun, rest)
	case "show":
		return artShow(c, noun, rest)
	case "status":
		return artStatus(c, noun, rest)
	case "lint":
		return artLint(c, noun, rest)
	case "supersede":
		return artSupersede(c, noun, rest)
	case "link":
		return artLink(c, noun, rest)
	}
	return Usagef("unknown verb %q — run: huddle %s --help", verb, noun)
}

func artNew(c *Ctx, noun string, args []string) error {
	values := []string{"author", "from"}
	switch noun {
	case "lessons":
		values = append(values, "eval", "intent")
	case "spike":
		values = append(values, "timebox")
	}
	a, err := artifacts.ParseArgs(args, []string{"dry-run"}, values)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	author := a.Flags["author"]
	if author == "" {
		author = artifacts.CurrentUser(env.Cfg)
	}
	title := strings.Join(a.Pos, " ")
	d, err := artifacts.Render(env.Repo.Root, env.Cfg, noun, artifacts.NewOptions{Author: author, Title: title})
	if err != nil {
		return err
	}
	if from := a.Flags["from"]; from != "" {
		// A reviewed draft replaces the template; id, kind and author stay
		// huddle's and status restarts at draft so a gate still decides it.
		raw, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		tpl, _ := artifacts.SplitFrontMatter([]byte(d.Content))
		d.Content = string(raw)
		if kind, _ := artifacts.Get(env.Cfg, noun); kind.Format == artifacts.FormatMD && tpl.Present {
			fm, body := artifacts.SplitFrontMatter(raw)
			if !fm.Present {
				fm = tpl
			}
			for _, key := range []string{"id", "kind", "author"} {
				fm.Set(key, tpl.Get(key))
			}
			fm.Set("status", "draft")
			d.Content = string(append(fm.Render(), body...))
		}
	}
	if tb := strings.TrimSpace(a.Flags["timebox"]); tb != "" {
		d.Content = strings.Replace(d.Content, "_e.g. 2d — started ", "_"+tb+" — started ", 1)
	}
	d.SHA256 = artifacts.SHA256([]byte(d.Content))
	// Chain links: spec/plan point back at their intent; lessons at eval + intent.
	links := map[string]string{}
	slug := artifacts.Slugify(title)
	switch noun {
	case "spec":
		links["intent"] = "intent-" + slug
	case "plan":
		links["intent"], links["spec"] = "intent-"+slug, "spec-"+slug
	case "lessons":
		if v := a.Flags["eval"]; v != "" {
			links["eval"] = v
		}
		if v := a.Flags["intent"]; v != "" {
			links["intent"] = v
		}
	}
	if len(links) > 0 {
		fm, body := artifacts.SplitFrontMatter([]byte(d.Content))
		for _, key := range []string{"intent", "spec", "eval"} {
			if v, ok := links[key]; ok {
				fm.AddLink(key, v)
			}
		}
		d.Content = string(append(fm.Render(), body...))
		d.SHA256 = artifacts.SHA256([]byte(d.Content))
	}

	if a.Bools["dry-run"] {
		if a.Bools["json"] {
			return output.JSON(c.Stdout, map[string]any{"dryRun": true, "kind": d.Kind, "id": d.ID, "path": d.Path, "sha256": d.SHA256, "content": d.Content})
		}
		fmt.Fprintf(c.Stdout, "DRY RUN: would write %s (%s)\n\n%s", d.Path, d.ID, d.Content)
		return nil
	}
	if err := artifacts.Create(env.Repo.Root, d); err != nil {
		if errors.Is(err, artifacts.ErrExists) && noun == "guardrails" {
			return fmt.Errorf("%w\nhuddle only edits its own block: add %s … %s to CLAUDE.md by hand, then run: huddle guardrails lint", err, artifacts.GuardrailsBegin, artifacts.GuardrailsEnd)
		}
		return err
	}
	ev, res, err := artifacts.RecordWrite(env, d.ID, d.Kind, d.Path, d.SHA256, author, "new")
	if err != nil {
		return err
	}
	k, _ := artifacts.Get(env.Cfg, noun)
	if a.Bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"kind": d.Kind, "id": d.ID, "path": d.Path, "sha256": d.SHA256, "event": ev.ID, "hooks": res})
	}
	output.KV(c.Stdout, "CREATED", d.Path, "ID", d.ID, "STATUS", artStatusOf(env, noun, d.Path))
	if noun == "guardrails" {
		fmt.Fprintln(c.Stdout, "NOTE: mirror shared content into AGENTS.md for non-Claude harnesses (Huddle convention).")
	}
	if k.Format == artifacts.FormatMD {
		fmt.Fprintf(c.Stdout, "NEXT: edit %s, then: huddle %s lint %s && huddle gate request %s --role %s\n", d.Path, noun, d.ID, d.ID, k.GateRole)
	}
	return nil
}

func artStatusOf(env artifacts.Env, noun, path string) string {
	k, _ := artifacts.Get(env.Cfg, noun)
	if a, err := artifacts.Load(env.Repo.Root, k, path); err == nil && a.Status != "" {
		return a.Status
	}
	return "-"
}

func artList(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"status"})
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	all, err := artifacts.Discover(env.Repo.Root, env.Cfg, noun)
	if err != nil {
		return err
	}
	want := strings.ToLower(a.Flags["status"])
	list := make([]artifacts.Artifact, 0, len(all))
	for _, x := range all {
		if want == "" || strings.EqualFold(x.Status, want) {
			list = append(list, x)
		}
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, list)
	}
	if len(list) == 0 {
		fmt.Fprintf(c.Stdout, "no %s artifacts — run: huddle %s new <slug>\n", noun, noun)
		return nil
	}
	rows := make([][]string, len(list))
	for i, x := range list {
		rows[i] = []string{x.ID, artDash(x.Status), artDash(x.Author), x.Path}
	}
	output.Table(c.Stdout, []string{"ID", "STATUS", "AUTHOR", "PATH"}, rows)
	return nil
}

func artDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func artShow(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	x, err := artifacts.Resolve(env.Repo.Root, env.Cfg, noun, strings.Join(a.Pos, " "))
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		k, _ := artifacts.Get(env.Cfg, noun)
		return output.JSON(c.Stdout, map[string]any{"artifact": x, "lint": artFindings(artifacts.Lint(x, k)), "content": string(x.Raw)})
	}
	_, err = c.Stdout.Write(x.Raw)
	return err
}

func artFindings(f []artifacts.Finding) []artifacts.Finding {
	if f == nil {
		return []artifacts.Finding{}
	}
	return f
}

func artStatus(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	k, _ := artifacts.Get(env.Cfg, noun)
	var list []artifacts.Artifact
	if len(a.Pos) > 0 {
		x, err := artifacts.Resolve(env.Repo.Root, env.Cfg, noun, strings.Join(a.Pos, " "))
		if err != nil {
			return err
		}
		list = []artifacts.Artifact{x}
	} else if list, err = artifacts.Discover(env.Repo.Root, env.Cfg, noun); err != nil {
		return err
	}
	type row struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Lint   int    `json:"lintFindings"`
	}
	rows := make([]row, 0, len(list))
	for _, x := range list {
		rows = append(rows, row{x.ID, artDash(x.Status), x.Path, x.SHA256, len(artifacts.Lint(x, k))})
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, rows)
	}
	if len(rows) == 0 {
		fmt.Fprintf(c.Stdout, "%s: missing (%s) — run: huddle %s new%s\n", noun, k.Pattern, noun, map[bool]string{true: "", false: " <slug>"}[k.Singleton()])
		return nil
	}
	table := make([][]string, len(rows))
	for i, r := range rows {
		lint := "ok"
		if r.Lint > 0 {
			lint = fmt.Sprintf("%d issue(s)", r.Lint)
		}
		table[i] = []string{r.ID, r.Status, lint, r.Path}
	}
	output.Table(c.Stdout, []string{"ID", "STATUS", "LINT", "PATH"}, table)
	return nil
}

func artLint(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	k, _ := artifacts.Get(env.Cfg, noun)
	var list []artifacts.Artifact
	if len(a.Pos) > 0 {
		x, err := artifacts.Resolve(env.Repo.Root, env.Cfg, noun, strings.Join(a.Pos, " "))
		if err != nil {
			return err
		}
		list = []artifacts.Artifact{x}
	} else if list, err = artifacts.Discover(env.Repo.Root, env.Cfg, noun); err != nil {
		return err
	}
	var all []artifacts.Finding
	for _, x := range list {
		all = append(all, artifacts.Lint(x, k)...)
	}
	if a.Bools["json"] {
		if err := output.JSON(c.Stdout, map[string]any{"checked": len(list), "findings": artFindings(all)}); err != nil {
			return err
		}
	} else {
		for _, f := range all {
			fmt.Fprintf(c.Stdout, "%s: %s [%s]\n", f.Path, f.Message, f.Rule)
		}
		if len(all) == 0 {
			fmt.Fprintf(c.Stdout, "LINT: ok (%d %s checked)\n", len(list), noun)
		}
	}
	if len(all) > 0 {
		return fmt.Errorf("%d lint finding(s) in %s", len(all), noun)
	}
	return nil
}

func artSupersede(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"by"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 || a.Flags["by"] == "" {
		return Usagef("huddle %s supersede <id> --by <id>", noun)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	old, err := artifacts.Resolve(env.Repo.Root, env.Cfg, noun, a.Pos[0])
	if err != nil {
		return err
	}
	repl, err := artifacts.Resolve(env.Repo.Root, env.Cfg, "", a.Flags["by"])
	if err != nil {
		return err
	}
	if repl.Path == old.Path {
		return errors.New("an artifact cannot supersede itself")
	}
	by := artifacts.CurrentUser(env.Cfg)
	old, err = artifacts.UpdateFrontMatter(env.Repo.Root, env.Cfg, old, func(fm *artifacts.FrontMatter) {
		fm.Set("status", "superseded")
		fm.Set("supersededBy", repl.ID)
	})
	if err != nil {
		return err
	}
	if _, _, err := artifacts.RecordWrite(env, old.ID, old.Kind, old.Path, old.SHA256, by, "supersede"); err != nil {
		return err
	}
	repl, err = artifacts.UpdateFrontMatter(env.Repo.Root, env.Cfg, repl, func(fm *artifacts.FrontMatter) {
		fm.AddLink("supersedes", old.ID)
	})
	if err != nil {
		return err
	}
	if _, _, err := artifacts.RecordWrite(env, repl.ID, repl.Kind, repl.Path, repl.SHA256, by, "link"); err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"superseded": old, "by": repl})
	}
	output.KV(c.Stdout, "SUPERSEDED", old.ID, "BY", repl.ID)
	return nil
}

func artLink(c *Ctx, noun string, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"to"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 || a.Flags["to"] == "" {
		return Usagef("huddle %s link <id> --to <id|url>", noun)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	x, err := artifacts.Resolve(env.Repo.Root, env.Cfg, noun, a.Pos[0])
	if err != nil {
		return err
	}
	to := a.Flags["to"]
	key, val := "external", to
	if !strings.Contains(to, "://") {
		target, err := artifacts.Resolve(env.Repo.Root, env.Cfg, "", to)
		if err != nil {
			return err
		}
		key, val = target.Kind, target.ID
		switch key {
		case "adr", "spike": // ADR 18: links {adrs[], spikes[]}
			key += "s"
		}
	}
	x, err = artifacts.UpdateFrontMatter(env.Repo.Root, env.Cfg, x, func(fm *artifacts.FrontMatter) { fm.AddLink(key, val) })
	if err != nil {
		return err
	}
	if _, _, err := artifacts.RecordWrite(env, x.ID, x.Kind, x.Path, x.SHA256, artifacts.CurrentUser(env.Cfg), "link"); err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, x)
	}
	output.KV(c.Stdout, "LINKED", x.ID, "LINK", key+": "+val)
	return nil
}

// artReadInput opens a file path or "-" for stdin.
func artReadInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}
