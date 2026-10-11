// Package eventlog is the append-only audit trail (ADR 14, 19). Committed
// artifacts are the source of truth; the log records what happened and the
// artifact hash at that moment.
package eventlog

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/paths"
)

// Event is one log line. Data holds type-specific fields.
type Event struct {
	ID       string         `json:"id"`
	TS       time.Time      `json:"ts"`
	Type     string         `json:"type"`
	By       string         `json:"by,omitempty"`
	Branch   string         `json:"branch,omitempty"`
	Text     string         `json:"text,omitempty"`
	Door     string         `json:"door,omitempty"` // one-way | two-way
	Refs     []string       `json:"refs,omitempty"`
	Artifact *ArtifactRef   `json:"artifact,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// ArtifactRef points at a committed artifact and its content hash.
type ArtifactRef struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

// NewID returns a sortable, collision-free event id.
func NewID(now time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return now.UTC().Format("20060102T150405.000Z") + "-" + hex.EncodeToString(b[:])
}

// Append writes e as one JSON line. O_APPEND writes under 4KB are atomic on
// POSIX, so concurrent huddles cannot interleave lines.
func Append(repoName string, e Event) (Event, error) {
	now := time.Now()
	if e.TS.IsZero() {
		e.TS = now.UTC()
	}
	if e.ID == "" {
		e.ID = NewID(now)
	}
	path := paths.Events(repoName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return e, err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return e, err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return e, err
}

// Filter selects events; zero values match everything.
type Filter struct {
	Types  map[string]bool
	Branch string
	Since  time.Time
	Limit  int // newest N after filtering; 0 = all
}

// List reads events oldest-first. Malformed lines are skipped, never fatal.
func List(repoName string, f Filter) ([]Event, error) {
	file, err := os.Open(paths.Events(repoName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []Event
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(f.Types) > 0 && !typeMayMatch(line, f.Types) {
			continue // cheap prefilter before decode
		}
		var e Event
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if len(f.Types) > 0 && !f.Types[e.Type] {
			continue
		}
		if f.Branch != "" && e.Branch != f.Branch {
			continue
		}
		if !f.Since.IsZero() && e.TS.Before(f.Since) {
			continue
		}
		out = append(out, e)
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out, sc.Err()
}

func typeMayMatch(line []byte, types map[string]bool) bool {
	for t := range types {
		if bytes.Contains(line, []byte(`"type":"`+t+`"`)) {
			return true
		}
	}
	return false
}

// summaryKeys are the Data keys worth showing first, in this order. Anything
// else in Data follows, sorted, so a new event type still renders something.
var summaryKeys = []string{"action", "gate", "name", "role", "verdict", "tier", "metric", "status", "key", "reason", "mode"}

// summarySkip are Data keys that are noise in a one-line summary: hashes,
// internal fingerprints, and values already shown elsewhere on the line.
var summarySkip = map[string]bool{
	"sha256": true, "fingerprint": true, "head": true, "artifactId": true,
	"findings": true, "hookReasons": true, "dismissed": true,
}

// Summary returns a one-line human description of the event. Events appended
// by the CLI itself (artifact.written, gate.request, verify, bands.*) carry no
// free-text `text` field — their meaning is in Artifact, Data and Refs — so a
// renderer that prints only Text shows a blank line for them. This builds the
// line from whatever the event has. It is a renderer helper only: nothing here
// changes what is written to the log, which stays append-only (ADR 14).
func (e Event) Summary() string {
	if s := strings.Join(strings.Fields(e.Text), " "); s != "" {
		return s
	}
	var parts []string
	if a := e.Artifact; a != nil {
		switch {
		case a.Kind != "" && a.Path != "":
			parts = append(parts, a.Kind+" "+a.Path)
		case a.Path != "":
			parts = append(parts, a.Path)
		case a.Kind != "":
			parts = append(parts, a.Kind)
		}
	}
	parts = append(parts, dataPairs(e.Data, e.Refs)...)
	if len(parts) == 0 && len(e.Refs) > 0 {
		parts = append(parts, "refs "+strings.Join(e.Refs, ","))
	}
	return strings.Join(parts, " · ")
}

// dataPairs renders scalar Data entries as key=value, priority keys first then
// the rest sorted, dropping noise and anything that only repeats Refs.
func dataPairs(data map[string]any, refs []string) []string {
	if len(data) == 0 {
		return nil
	}
	isRef := map[string]bool{}
	for _, r := range refs {
		isRef[r] = true
	}
	seen := map[string]bool{}
	rest := make([]string, 0, len(data))
	for k := range data {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	order := append(append([]string{}, summaryKeys...), rest...)
	out := make([]string, 0, len(data))
	for _, k := range order {
		if seen[k] || summarySkip[k] {
			continue
		}
		seen[k] = true
		v, ok := data[k]
		if !ok {
			continue
		}
		s := scalarString(v)
		if s == "" || isRef[s] {
			continue
		}
		out = append(out, k+"="+s)
		if len(out) == 6 {
			break
		}
	}
	return out
}

// scalarString formats a JSON scalar for a one-line summary; anything
// composite (object, array) returns "" and is left out.
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(t), " ")
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}
