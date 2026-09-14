package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/hooks"
	"github.com/muthuishere/huddle/internal/output"
)

func init() {
	register(Command{
		Name:    "hooks",
		Summary: "List, test, fire, and export lifecycle hooks (config.json hooks[])",
		Run:     runHooks,
	})
}

const hooksHelp = `
Usage:
  huddle hooks list [--json]
  huddle hooks test <event> [--payload file|-] [--json]
  huddle hooks fire <event> [--payload file|-] [--json]
  huddle hooks export --host claude [--write] [--force] [--json]

Hooks live in hooks[] of ~/.config/.huddle/config.json (global, run first) and
<repo>/.huddle/config.json (repo): {on, match, run|prompt, blocking, timeout, async}.
A hook gets the payload + event + ts as JSON on stdin and may print
{decision, reason, context, ingest, links, log}. Output is redacted.

test runs matching hooks and prints the combined result. fire does the same
and exits non-zero when the decision is not allow. A blocking hook that denies,
fails, or times out keeps the gate closed until a human runs:
  huddle gate approve <gate> --override --reason "<why>"

export drafts a Claude Code pack (.claude/settings.json + .claude/hooks/*.sh):
protected paths, no test edits in fix mode, plan drift reminder, production
gate, post-edit format/lint, inner-loop permissions.allow, plus a managed
settings snippet for platform teams. Nothing is written without --write;
existing files are never replaced without --force.
Config keys: protectedPaths, deployPatterns, formatCommand, lintCommand, innerLoop.
`

func runHooks(c *Ctx, args []string) error {
	a, err := parseCoreArgs("hooks", args, []string{"payload", "host"}, []string{"json", "write", "force"})
	if err != nil {
		return err
	}
	if a.bools["help"] || len(a.pos) == 0 {
		return coreHelp(c, hooksHelp)
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	cfg, err := config.Load(info.Root)
	if err != nil {
		return err
	}
	switch sub := a.pos[0]; sub {
	case "list":
		return hooksList(c, cfg, a.bools["json"], func() error { return writeHooksSeen(info) })
	case "test", "fire":
		if len(a.pos) < 2 {
			return Usagef("hooks %s needs an event, e.g. huddle hooks %s gate.request --payload payload.json", sub, sub)
		}
		payload, err := readPayload(a.values["payload"], os.Stdin)
		if err != nil {
			return err
		}
		res := hooks.Fire(cfg, info.Root, a.pos[1], payload)
		if err := printHookResult(c, res, a.bools["json"]); err != nil {
			return err
		}
		if sub == "fire" && res.Decision != hooks.Allow {
			return fmt.Errorf("hooks decided %q for %s — a human can override with: huddle gate approve <gate> --override --reason \"<why>\"", res.Decision, a.pos[1])
		}
		return nil
	case "export":
		host := a.values["host"]
		if host == "" {
			host = "claude"
		}
		if host != "claude" {
			return Usagef("hooks export supports --host claude only")
		}
		return hooksExport(c, cfg, info.Root, a.bools["write"], a.bools["force"], a.bools["json"])
	default:
		return Usagef("unknown hooks subcommand %q — run: huddle hooks --help", sub)
	}
}

func hooksList(c *Ctx, cfg config.Config, asJSON bool, markSeen func() error) error {
	type row struct {
		config.Hook
		Scope   string `json:"scope"`
		Timeout string `json:"timeout"`
	}
	rows := make([]row, 0, len(cfg.Hooks))
	for _, h := range cfg.Hooks {
		rows = append(rows, row{Hook: h, Scope: h.Scope, Timeout: hooks.Timeout(h).String()})
	}
	// Listing is the review step `huddle start` points at; failure to record it is not fatal.
	seenErr := markSeen()
	if asJSON {
		return output.JSON(c.Stdout, map[string]any{"hooks": rows})
	}
	if len(rows) == 0 {
		fmt.Fprintln(c.Stdout, "No hooks. Add one to .huddle/config.json, e.g.:")
		fmt.Fprintln(c.Stdout, `  {"hooks":[{"on":"gate.request","match":{"stage":"deploy"},"run":".huddle/hooks/pr-approved.sh","blocking":true}]}`)
		return nil
	}
	var table [][]string
	for _, r := range rows {
		cmd := r.Run
		if cmd == "" {
			cmd = "prompt: " + r.Prompt
		}
		flags := []string{}
		if r.Blocking {
			flags = append(flags, "blocking")
		}
		if r.Async {
			flags = append(flags, "async")
		}
		table = append(table, []string{r.Scope, r.On, matchString(r.Match), coreTruncate(cmd, 60), strings.Join(flags, ","), r.Timeout})
	}
	output.Table(c.Stdout, []string{"SCOPE", "EVENT", "MATCH", "RUN", "FLAGS", "TIMEOUT"}, table)
	if seenErr != nil {
		fmt.Fprintln(c.Stderr, "huddle: could not record hooks review:", seenErr)
	}
	return nil
}

func matchString(m map[string]string) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}

