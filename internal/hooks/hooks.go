// Package hooks runs user lifecycle hooks declared in config.json (ADR 17, 19).
//
// Contract: the hook gets the payload (plus event and ts) as JSON on stdin and
// HUDDLE_EVENT / HUDDLE_REPO_ROOT in its environment; it may print one JSON
// object {decision, reason, context, ingest, links, log}. Everything a hook
// prints is redacted before it reaches the caller.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/redact"
)

// Payload is the JSON sent to a hook on stdin.
type Payload map[string]any

// Link is an external reference a hook reports (e.g. a Jira issue).
type Link struct {
	System   string `json:"system"`
	ID       string `json:"id,omitempty"`
	URL      string `json:"url,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// Result is the combined outcome of every hook matching an event.
type Result struct {
	// Decision is "allow", "deny", or "ask". Only blocking hooks can make it non-allow.
	Decision string           `json:"decision"`
	Reasons  []string         `json:"reasons,omitempty"`
	Context  []string         `json:"context,omitempty"`
	Links    []Link           `json:"links,omitempty"`
	Ingest   []map[string]any `json:"ingest,omitempty"`
	Log      []map[string]any `json:"log,omitempty"`
	Errors   []string         `json:"errors,omitempty"`
	Ran      int              `json:"ran"`
}

// Decisions.
const (
	Allow = "allow"
	Deny  = "deny"
	Ask   = "ask"
)

// DefaultTimeout applies when a hook sets none.
const DefaultTimeout = 10 * time.Second

const maxOutput = 1 << 20

// Fire runs every hook in cfg whose On equals event and whose Match keys equal
// payload values. Non-blocking hooks never change Decision.
func Fire(cfg config.Config, repoRoot, event string, payload Payload) Result {
	res := Result{Decision: Allow}
	matched := Select(cfg.Hooks, event, payload)
	if len(matched) == 0 {
		return res
	}
	stdin, err := stdinJSON(event, payload)
	if err != nil {
		res.Errors = append(res.Errors, "encode payload: "+err.Error())
		return res
	}
	for _, h := range matched {
		label := Label(h)
		switch {
		case strings.TrimSpace(h.Run) == "":
			res.Context = append(res.Context, "agent hook: "+redact.String(strings.TrimSpace(h.Prompt)))
		case h.Async:
			res.Ran++
			if err := startDetached(h, repoRoot, event, payload, stdin); err != nil {
				res.Errors = append(res.Errors, label+": start async: "+redact.String(err.Error()))
			}
		default:
			res.Ran++
			runOne(&res, h, label, repoRoot, event, payload, stdin)
		}
	}
	return res
}

// Select returns the hooks for event whose match keys all equal payload values,
// global hooks first, then repo, each in declared order.
func Select(all []config.Hook, event string, payload Payload) []config.Hook {
	var out []config.Hook
	for _, h := range all {
		if h.On != event || (strings.TrimSpace(h.Run) == "" && strings.TrimSpace(h.Prompt) == "") {
			continue
		}
		ok := true
		for k, want := range h.Match {
			if got, present := scalar(payload[k]); !present || got != want {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return scopeRank(out[i].Scope) < scopeRank(out[j].Scope) })
	return out
}

// Label names a hook in messages: "<scope> <on>: <run|prompt>".
func Label(h config.Hook) string {
	cmd := h.Run
	if cmd == "" {
		cmd = "prompt"
	}
	scope := h.Scope
	if scope == "" {
		scope = "config"
	}
	return fmt.Sprintf("%s %s: %s", scope, h.On, redact.String(cmd))
}

// Timeout parses a hook timeout ("20s", "1m", or bare seconds); invalid or empty is the default.
func Timeout(h config.Hook) time.Duration {
	s := strings.TrimSpace(h.Timeout)
	if s == "" {
		return DefaultTimeout
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return DefaultTimeout
}

func scopeRank(s string) int {
	switch s {
	case string(config.Global):
		return 0
	case string(config.Repo):
		return 1
	}
	return 2
}

func scalar(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case bool, float64, float32, int, int64, json.Number:
		return fmt.Sprint(t), true
	}
	return "", false
}

func stdinJSON(event string, payload Payload) ([]byte, error) {
	in := make(map[string]any, len(payload)+2)
	for k, v := range payload {
		in[k] = v
	}
	in["event"] = event
	in["ts"] = time.Now().UTC().Format(time.RFC3339)
	return json.Marshal(in)
}

func shell(ctx context.Context, run string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", run)
	}
	return exec.CommandContext(ctx, "sh", "-c", run)
}

func env(repoRoot, event string, payload Payload) []string {
	e := append(os.Environ(), "HUDDLE_EVENT="+event, "HUDDLE_REPO_ROOT="+repoRoot)
	if v, ok := scalar(payload["change"]); ok {
		e = append(e, "HUDDLE_CHANGE="+v)
	}
	// HUDDLE_AGENT is inherited from os.Environ when set; it is never invented here.
	return e
}

// hookOutput is the optional JSON a hook prints.
type hookOutput struct {
	Decision string           `json:"decision"`
	Reason   string           `json:"reason"`
	Context  json.RawMessage  `json:"context"`
	Ingest   []map[string]any `json:"ingest"`
	Links    []Link           `json:"links"`
	Log      []map[string]any `json:"log"`
}

type capped struct {
	bytes.Buffer
}

func (c *capped) Write(p []byte) (int, error) {
	if room := maxOutput - c.Len(); room > 0 {
		if len(p) > room {
			c.Buffer.Write(p[:room])
		} else {
			c.Buffer.Write(p)
		}
	}
	return len(p), nil
}

func runOne(res *Result, h config.Hook, label, repoRoot, event string, payload Payload, stdin []byte) {
	timeout := Timeout(h)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := shell(ctx, h.Run)
	cmd.Dir = repoRoot
	cmd.Env = env(repoRoot, event, payload)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr capped
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	setGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = time.Second
	err := cmd.Run()

	fail := func(msg string) {
		res.Errors = append(res.Errors, label+": "+redact.String(msg))
		if h.Blocking {
			res.Reasons = append(res.Reasons, label+": "+redact.String(msg)+" — gate stays closed until a human runs: huddle gate approve --override --reason")
			escalate(res, Ask)
		}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		fail(fmt.Sprintf("timed out after %s", timeout))
		return
	}
	if err != nil {
		msg := err.Error()
		if tail := lastLine(stderr.String()); tail != "" {
			msg += ": " + tail
		}
		fail(msg)
		return
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return
	}
	var ho hookOutput
	if json.Unmarshal([]byte(out), &ho) != nil {
		if h.Blocking {
			fail("blocking hook printed invalid JSON")
			return
		}
		res.Context = append(res.Context, redact.String(out))
		return
	}
	decision := strings.ToLower(strings.TrimSpace(ho.Decision))
	switch decision {
	case "", Allow:
	case Deny, Ask:
		if h.Blocking {
			escalate(res, decision)
			reason := ho.Reason
			if reason == "" {
				reason = "hook returned " + decision
			}
			res.Reasons = append(res.Reasons, label+": "+redact.String(reason))
		} else if ho.Reason != "" {
			res.Context = append(res.Context, redact.String(ho.Reason))
		}
	default:
		if h.Blocking {
			fail(fmt.Sprintf("unknown decision %q", decision))
			return
		}
		res.Errors = append(res.Errors, label+": "+redact.String(fmt.Sprintf("unknown decision %q ignored", decision)))
	}
	if decision == "" || decision == Allow {
		if ho.Reason != "" {
			res.Context = append(res.Context, redact.String(ho.Reason))
		}
	}
	res.Context = append(res.Context, contextStrings(ho.Context)...)
	for _, l := range ho.Links {
		res.Links = append(res.Links, Link{System: redact.String(l.System), ID: redact.String(l.ID), URL: redact.String(l.URL), Artifact: redact.String(l.Artifact)})
	}
	for _, m := range ho.Ingest {
		res.Ingest = append(res.Ingest, redact.Any(m).(map[string]any))
	}
	for _, m := range ho.Log {
		res.Log = append(res.Log, redact.Any(m).(map[string]any))
	}
}

func contextStrings(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s = strings.TrimSpace(s); s != "" {
			return []string{redact.String(s)}
		}
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		var out []string
		for _, x := range list {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, redact.String(x))
			}
		}
		return out
	}
	return []string{redact.String(string(raw))}
}

// escalate raises the decision: deny beats ask beats allow.
func escalate(res *Result, d string) {
	rank := map[string]int{Allow: 0, Ask: 1, Deny: 2}
	if rank[d] > rank[res.Decision] {
		res.Decision = d
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// startDetached launches an async hook in its own session and returns without
// waiting. Stdin comes from a temp file so the child can read it after we exit.
func startDetached(h config.Hook, repoRoot, event string, payload Payload, stdin []byte) error {
	f, err := os.CreateTemp("", "huddle-hook-*.json")
	if err != nil {
		return err
	}
	name := f.Name()
	if _, err := f.Write(stdin); err != nil {
		f.Close()
		os.Remove(name)
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		f.Close()
		os.Remove(name)
		return err
	}
	cmd := shell(context.Background(), h.Run)
	cmd.Dir = repoRoot
	cmd.Env = env(repoRoot, event, payload)
	cmd.Stdin = f
	detach(cmd)
	err = cmd.Start()
	f.Close()
	if runtime.GOOS != "windows" || err != nil {
		os.Remove(name) // the child holds its own fd on unix
	}
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}
