package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle/internal/output"
	"github.com/muthuishere/huddle/internal/usage"
)

func init() {
	register(Command{
		Name:    "stats",
		Summary: "Local-only CLI and skill usage, latency vs budgets, and flow metrics",
		Run:     runStats,
	})
}

const statsHelp = `Usage: huddle stats [--since 30d|7d|all] [--repo <name> | --all-repos] [--json]

Reads ~/.config/.huddle/usage.jsonl (one line per huddle invocation, no
arguments, never sent anywhere) and the repo event log. Shows totals, human vs
agent share, commands with count/errors/p50/p95 against budgets, skills, top
repos, a per-day sparkline, and flow metrics for the repo.
Defaults: --since 30d, the current repo (all repos outside a project).
Opt out: huddle config set usage false --scope global, or HUDDLE_NO_USAGE=1.
`

// StatsResult is the `huddle stats --json` payload.
type StatsResult struct {
	Repo    string        `json:"repo,omitempty"` // "" = all repos
	Usage   usage.Summary `json:"usage"`
	Flow    *usage.Flow   `json:"flow,omitempty"`
	Enabled bool          `json:"enabled"`
}

func runStats(c *Ctx, args []string) error {
	a, err := parseCoreArgs("stats", args, []string{"since", "repo"}, []string{"json", "all-repos"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, statsHelp)
	}
	sinceArg, ok := a.values["since"]
	if !ok {
		sinceArg = "30d"
	}
	since, err := usage.ParseSince(sinceArg, time.Now())
	if err != nil {
		return Usagef("%v — run: huddle stats --help", err)
	}
	if a.values["repo"] != "" && a.bools["all-repos"] {
		return Usagef("use --repo or --all-repos, not both — run: huddle stats --help")
	}
	res := StatsResult{}
	root := ""
	switch {
	case a.values["repo"] != "":
		res.Repo = a.values["repo"]
		if info, err := c.Repo(); err == nil && info.Name == res.Repo {
			root = info.Root
		}
	default:
		if info, err := c.Repo(); err == nil {
			root = info.Root
			if !a.bools["all-repos"] {
				res.Repo = info.Name
			}
		}
	}
	res.Enabled = !usage.Disabled(root)
	if res.Usage, err = usage.Aggregate(since, res.Repo); err != nil {
		return fmt.Errorf("read usage: %w", err)
	}
	flowRepo := res.Repo
	if flowRepo == "" && c.repo != nil {
		flowRepo = c.repo.Name
	}
	if flowRepo != "" {
		if f, err := usage.FlowMetricsAt(flowRepo, root); err == nil {
			res.Flow = &f
		}
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, res)
	}
	renderStats(c.Stdout, res, sinceArg, time.Now())
	return nil
}

func pct(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", float64(n)*100/float64(total))
}

