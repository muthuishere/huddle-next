// Package usage records one local line per CLI invocation and aggregates them
// for `huddle stats` and `huddle serve` (ADR 20). Nothing is ever sent anywhere.
package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle/internal/paths"
)

// Entry is one CLI invocation. It never contains arguments or free text.
type Entry struct {
	TS      time.Time `json:"ts"`
	Cmd     string    `json:"cmd"`
	Sub     string    `json:"sub,omitempty"`
	MS      float64   `json:"ms"`
	Exit    int       `json:"exit"`
	Agent   bool      `json:"agent"`
	Skill   string    `json:"skill,omitempty"`
	Repo    string    `json:"repo,omitempty"`
	Version string    `json:"version"`
}

// CmdStat summarises one command.
type CmdStat struct {
	Cmd    string  `json:"cmd"`
	Count  int     `json:"count"`
	Errors int     `json:"errors"`
	P50MS  float64 `json:"p50ms"`
	P95MS  float64 `json:"p95ms"`
	Agent  int     `json:"agent"`
	Human  int     `json:"human"`
	// BudgetMS is the p50 performance budget for the command (0 = none).
	BudgetMS float64 `json:"budgetMs,omitempty"`
}

// Summary is the aggregate shown by stats and serve.
type Summary struct {
	Since    time.Time      `json:"since"`
	Total    int            `json:"total"`
	Agent    int            `json:"agent"`
	Human    int            `json:"human"`
	Errors   int            `json:"errors"`
	Commands []CmdStat      `json:"commands"`
	Skills   map[string]int `json:"skills"`
	Repos    map[string]int `json:"repos"`
	PerDay   map[string]int `json:"perDay"`
}

// MaxBytes is the size at which usage.jsonl rotates to usage.jsonl.1.
const MaxBytes = 10 << 20

// EnvOptOut disables recording when set to 1/true.
const EnvOptOut = "HUDDLE_NO_USAGE"

// Budgets are the p50 latency budgets (ms) enforced elsewhere by benchmarks.
var Budgets = map[string]float64{"start": 60}

// Disabled reports whether recording is off: HUDDLE_NO_USAGE, else a `usage`
// key set to false in the repo config (when repoRoot is known) or the global
// config. Only those two small files are read.
func Disabled(repoRoot string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvOptOut))) {
	case "1", "true", "yes", "on":
		return true
	}
	if repoRoot != "" {
		if v, ok := usageKey(paths.RepoConfig(repoRoot)); ok {
			return !v
		}
	}
	if v, ok := usageKey(paths.GlobalConfig()); ok {
		return !v
	}
	return false
}

// usageKey reads the top-level `usage` bool from a config file, if present.
func usageKey(path string) (bool, bool) {
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte(`"usage"`)) {
		return false, false
	}
	var m struct {
		Usage *json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(b, &m) != nil || m.Usage == nil {
		return false, false
	}
	switch strings.Trim(strings.TrimSpace(string(*m.Usage)), `"`) {
	case "false", "0", "off", "no":
		return false, true
	}
	return true, true
}

// Record appends e to the usage file unless usage is disabled. Errors are swallowed.
func Record(e Entry) { RecordFor(e, "") }

// RecordFor is Record with the repo root (when already known) so a repo-scoped
// `usage: false` is honoured. Errors are swallowed.
func RecordFor(e Entry, repoRoot string) {
	if Disabled(repoRoot) {
		return
	}
	_ = write(e)
}