func readPayload(path string, stdin io.Reader) (hooks.Payload, error) {
	if path == "" {
		return hooks.Payload{}, nil
	}
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	p := hooks.Payload{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, Usagef("--payload must be a JSON object: %v", err)
	}
	return p, nil
}

func printHookResult(c *Ctx, r hooks.Result, asJSON bool) error {
	if asJSON {
		return output.JSON(c.Stdout, r)
	}
	output.KV(c.Stdout, "DECISION", r.Decision, "RAN", fmt.Sprint(r.Ran))
	section := func(label string, items []string) {
		for _, s := range items {
			output.KV(c.Stdout, label, s)
		}
	}
	section("REASON", r.Reasons)
	section("CONTEXT", r.Context)
	for _, l := range r.Links {
		output.KV(c.Stdout, "LINK", strings.TrimSpace(l.System+" "+l.ID+" "+l.URL))
	}
	if n := len(r.Ingest); n > 0 {
		output.KV(c.Stdout, "INGEST", fmt.Sprintf("%d item(s)", n))
	}
	if n := len(r.Log); n > 0 {
		output.KV(c.Stdout, "LOG", fmt.Sprintf("%d entr(ies)", n))
	}
	section("ERROR", r.Errors)
	return nil
}

func hooksExport(c *Ctx, cfg config.Config, root string, write, force, asJSON bool) error {
	pack := hooks.ExportClaude(cfg)
	if !write {
		if asJSON {
			return output.JSON(c.Stdout, pack)
		}
		w := c.Stdout
		fmt.Fprintln(w, "# DRAFT — Claude Code hook pack. Nothing was written. Review, then: huddle hooks export --host claude --write")
		fmt.Fprintf(w, "\n## %s (merge into the existing file)\n", hooks.PackSettingsPath)
		b, _ := json.MarshalIndent(pack.Settings, "", "  ")
		fmt.Fprintln(w, string(b))
		names := make([]string, 0, len(pack.Files))
		for n := range pack.Files {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(w, "\n## %s\n%s", n, pack.Files[n])
		}
		fmt.Fprintln(w, "\n## Managed settings — for platform teams (managed-settings.json; huddle never writes this)")
		b, _ = json.MarshalIndent(pack.Managed, "", "  ")
		fmt.Fprintln(w, string(b))
		for _, note := range pack.Notes {
			fmt.Fprintln(w, "\nNOTE:", note)
		}
		return nil
	}
	report, err := hooks.WritePack(pack, root, force)
	if asJSON {
		if jerr := output.JSON(c.Stdout, map[string]any{"written": report, "managedSettings": pack.Managed, "notes": pack.Notes}); jerr != nil {
			return jerr
		}
		return err
	}
	rows := make([][]string, 0, len(report))
	for _, r := range report {
		rows = append(rows, []string{r.Path, r.Action})
	}
	output.Table(c.Stdout, []string{"PATH", "ACTION"}, rows)
	fmt.Fprintln(c.Stdout, "Managed settings for platform teams (not written): huddle hooks export --host claude")
	for _, note := range pack.Notes {
		fmt.Fprintln(c.Stdout, "NOTE:", note)
	}
	return err
}