func fmtMS(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func renderStats(w io.Writer, r StatsResult, sinceArg string, now time.Time) {
	s := r.Usage
	scope := r.Repo
	if scope == "" {
		scope = "all repos"
	}
	window := "last " + sinceArg
	if s.Since.IsZero() {
		window = "all time"
	}
	enabled := "on"
	if !r.Enabled {
		enabled = "off (opted out)"
	}
	output.KV(w,
		"SCOPE", scope+", "+window,
		"USAGE_RECORDING", enabled,
		"INVOCATIONS", fmt.Sprint(s.Total),
		"HUMAN", fmt.Sprintf("%d (%s)", s.Human, pct(s.Human, s.Total)),
		"AGENT", fmt.Sprintf("%d (%s)", s.Agent, pct(s.Agent, s.Total)),
		"ERRORS", fmt.Sprintf("%d (%s)", s.Errors, pct(s.Errors, s.Total)),
	)
	if s.Total == 0 {
		fmt.Fprintln(w, "\nNo usage recorded yet — run some huddle commands, then: huddle stats")
	} else {
		fmt.Fprintln(w, "\nCommands")
		rows := make([][]string, 0, len(s.Commands))
		for _, cs := range s.Commands {
			budget := "-"
			if cs.BudgetMS > 0 {
				mark := "ok"
				if cs.P50MS > cs.BudgetMS {
					mark = "OVER"
				}
				budget = fmt.Sprintf("p50≤%s %s", fmtMS(cs.BudgetMS), mark)
			}
			rows = append(rows, []string{cs.Cmd, fmt.Sprint(cs.Count), fmt.Sprint(cs.Errors), fmtMS(cs.P50MS), fmtMS(cs.P95MS), pct(cs.Agent, cs.Count), budget})
		}
		output.Table(w, []string{"  COMMAND", "COUNT", "ERRORS", "P50_MS", "P95_MS", "AGENT", "BUDGET"}, indentRows(rows))

		if len(s.Skills) > 0 {
			fmt.Fprintln(w, "\nSkills")
			output.Table(w, []string{"  SKILL", "COUNT"}, indentRows(topCounts(s.Skills, 0)))
		}
		if r.Repo == "" && len(s.Repos) > 0 {
			fmt.Fprintln(w, "\nTop repos")
			output.Table(w, []string{"  REPO", "COUNT"}, indentRows(topCounts(s.Repos, 10)))
		}
		fmt.Fprintln(w, "\nPer day")
		fmt.Fprintln(w, "  "+sparkline(s.PerDay, s.Since, now))
	}
	if f := r.Flow; f != nil {
		fmt.Fprintf(w, "\nFlow (%s)\n", f.Repo)
		stale := "not checked (outside the repo)"
		if f.StaleChecked {
			stale = fmt.Sprint(f.StaleVerdicts)
		}
		firstPass := "-"
		if f.VerifiedChanges > 0 {
			firstPass = fmt.Sprintf("%.0f%% (%d/%d)", f.FirstPassRate*100, f.FirstPassPASS, f.VerifiedChanges)
		}
		wait := "-"
		if f.GatesApproved+f.GatesRejected > 0 {
			wait = fmt.Sprintf("p50 %s, p95 %s", fmtMinutes(f.GateWaitP50Min), fmtMinutes(f.GateWaitP95Min))
		}
		output.KV(w,
			"  EVENTS", fmt.Sprint(f.Events),
			"  GATES", fmt.Sprintf("%d requested, %d approved, %d rejected, %d pending", f.GatesRequested, f.GatesApproved, f.GatesRejected, f.GatesPending),
			"  GATE_WAIT", wait,
			"  OVERRIDES", fmt.Sprint(f.Overrides),
			"  VERDICTS", fmt.Sprint(f.Verdicts),
			"  FIRST_PASS_PASS", firstPass,
			"  STALE_VERDICTS", stale,
		)
	}
}

func fmtMinutes(m float64) string {
	switch {
	case m >= 60*24:
		return fmt.Sprintf("%.1fd", m/60/24)
	case m >= 60:
		return fmt.Sprintf("%.1fh", m/60)
	default:
		return fmt.Sprintf("%.0fm", m)
	}
}

func indentRows(rows [][]string) [][]string {
	for _, r := range rows {
		if len(r) > 0 {
			r[0] = "  " + r[0]
		}
	}
	return rows
}

// topCounts sorts a count map descending (ties by name); limit 0 = all.
func topCounts(m map[string]int, limit int) [][]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	rows := make([][]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, []string{k, fmt.Sprint(m[k])})
	}
	return rows
}

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkline renders one block per local day from since (or the first recorded
// day) through today, capped at the last 60 days.
func sparkline(perDay map[string]int, since, now time.Time) string {
	if len(perDay) == 0 {
		return "(no data)"
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	start := today.AddDate(0, 0, -59)
	if !since.IsZero() {
		s := since.Local()
		start = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, time.Local)
	} else {
		days := make([]string, 0, len(perDay))
		for d := range perDay {
			days = append(days, d)
		}
		sort.Strings(days)
		if t, err := time.ParseInLocation("2006-01-02", days[0], time.Local); err == nil {
			start = t
		}
	}
	if today.Sub(start) > 59*24*time.Hour {
		start = today.AddDate(0, 0, -59)
	}
	var counts []int
	max := 0
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		n := perDay[d.Format("2006-01-02")]
		counts = append(counts, n)
		if n > max {
			max = n
		}
	}
	var b strings.Builder
	for _, n := range counts {
		switch {
		case n == 0:
			b.WriteRune(' ')
		default:
			b.WriteRune(sparkBlocks[(n*(len(sparkBlocks)-1)+max-1)/max])
		}
	}
	return fmt.Sprintf("%s → %s  |%s|  max %d/day", start.Format("01-02"), today.Format("01-02"), b.String(), max)
}
