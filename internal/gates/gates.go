// Package gates holds human approval gates with separation of duties (ADR 16,
// 18, 19 row 7) and verdicts stamped with a tree fingerprint (row 10). Gate
// state is derived from the event log; artifact status is written to, and
// always read back from, the artifact file.
package gates

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/hooks"
	"github.com/muthuishere/huddle-next/internal/redact"
)

// Event types written by this package.
const (
	EvRequest  = "gate.request"
	EvApproved = "gate.approved"
	EvRejected = "gate.rejected"
	EvOverride = "gate.override"
)

// Gate states.
const (
	StateRequested = "requested"
	StateApproved  = "approved"
	StateRejected  = "rejected"
)

// Gate is the derived state of one approval request. The JSON contract read by
// the exported production-gate hook is: id, name, artifact, role, state
// (requested|approved|rejected), stale, approvedBy, ts.
type Gate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"` // deploy | production | release | free text; default: the artifact kind
	ArtifactID  string    `json:"artifact"`
	Role        string    `json:"role"`
	State       string    `json:"state"`
	Stale       bool      `json:"stale"`
	ApprovedBy  string    `json:"approvedBy"`
	TS          time.Time `json:"ts"` // last state change
	Kind        string    `json:"kind"`
	Path        string    `json:"path"`
	RequestedBy string    `json:"requestedBy,omitempty"`
	RequestedAt time.Time `json:"requestedAt"`
	DecidedBy   string    `json:"decidedBy,omitempty"`
	DecidedAt   time.Time `json:"decidedAt,omitzero"`
	Reason      string    `json:"reason,omitempty"`
	Override    bool      `json:"override,omitempty"`
	SHA256      string    `json:"sha256,omitempty"` // artifact hash at approval
	// Filled by Annotate from the current file.
	FileStatus string `json:"fileStatus,omitempty"`
	Missing    bool   `json:"missing,omitempty"`
}

