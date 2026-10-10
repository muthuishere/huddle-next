package serve

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/paths"
)

// EventView is an event plus whether a later supersede event replaced it.
type EventView struct {
	eventlog.Event
	Superseded bool `json:"superseded,omitempty"`
}

// EventQuery selects a newest-first page of the log.
type EventQuery struct {
	Types  map[string]bool
	Door   string
	Branch string
	Since  time.Time
	Before string // exclusive event id cursor; ids sort by time
	Limit  int
}

// EventPage is one page of events, newest first.
type EventPage struct {
	Events     []EventView `json:"events"`
	NextBefore string      `json:"nextBefore,omitempty"`
	Scanned    int         `json:"scanned"`
}

const maxEventLimit = 500

// PageEvents streams the log once, keeping only a ring of `limit` matches, so
// memory stays O(limit) however large the log is.
func PageEvents(repoName string, q EventQuery) (EventPage, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > maxEventLimit {
		q.Limit = maxEventLimit
	}
	page := EventPage{Events: []EventView{}}
	f, err := os.Open(paths.Events(repoName))
	if errors.Is(err, os.ErrNotExist) {
		return page, nil
	}
	if err != nil {
		return page, err
	}
	defer f.Close()
	ring := make([]eventlog.Event, q.Limit)
	n, head := 0, 0 // n = matches seen, head = next write slot
	superseded := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		page.Scanned++
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e eventlog.Event
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if e.Type == "supersede" {
			for _, ref := range e.Refs {
				superseded[ref] = true
			}
		}
		if q.Before != "" && e.ID >= q.Before {
			continue
		}
		if len(q.Types) > 0 && !q.Types[e.Type] {
			continue
		}
		if q.Door != "" && e.Door != q.Door {
			continue
		}
		if q.Branch != "" && e.Branch != q.Branch {
			continue
		}
		if !q.Since.IsZero() && e.TS.Before(q.Since) {
			continue
		}
		ring[head] = e
		head = (head + 1) % q.Limit
		n++
	}
	if err := sc.Err(); err != nil {
		return page, err
	}
	kept := n
	if kept > q.Limit {
		kept = q.Limit
	}
	for i := 1; i <= kept; i++ { // newest first
		e := ring[(head-i+q.Limit)%q.Limit]
		page.Events = append(page.Events, EventView{Event: e, Superseded: superseded[e.ID]})
	}
	if n > q.Limit {
		page.NextBefore = page.Events[len(page.Events)-1].ID
	}
	return page, nil
}

func (s *Server) apiEvents(t target, r *http.Request) (any, error) {
	v := r.URL.Query()
	q := EventQuery{Door: v.Get("door"), Branch: v.Get("branch"), Before: v.Get("before")}
	if types := strings.TrimSpace(v.Get("type")); types != "" {
		q.Types = map[string]bool{}
		for _, x := range strings.Split(types, ",") {
			if x = strings.TrimSpace(x); x != "" {
				q.Types[x] = true
			}
		}
	}
	if l := v.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			return nil, errStatus(http.StatusBadRequest, "limit must be a non-negative integer")
		}
		q.Limit = n
	}
	if since := v.Get("since"); since != "" {
		ts, err := parseSince(since, s.now())
		if err != nil {
			return nil, err
		}
		q.Since = ts
	}
	return PageEvents(t.info.Name, q)
}

// parseSince accepts "", "all", "<n>d", "<n>h", or a YYYY-MM-DD date.
func parseSince(v string, now time.Time) (time.Time, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "all" {
		return time.Time{}, nil
	}
	if ts, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return ts, nil
	}
	if len(v) > 1 {
		n, err := strconv.Atoi(v[:len(v)-1])
		if err == nil && n >= 0 {
			switch v[len(v)-1] {
			case 'd':
				return now.Add(-time.Duration(n) * 24 * time.Hour), nil
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			}
		}
	}
	return time.Time{}, errStatus(http.StatusBadRequest, "since must be all, <n>d, <n>h, or YYYY-MM-DD")
}

// RepoRow is one repo with huddle state on this machine.
type RepoRow struct {
	Name       string    `json:"name"`
	Events     int       `json:"events"`
	LastWrite  time.Time `json:"lastWrite,omitzero"`
	Root       string    `json:"root,omitempty"`
	Switchable bool      `json:"switchable"`
	Current    bool      `json:"current"`
}

func (s *Server) apiRepos(t target, r *http.Request) (any, error) {
	dir := filepath.Join(paths.Global(), "repos")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	roots := knownRoots()
	rows := []RepoRow{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		row := RepoRow{Name: e.Name(), Root: roots[e.Name()], Current: e.Name() == s.repo.Name}
		row.Switchable = row.Root != "" || row.Current
		if row.Current {
			row.Root = s.repo.Root
		}
		ev := paths.Events(e.Name())
		if st, err := os.Stat(ev); err == nil {
			row.LastWrite = st.ModTime().UTC()
			row.Events, _ = countLines(ev)
		}
		rows = append(rows, row)
	}
	return map[string]any{"repos": rows, "current": s.repo.Name, "home": paths.Global()}, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n := 0
	for {
		c, err := f.Read(buf)
		n += bytes.Count(buf[:c], []byte{'\n'})
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
	}
}
