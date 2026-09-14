// Package usage records one local line per CLI invocation and aggregates them
// for `huddle stats` and `huddle serve` (ADR 20). Nothing is ever sent anywhere.
// STUB: the usage build task implements Record and Aggregate; signatures are the contract.
package usage

import "time"

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
}

// Summary is the aggregate shown by stats and serve.
type Summary struct {
	Since    time.Time      `json:"since"`
	Total    int            `json:"total"`
	Agent    int            `json:"agent"`
	Human    int            `json:"human"`
	Commands []CmdStat      `json:"commands"`
	Skills   map[string]int `json:"skills"`
	Repos    map[string]int `json:"repos"`
	PerDay   map[string]int `json:"perDay"`
}

// Record appends e to the usage file unless usage is disabled. Errors are swallowed.
func Record(e Entry) {}

// Aggregate reads usage since the given time (zero = all), optionally for one repo.
func Aggregate(since time.Time, repo string) (Summary, error) { return Summary{}, nil }
