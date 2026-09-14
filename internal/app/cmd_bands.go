package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muthuishere/huddle/internal/artifacts"
	"github.com/muthuishere/huddle/internal/bands"
	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/output"
	"github.com/muthuishere/huddle/internal/redact"
)

func init() {
	k, _ := artifacts.Get(config.Config{}, "bands")
	register(Command{Name: "bands", Summary: k.Summary + "; check a series", Run: runBands})
}

const bandsHelp = `Usage: huddle bands <verb> [options]

  check --series file.csv|- [--config bands.yaml] [--write] [--date YYYY-MM-DD]
          latest point vs baseline: <1σ ok, ≥1σ log, ≥2σ diagnose, ≥3σ breach
          a breach drafts intent/<metric>-breach-<date>/intent.md (dry run unless --write)
  dismiss <event-id> --reason "why"      dismiss a logged band signal (reason required)
  new | list | show | status | lint      the bands.yaml artifact
`

func runBands(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, bandsHelp)
		return nil
	}
	switch args[0] {
	case "check":
		return bandsCheck(c, args[1:])
	case "dismiss":
		return bandsDismiss(c, args[1:])
	}
	return artRunNoun(c, "bands", args)
}

func bandsCheck(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, []string{"write"}, []string{"config", "series", "date"})
	if err != nil {
		return Usagef("%v", err)
	}
	if a.Flags["series"] == "" {
		return Usagef("huddle bands check --series file.csv|- [--config bands.yaml]")
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	cfgPath := a.Flags["config"]
	if cfgPath == "" {
		k, _ := artifacts.Get(env.Cfg, "bands")
		cfgPath = filepath.Join(env.Repo.Root, filepath.FromSlash(k.Pattern))
	} else if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(c.Cwd, cfgPath)
	}
	src, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("%v — run: huddle bands new <metric>", err)
	}
	bc, err := bands.ParseConfig(src)
	if err != nil {
		return err
	}
	seriesPath := a.Flags["series"]
	if seriesPath != "-" && !filepath.IsAbs(seriesPath) {
		seriesPath = filepath.Join(c.Cwd, seriesPath)
	}
	in, err := artReadInput(seriesPath)
	if err != nil {
		return err
	}
	series, err := bands.ReadSeries(in)
	in.Close()
	if err != nil {
		return err
	}
	res, err := bands.Check(bc, series)
	if err != nil {
		return err
	}
	date := a.Flags["date"]
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", date); err != nil {
		return Usagef("--date must be YYYY-MM-DD")
	}

	out := map[string]any{"result": res}
	var ev eventlog.Event
	if res.Tier != bands.TierOK {
		ev, err = eventlog.Append(env.Repo.Name, eventlog.Event{
			Type: "bands." + res.Tier, By: artifacts.CurrentUser(env.Cfg), Branch: env.Repo.Branch,
			Text: fmt.Sprintf("%s at %.2fσ (%s)", res.Metric, res.Sigma, res.Tier),
			Data: map[string]any{"metric": res.Metric, "latest": res.Latest, "mean": res.Mean, "stddev": res.StdDev, "sigma": res.Sigma, "tier": res.Tier},
		})
		if err != nil {
			return err
		}
		out["event"] = ev.ID
	}
	var draft *artifacts.Draft
	written := false
	if res.Tier == bands.TierBreach {
		now, _ := time.Parse("2006-01-02", date)
		slug := artifacts.Slugify(res.Metric) + "-breach-" + date
		author := artifacts.CurrentUser(env.Cfg)
		d, err := artifacts.Render(env.Repo.Root, env.Cfg, "intent", artifacts.NewOptions{Author: author, Title: slug, Now: now})
		if err != nil {
			return err
		}
		d.Content = bandsFillProblem(d.Content, bands.BreachProblem(res), ev.ID)
		d.SHA256 = artifacts.SHA256([]byte(d.Content))
		draft = &d
		out["intent"] = map[string]any{"id": d.ID, "path": d.Path, "dryRun": !a.Bools["write"]}
		if a.Bools["write"] {
			if err := artifacts.Create(env.Repo.Root, d); err != nil && !errors.Is(err, artifacts.ErrExists) {
				return err
			} else if err == nil {
				written = true
				if _, _, err := artifacts.RecordWrite(env, d.ID, d.Kind, d.Path, d.SHA256, author, "bands.breach"); err != nil {
					return err
				}
			} else {
				out["intentExists"] = true
			}
		}
	}
	if a.Bools["json"] {
		if draft != nil && !written {
			out["content"] = draft.Content
		}
		return output.JSON(c.Stdout, out)
	}
	output.KV(c.Stdout, "METRIC", res.Metric, "TIER", strings.ToUpper(res.Tier),
		"LATEST", fmt.Sprintf("%g", res.Latest), "SIGMA", fmt.Sprintf("%.2f", res.Sigma),
		"BASELINE", fmt.Sprintf("mean %g, σ %g", res.Mean, res.StdDev))
	if ev.ID != "" {
		output.KV(c.Stdout, "EVENT", ev.ID)
	}
	switch {
	case written:
		output.KV(c.Stdout, "INTENT", draft.Path)
	case draft != nil && out["intentExists"] == true:
		output.KV(c.Stdout, "INTENT", draft.Path+" (already exists, left untouched)")
	case draft != nil:
		fmt.Fprintf(c.Stdout, "DRY RUN: would write %s — rerun with --write, or dismiss: huddle bands dismiss %s --reason \"…\"\n\n%s", draft.Path, ev.ID, draft.Content)
	}
	return nil
}

func bandsFillProblem(content, problem, eventID string) string {
	start := strings.Index(content, "## Problem")
	if start < 0 {
		return content
	}
	bodyStart := start + len("## Problem")
	end := strings.Index(content[bodyStart:], "\n## ")
	if end < 0 {
		end = len(content) - bodyStart
	}
	evidence := ""
	if eventID != "" {
		evidence = "\n\nEvidence: event `" + eventID + "` in `huddle log list`."
	}
	return content[:bodyStart] + "\n\n" + problem + evidence + "\n" + content[bodyStart+end:]
}

func bandsDismiss(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"reason"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 {
		return Usagef("huddle bands dismiss <event-id> --reason \"why\"")
	}
	reason := strings.TrimSpace(a.Flags["reason"])
	if reason == "" {
		return Usagef("dismissing a band signal needs --reason \"why\" (reasons tune the bands)")
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	evs, err := eventlog.List(env.Repo.Name, eventlog.Filter{Types: map[string]bool{"bands.log": true, "bands.diagnose": true, "bands.breach": true}})
	if err != nil {
		return err
	}
	var target *eventlog.Event
	for i := range evs {
		if evs[i].ID == a.Pos[0] {
			target = &evs[i]
		}
	}
	if target == nil {
		return fmt.Errorf("no band signal %q — find it with: huddle log list", a.Pos[0])
	}
	ev, err := eventlog.Append(env.Repo.Name, eventlog.Event{
		Type: "bands.dismissed", By: artifacts.CurrentUser(env.Cfg), Branch: env.Repo.Branch,
		Text: redact.String(reason), Refs: []string{target.ID},
		Data: map[string]any{"dismissed": target.ID, "tier": target.Data["tier"], "metric": target.Data["metric"]},
	})
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, ev)
	}
	output.KV(c.Stdout, "DISMISSED", target.ID, "EVENT", ev.ID)
	return nil
}