// Load derives every gate from the log, oldest request first.
func Load(repoName string) ([]Gate, error) {
	evs, err := eventlog.List(repoName, eventlog.Filter{Types: map[string]bool{EvRequest: true, EvApproved: true, EvRejected: true}})
	if err != nil {
		return nil, err
	}
	byID := map[string]*Gate{}
	var order []string
	for _, e := range evs {
		gid, _ := e.Data["gate"].(string)
		if gid == "" {
			continue
		}
		switch e.Type {
		case EvRequest:
			if _, dup := byID[gid]; dup {
				continue
			}
			g := &Gate{ID: gid, State: StateRequested, RequestedBy: e.By, RequestedAt: e.TS, TS: e.TS}
			g.ArtifactID, _ = e.Data["artifactId"].(string)
			g.Role, _ = e.Data["role"].(string)
			g.Name, _ = e.Data["name"].(string)
			if e.Artifact != nil {
				g.Kind, g.Path = e.Artifact.Kind, e.Artifact.Path
			}
			if g.Name == "" {
				g.Name = g.Kind
			}
			byID[gid] = g
			order = append(order, gid)
		case EvApproved, EvRejected:
			g := byID[gid]
			if g == nil || g.State != StateRequested {
				continue
			}
			g.State = StateApproved
			if e.Type == EvRejected {
				g.State = StateRejected
			} else {
				g.ApprovedBy = e.By
			}
			g.DecidedBy, g.DecidedAt, g.TS = e.By, e.TS, e.TS
			g.Reason, _ = e.Data["reason"].(string)
			g.Override, _ = e.Data["override"].(bool)
			if e.Artifact != nil {
				g.SHA256 = e.Artifact.SHA256
			}
		}
	}
	out := make([]Gate, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// Annotate fills FileStatus/Stale/Missing from the current artifact files.
// An approved gate is STALE when the file hash differs from the approved hash.
func Annotate(gs []Gate, current map[string]artifacts.Artifact) {
	for i := range gs {
		g := &gs[i]
		a, ok := current[g.Path]
		if !ok {
			g.Missing = true
			g.Stale = g.State == StateApproved
			continue
		}
		g.FileStatus = a.Status
		g.Stale = g.State == StateApproved && g.SHA256 != "" && g.SHA256 != a.SHA256
	}
}

func newGateID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return "g-" + hex.EncodeToString(b[:])
}

// AgentEnvVars are the environment variables that mark an agent session
// (Huddle, Claude Code, Codex, Devin, OpenCode). Any one non-empty is enough.
var AgentEnvVars = []string{
	"HUDDLE_AGENT", "CLAUDECODE", "CLAUDE_CODE_SESSION_ID",
	"CODEX_THREAD_ID", "CODEX_SANDBOX", "DEVIN_SESSION_ID", "OPENCODE_SESSION_ID",
}

// AgentSession returns the first agent marker set in the environment, or "".
func AgentSession(getenv func(string) string) string {
	for _, name := range AgentEnvVars {
		if strings.TrimSpace(getenv(name)) != "" {
			return name
		}
	}
	return ""
}

// Identity describes who is running the command.
type Identity struct {
	Agent       string // agent marker env var found by AgentSession, "" for a human
	Interactive bool   // stdin and stdout are terminals
	As          string // --as <person>
	User        string // configured user for an interactive human
}

// ErrAgent is returned when an agent session tries to decide a gate.
var ErrAgent = errors.New("an agent session cannot approve or reject gates")

// Decider resolves the human deciding a gate. An agent session is always
// refused, even with --as; a non-interactive call needs --as.
func Decider(id Identity) (string, error) {
	if id.Agent != "" {
		return "", fmt.Errorf("%w (%s is set): gates are approved by a human in a terminal: run `huddle gate approve <id>` yourself", ErrAgent, id.Agent)
	}
	if as := strings.TrimSpace(id.As); as != "" {
		return as, nil
	}
	if !id.Interactive {
		return "", fmt.Errorf("%w without a terminal — a human owner must run it interactively, or pass --as <owner>", ErrAgent)
	}
	if u := strings.TrimSpace(id.User); u != "" && u != "unknown" {
		return u, nil
	}
	return "", errors.New("cannot tell who you are — set: huddle config set user <name>, or pass --as <owner>")
}

func samePerson(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// IsOwner reports whether person is listed under owners[role].
func IsOwner(cfg config.Config, role, person string) bool {
	for _, p := range cfg.Owners[role] {
		if samePerson(p, person) {
			return true
		}
	}
	return false
}

// Deps are the collaborators a gate decision needs.
type Deps struct {
	Root string
	Env  artifacts.Env
}

// Request opens a gate for an artifact. name labels the gate (deploy,
// production, release, …); empty means the artifact kind.
func Request(d Deps, ref, role, name, by string) (Gate, error) {
	a, err := artifacts.Resolve(d.Root, d.Env.Cfg, "", ref)
	if err != nil {
		return Gate{}, err
	}
	k, _ := artifacts.Get(d.Env.Cfg, a.Kind)
	if k.Format != artifacts.FormatMD {
		return Gate{}, fmt.Errorf("%s is a %s file with no author/status front matter — gates apply to markdown artifacts", a.Path, k.Format)
	}
	if role == "" {
		role = k.GateRole
	}
	if a.Kind == "skill" && role != "policy" {
		return Gate{}, fmt.Errorf("skill changes are approved by the policy owner — run: huddle gate request %s --role policy", a.ID)
	}
	if len(d.Env.Cfg.Owners[role]) == 0 {
		return Gate{}, fmt.Errorf("no owners for role %q — run: huddle config set owners.%s '[\"<name>\"]'", role, role)
	}
	existing, err := Load(d.Env.Repo.Name)
	if err != nil {
		return Gate{}, err
	}
	for _, g := range existing {
		if g.State == StateRequested && g.Path == a.Path {
			return g, fmt.Errorf("gate %s is already pending for %s — run: huddle gate status", g.ID, a.ID)
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = a.Kind
	}
	g := Gate{ID: newGateID(), Name: name, ArtifactID: a.ID, Kind: a.Kind, Path: a.Path, Role: role, RequestedBy: by, State: StateRequested}
	ev, err := eventlog.Append(d.Env.Repo.Name, eventlog.Event{
		Type: EvRequest, By: by, Branch: d.Env.Repo.Branch, Refs: []string{a.ID},
		Artifact: &eventlog.ArtifactRef{Kind: a.Kind, Path: a.Path, SHA256: a.SHA256},
		Data:     map[string]any{"gate": g.ID, "name": name, "artifactId": a.ID, "role": role, "author": a.Author},
	})
	g.RequestedAt, g.TS = ev.TS, ev.TS
	return g, err
}

// DecideOptions are the flags of gate approve|reject.
type DecideOptions struct {
	GateID   string
	Approve  bool
	Reason   string
	Override bool
	Identity Identity
}

// ErrHookBlocked is returned when a blocking hook denies or asks.
var ErrHookBlocked = errors.New("gate blocked by hook")

// Decide approves or rejects a pending gate, enforcing separation of duties:
// the decider is a human, listed under owners[role], and (to approve) not the
// artifact's author. A blocking gate.request hook that denies or asks keeps
// the gate closed unless an owner passes --override --reason (logged).
func Decide(d Deps, o DecideOptions) (Gate, error) {
	gs, err := Load(d.Env.Repo.Name)
	if err != nil {
		return Gate{}, err
	}
	var g *Gate
	for i := range gs {
		if gs[i].ID == o.GateID {
			g = &gs[i]
		}
	}
	if g == nil {
		return Gate{}, fmt.Errorf("no gate %q — run: huddle gate status", o.GateID)
	}
	if g.State != StateRequested {
		return *g, fmt.Errorf("gate %s is already %s by %s", g.ID, g.State, g.DecidedBy)
	}
	who, err := Decider(o.Identity)
	if err != nil {
		return *g, err
	}
	if !IsOwner(d.Env.Cfg, g.Role, who) {
		return *g, fmt.Errorf("%s is not an owner for role %q (owners: %s) — ask one of them to decide", who, g.Role, strings.Join(d.Env.Cfg.Owners[g.Role], ", "))
	}
	k, ok := artifacts.Get(d.Env.Cfg, g.Kind)
	if !ok {
		return *g, fmt.Errorf("unknown artifact kind %q", g.Kind)
	}
	a, err := artifacts.Load(d.Root, k, g.Path)
	if err != nil {
		return *g, fmt.Errorf("artifact %s is gone (%v) — reject the gate or restore the file", g.Path, err)
	}
	reason := redact.String(strings.TrimSpace(o.Reason))
	if o.Override && reason == "" {
		return *g, errors.New("--override needs --reason \"why the hook result is wrong\"")
	}

	if !o.Approve {
		if reason == "" {
			return *g, errors.New("rejecting needs --reason")
		}
		ev, err := eventlog.Append(d.Env.Repo.Name, eventlog.Event{
			Type: EvRejected, By: who, Branch: d.Env.Repo.Branch, Text: reason, Refs: []string{g.ArtifactID},
			Artifact: &eventlog.ArtifactRef{Kind: a.Kind, Path: a.Path, SHA256: a.SHA256},
			Data:     map[string]any{"gate": g.ID, "reason": reason},
		})
		fireAfter(d, EvRejected, *g, a, who)
		g.State, g.DecidedBy, g.Reason, g.DecidedAt, g.TS = StateRejected, who, reason, ev.TS, ev.TS
		return *g, err
	}

	if a.Author != "" && samePerson(a.Author, who) {
		return *g, fmt.Errorf("%s wrote %s and cannot approve it — separation of duties: another %s owner must approve", who, a.ID, g.Role)
	}

	res := hooks.Result{Decision: "allow"}
	if d.Env.Fire != nil {
		res = d.Env.Fire(d.Env.Cfg, d.Root, EvRequest, payload(d, EvRequest, *g, a, who))
	}
	decision := strings.ToLower(strings.TrimSpace(res.Decision))
	if decision == "" {
		decision = "allow"
	}
	overridden := false
	if decision != "allow" {
		reasons := strings.Join(res.Reasons, "; ")
		if !o.Override {
			return *g, fmt.Errorf("%w: hook decision %q (%s) — gate %s stays closed; an owner can run: huddle gate approve %s --override --reason \"…\"", ErrHookBlocked, decision, reasons, g.ID, g.ID)
		}
		overridden = true
		if _, err := eventlog.Append(d.Env.Repo.Name, eventlog.Event{
			Type: EvOverride, By: who, Branch: d.Env.Repo.Branch, Text: reason, Refs: []string{g.ArtifactID},
			Artifact: &eventlog.ArtifactRef{Kind: a.Kind, Path: a.Path, SHA256: a.SHA256},
			Data:     map[string]any{"gate": g.ID, "hookDecision": decision, "hookReasons": redact.String(reasons), "reason": reason},
		}); err != nil {
			return *g, err
		}
	}

	updated, err := artifacts.UpdateFrontMatter(d.Root, d.Env.Cfg, a, func(fm *artifacts.FrontMatter) {
		fm.Set("status", "approved")
		fm.Set("approvedBy", who)
	})
	if err != nil {
		return *g, err
	}
	if _, _, err := artifacts.RecordWrite(d.Env, updated.ID, updated.Kind, updated.Path, updated.SHA256, who, "gate.approve"); err != nil {
		return *g, err
	}
	ev, err := eventlog.Append(d.Env.Repo.Name, eventlog.Event{
		Type: EvApproved, By: who, Branch: d.Env.Repo.Branch, Text: reason, Refs: []string{g.ArtifactID},
		Artifact: &eventlog.ArtifactRef{Kind: updated.Kind, Path: updated.Path, SHA256: updated.SHA256},
		Data:     map[string]any{"gate": g.ID, "reason": reason, "override": overridden, "hookDecision": decision},
	})
	fireAfter(d, EvApproved, *g, updated, who)
	g.State, g.DecidedBy, g.ApprovedBy, g.Reason, g.Override, g.SHA256 = StateApproved, who, who, reason, overridden, updated.SHA256
	g.FileStatus, g.DecidedAt, g.TS = updated.Status, ev.TS, ev.TS
	return *g, err
}

func payload(d Deps, event string, g Gate, a artifacts.Artifact, who string) hooks.Payload {
	k, _ := artifacts.Get(d.Env.Cfg, a.Kind)
	return hooks.Payload{
		"event": event, "repo": d.Env.Repo.Name, "branch": d.Env.Repo.Branch,
		"gate": g.ID, "name": g.Name, "role": g.Role, "stage": k.Stage, "kind": a.Kind, "id": a.ID, "by": who,
		"artifact": map[string]any{"kind": a.Kind, "path": a.Path, "sha256": a.SHA256},
	}
}

func fireAfter(d Deps, event string, g Gate, a artifacts.Artifact, who string) {
	if d.Env.Fire != nil {
		d.Env.Fire(d.Env.Cfg, d.Root, event, payload(d, event, g, a, who))
	}
}

// Sorted returns requested gates first, then newest.
func Sorted(gs []Gate) []Gate {
	out := append([]Gate(nil), gs...)
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].State == StateRequested) != (out[j].State == StateRequested) {
			return out[i].State == StateRequested
		}
		return out[i].RequestedAt.After(out[j].RequestedAt)
	})
	return out
}
