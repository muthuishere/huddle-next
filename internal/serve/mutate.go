// The dashboard's mutations (ADR 20 §3): create an artifact from a template,
// lint, append to the log, and test a hook. Every one is local, auditable, and
// recorded with by:"serve". Gates are deliberately absent — a localhost request
// cannot prove a human, so they stay in the terminal (ADR 19, ADR 20 §3).
package serve

import (
	"errors"
	"net/http"
	"strings"

	"github.com/muthuishere/huddle/internal/artifacts"
	"github.com/muthuishere/huddle/internal/eventlog"
	"github.com/muthuishere/huddle/internal/hooks"
	"github.com/muthuishere/huddle/internal/redact"
)

// env builds the audit bundle artifact writes need.
func (s *Server) env(t target) (artifacts.Env, error) {
	cfg, err := t.config()
	if err != nil {
		return artifacts.Env{}, err
	}
	return artifacts.Env{Repo: t.info, Cfg: cfg, Fire: s.fire}, nil
}

type artifactNewReq struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Dry   bool   `json:"dryRun"`
}

// postArtifactNew renders a template and writes it, mirroring `huddle <noun> new`.
func (s *Server) postArtifactNew(t target, r *http.Request, body []byte) (any, error) {
	var req artifactNewReq
	if err := decodeBody(body, &req); err != nil {
		return nil, err
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Kind == "" {
		return nil, errStatus(http.StatusBadRequest, "kind is required — kinds: "+strings.Join(artifacts.Names(), ", "))
	}
	env, err := s.env(t)
	if err != nil {
		return nil, err
	}
	if _, ok := artifacts.Get(env.Cfg, req.Kind); !ok {
		return nil, errStatus(http.StatusBadRequest, "unknown kind "+req.Kind+" — kinds: "+strings.Join(artifacts.Names(), ", "))
	}
	author := artifacts.CurrentUser(env.Cfg)
	d, err := artifacts.Render(t.info.Root, env.Cfg, req.Kind, artifacts.NewOptions{
		Author: author,
		Title:  redact.String(strings.TrimSpace(req.Title)),
		Now:    s.now(),
	})
	if err != nil {
		return nil, errStatus(http.StatusBadRequest, err.Error())
	}
	if req.Dry {
		return map[string]any{"dryRun": true, "kind": d.Kind, "id": d.ID, "path": d.Path, "sha256": d.SHA256, "content": d.Content}, nil
	}
	if err := artifacts.Create(t.info.Root, d); err != nil {
		if errors.Is(err, artifacts.ErrExists) {
			return nil, errStatus(http.StatusConflict, "artifact already exists at "+d.Path)
		}
		return nil, err
	}
	ev, res, err := artifacts.RecordWrite(env, d.ID, d.Kind, d.Path, d.SHA256, By, "new")
	if err != nil {
		return nil, err
	}
	return map[string]any{"kind": d.Kind, "id": d.ID, "path": d.Path, "sha256": d.SHA256, "event": ev.ID, "hooks": res}, nil
}

type lintReq struct {
	Kind string `json:"kind"` // empty = every kind
}

// postLint runs the same lint the CLI runs and returns every finding.
func (s *Server) postLint(t target, r *http.Request, body []byte) (any, error) {
	var req lintReq
	if err := decodeBody(body, &req); err != nil {
		return nil, err
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	var names []string
	if k := strings.TrimSpace(req.Kind); k != "" {
		if _, ok := artifacts.Get(cfg, k); !ok {
			return nil, errStatus(http.StatusBadRequest, "unknown kind "+k+" — kinds: "+strings.Join(artifacts.Names(), ", "))
		}
		names = []string{k}
	}
	all, err := artifacts.Discover(t.info.Root, cfg, names...)
	if err != nil {
		return nil, err
	}
	findings := []artifacts.Finding{}
	for _, a := range all {
		k, _ := artifacts.Get(cfg, a.Kind)
		findings = append(findings, artifacts.Lint(a, k)...)
	}
	return map[string]any{"checked": len(all), "findings": findings, "ok": len(findings) == 0}, nil
}

type logReq struct {
	Type string   `json:"type"`
	Text string   `json:"text"`
	Door string   `json:"door"`
	Refs []string `json:"refs"`
}

// logTypes are the event types the dashboard may append (ADR 20 §3:
// a decision, a learning, or a question — never a gate decision).
var logTypes = map[string]bool{"decision": true, "learning": true, "question": true}

// postLog appends one event, attributed to the dashboard.
func (s *Server) postLog(t target, r *http.Request, body []byte) (any, error) {
	var req logReq
	if err := decodeBody(body, &req); err != nil {
		return nil, err
	}
	typ := strings.TrimSpace(strings.ToLower(req.Type))
	if !logTypes[typ] {
		return nil, errStatus(http.StatusBadRequest, "type must be decision, learning or question — gates are decided in a terminal")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, errStatus(http.StatusBadRequest, "text is required")
	}
	door := strings.TrimSpace(strings.ToLower(req.Door))
	if typ == "decision" && door != "one-way" && door != "two-way" {
		return nil, errStatus(http.StatusBadRequest, `a decision needs door "one-way" or "two-way"`)
	}
	if typ != "decision" {
		door = ""
	}
	ev, err := eventlog.Append(t.info.Name, eventlog.Event{
		Type:   typ,
		By:     By,
		Branch: t.info.Branch,
		Text:   redact.String(text),
		Door:   door,
		Refs:   req.Refs,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"event": ev}, nil
}

type hooksTestReq struct {
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

// postHooksTest fires the configured hooks for an event with a sample payload,
// exactly as `huddle hooks test` does. Its decision is reported, never enforced.
func (s *Server) postHooksTest(t target, r *http.Request, body []byte) (any, error) {
	var req hooksTestReq
	if err := decodeBody(body, &req); err != nil {
		return nil, err
	}
	event := strings.TrimSpace(req.Event)
	if event == "" {
		return nil, errStatus(http.StatusBadRequest, "event is required, e.g. gate.request")
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	payload := hooks.Payload(req.Payload)
	if payload == nil {
		payload = hooks.Payload{}
	}
	res := s.fire(cfg, t.info.Root, event, payload)
	return map[string]any{"event": event, "result": res}, nil
}