func write(e Entry) error {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	e.TS = e.TS.UTC()
	e.MS = float64(int64(e.MS*1000+0.5)) / 1000
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	path := paths.Usage()
	if st, err := os.Stat(path); err == nil {
		if st.Size() > MaxBytes {
			_ = os.Rename(path, path+".1")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Aggregate reads usage since the given time (zero = all), optionally for one repo.
func Aggregate(since time.Time, repo string) (Summary, error) {
	s := Summary{Since: since, Commands: []CmdStat{}, Skills: map[string]int{}, Repos: map[string]int{}, PerDay: map[string]int{}}
	f, err := os.Open(paths.Usage())
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	err = aggregate(f, since, repo, &s)
	return s, err
}

type cmdAcc struct {
	stat CmdStat
	ms   []float64
}

func aggregate(r interface{ Read([]byte) (int, error) }, since time.Time, repo string, s *Summary) error {
	cmds := map[string]*cmdAcc{}
	days := map[int64]string{} // 15-min bucket -> local YYYY-MM-DD, cached
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var e Entry
	for sc.Scan() {
		e = Entry{}
		if !parseLine(sc.Bytes(), &e) || e.Cmd == "" {
			continue
		}
		if !since.IsZero() && e.TS.Before(since) {
			continue
		}
		if repo != "" && e.Repo != repo {
			continue
		}
		s.Total++
		if e.Agent {
			s.Agent++
		} else {
			s.Human++
		}
		acc := cmds[e.Cmd]
		if acc == nil {
			acc = &cmdAcc{stat: CmdStat{Cmd: e.Cmd, BudgetMS: Budgets[e.Cmd]}}
			cmds[e.Cmd] = acc
		}
		acc.stat.Count++
		if e.Exit != 0 {
			acc.stat.Errors++
			s.Errors++
		}
		if e.Agent {
			acc.stat.Agent++
		} else {
			acc.stat.Human++
		}
		acc.ms = append(acc.ms, e.MS)
		if e.Skill != "" {
			s.Skills[e.Skill]++
		}
		if e.Repo != "" {
			s.Repos[e.Repo]++
		}
		key := e.TS.Unix() / 900 // 15-min bucket: every zone offset is a multiple of 15 min
		day, ok := days[key]
		if !ok {
			day = e.TS.Local().Format("2006-01-02")
			days[key] = day
		}
		s.PerDay[day]++
	}
	for _, acc := range cmds {
		sort.Float64s(acc.ms)
		acc.stat.P50MS = percentile(acc.ms, 50)
		acc.stat.P95MS = percentile(acc.ms, 95)
		s.Commands = append(s.Commands, acc.stat)
	}
	sort.Slice(s.Commands, func(i, j int) bool {
		if s.Commands[i].Count != s.Commands[j].Count {
			return s.Commands[i].Count > s.Commands[j].Count
		}
		return s.Commands[i].Cmd < s.Commands[j].Cmd
	})
	return sc.Err()
}

// percentile uses nearest-rank on a sorted slice.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted)*p + 99) / 100
	if i < 1 {
		i = 1
	}
	if i > len(sorted) {
		i = len(sorted)
	}
	return sorted[i-1]
}

// parseLine decodes one flat usage line. It hand-scans the object (usage lines
// are flat, written by this package) and falls back to encoding/json for
// anything unusual such as escaped strings.
func parseLine(b []byte, e *Entry) bool {
	b = bytes.TrimSpace(b)
	if len(b) < 2 || b[0] != '{' || b[len(b)-1] != '}' {
		return false
	}
	if bytes.IndexByte(b, '\\') >= 0 {
		return json.Unmarshal(b, e) == nil
	}
	i := 1
	n := len(b) - 1
	skipWS := func() {
		for i < n && (b[i] == ' ' || b[i] == '\t' || b[i] == ',') {
			i++
		}
	}
	for {
		skipWS()
		if i >= n {
			return true
		}
		if b[i] != '"' {
			return json.Unmarshal(b, e) == nil
		}
		end := bytes.IndexByte(b[i+1:n], '"')
		if end < 0 {
			return false
		}
		key := b[i+1 : i+1+end]
		i += end + 2
		for i < n && (b[i] == ' ' || b[i] == ':') {
			i++
		}
		if i >= n {
			return false
		}
		var sval []byte
		var raw []byte
		if b[i] == '"' {
			end := bytes.IndexByte(b[i+1:n], '"')
			if end < 0 {
				return false
			}
			sval = b[i+1 : i+1+end]
			i += end + 2
		} else {
			j := i
			for j < n && b[j] != ',' && b[j] != ' ' {
				j++
			}
			raw = b[i:j]
			i = j
			if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
				return json.Unmarshal(b, e) == nil
			}
		}
		switch string(key) {
		case "ts":
			t, err := time.Parse(time.RFC3339Nano, string(sval))
			if err != nil {
				return false
			}
			e.TS = t
		case "cmd":
			e.Cmd = string(sval)
		case "sub":
			e.Sub = string(sval)
		case "skill":
			e.Skill = string(sval)
		case "repo":
			e.Repo = string(sval)
		case "version":
			e.Version = string(sval)
		case "ms":
			f, err := strconv.ParseFloat(string(raw), 64)
			if err != nil {
				return false
			}
			e.MS = f
		case "exit":
			v, err := strconv.Atoi(string(raw))
			if err != nil {
				return false
			}
			e.Exit = v
		case "agent":
			e.Agent = string(raw) == "true"
		}
	}
}

// ParseSince turns "30d", "7d", "12h", "all" or "" into a cutoff (zero = all).
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "all" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return time.Time{}, errors.New("--since takes Nd, Nh or all (e.g. 30d)")
		}
		return now.AddDate(0, 0, -n), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return time.Time{}, errors.New("--since takes Nd, Nh or all (e.g. 30d)")
	}
	return now.Add(-d), nil
}
