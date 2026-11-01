package serve

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/hooks"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/sdlc"
	"github.com/muthuishere/huddle-next/personas"
)

// Warning is one overview callout.
type Warning struct {
	Level   string `json:"level"` // warn | info
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

func (s *Server) apiOverview(t target, r *http.Request) (any, error) {
	if err := t.needRoot(); err != nil {
		return nil, err
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	rep, err := sdlc.Status(t.info, cfg)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"repo":     t.info.Name,
		"branch":   t.info.Branch,
		"root":     t.info.Root,
		"isGit":    t.info.IsGit,
		"current":  t.info.Name == s.repo.Name,
		"report":   rep,
		"next":     rep.Next,
		"warnings": warnings(rep),
	}, nil
}

func warnings(rep sdlc.Report) []Warning {
	out := []Warning{}
	if !rep.Guardrails.Exists {
		out = append(out, Warning{"warn", "No CLAUDE.md guardrails — agents work without repo rules.", "huddle guardrails new"})
	}
	if !rep.AgentsMD.Exists {
		out = append(out, Warning{"info", "No AGENTS.md mirror for non-Claude harnesses.", ""})
	}
	if !rep.ReviewPolicy.Exists {
		out = append(out, Warning{"warn", "No REVIEW.md review policy.", "huddle review-policy new"})
	}
	if rep.Evals == 0 {
		out = append(out, Warning{"warn", "No evals — verdicts cannot cite eval results.", "huddle eval new <slug>"})
	}
	if !rep.Bands.Exists {
		out = append(out, Warning{"warn", "No bands.yaml control bands for production metrics.", "huddle bands new <metric>"})
	}
	for _, c := range rep.Changes {
		if c.Verdict != nil && c.Verdict.Stale {
			out = append(out, Warning{"warn", "Stale verdict for " + c.Verdict.ArtifactID + " — the tree changed after it was recorded.",
				"huddle verify record " + c.Verdict.ArtifactID + " --verdict PASS|CONCERNS|FAIL"})
		}
	}
	return out
}

// ArtifactRow is one artifact in the list view.
type ArtifactRow struct {
	artifacts.Artifact
	Format   string              `json:"format"`
	Stage    string              `json:"stage"`
	Lint     int                 `json:"lint"`
	Findings []artifacts.Finding `json:"findings"`
}

func (s *Server) apiArtifacts(t target, r *http.Request) (any, error) {
	if err := t.needRoot(); err != nil {
		return nil, err
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	var names []string
	if k := r.URL.Query().Get("kind"); k != "" {
		if _, ok := artifacts.Get(cfg, k); !ok {
			return nil, errStatus(http.StatusBadRequest, "unknown kind "+k+" — kinds: "+strings.Join(artifacts.Names(), ", "))
		}
		names = []string{k}
	}
	all, err := artifacts.Discover(t.info.Root, cfg, names...)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(r.URL.Query().Get("status"))
	rows := []ArtifactRow{}
	for _, a := range all {
		if want != "" && !strings.EqualFold(a.Status, want) {
			continue
		}
		k, _ := artifacts.Get(cfg, a.Kind)
		f := artifacts.Lint(a, k)
		if f == nil {
			f = []artifacts.Finding{}
		}
		rows = append(rows, ArtifactRow{Artifact: a, Format: k.Format, Stage: k.Stage, Lint: len(f), Findings: f})
	}
	return map[string]any{"kinds": artifacts.Names(), "statuses": artifacts.Statuses, "artifacts": rows}, nil
}

func (s *Server) apiArtifact(t target, r *http.Request) (any, error) {
	if err := t.needRoot(); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		return nil, errStatus(http.StatusBadRequest, "id is required")
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	a, err := artifacts.Resolve(t.info.Root, cfg, r.URL.Query().Get("kind"), id)
	if err != nil {
		return nil, errStatus(http.StatusNotFound, err.Error())
	}
	k, _ := artifacts.Get(cfg, a.Kind)
	f := artifacts.Lint(a, k)
	if f == nil {
		f = []artifacts.Finding{}
	}
	out := map[string]any{"artifact": a, "format": k.Format, "lint": f, "raw": string(a.Raw)}
	if k.Format == artifacts.FormatMD {
		fm, body := artifacts.SplitFrontMatter(a.Raw)
		fields := map[string]any{}
		fmText := ""
		if fm.Present {
			fmText = string(fm.Render())
			inner := strings.TrimSuffix(strings.TrimPrefix(fmText, "---\n"), "---\n")
			if m, err := artifacts.ParseYAML([]byte(inner)); err == nil {
				fields = m
			}
		}
		out["frontMatter"] = fields
		out["frontMatterRaw"] = fmText
		out["markdown"] = string(body)
		out["html"] = RenderMarkdown(string(body))
	}
	return out, nil
}

// GateView is a gate plus the terminal commands a human runs to decide it.
type GateView struct {
	gates.Gate
	Approve string `json:"approveCommand"`
	Reject  string `json:"rejectCommand"`
}

// GateWhy explains why the dashboard never approves gates.
const GateWhy = "Gates are decided in a terminal, not here: any process on this machine can reach a localhost endpoint, so a browser click cannot prove a human owner made the call (ADR 19, ADR 20 §3)."

func (s *Server) apiGates(t target, r *http.Request) (any, error) {
	all, err := gates.Load(t.info.Name)
	if err != nil {
		return nil, err
	}
	if t.known {
		cfg, err := t.config()
		if err != nil {
			return nil, err
		}
		byPath := map[string]artifacts.Artifact{}
		for _, g := range all {
			if _, seen := byPath[g.Path]; seen {
				continue
			}
			if k, ok := artifacts.Get(cfg, g.Kind); ok {
				if x, err := artifacts.Load(t.info.Root, k, g.Path); err == nil {
					byPath[g.Path] = x
				}
			}
		}
		gates.Annotate(all, byPath)
	}
	views := []GateView{}
	counts := map[string]int{}
	for _, g := range gates.Sorted(all) {
		counts[g.State]++
		if g.Stale {
			counts["stale"]++
		}
		if g.Override {
			counts["override"]++
		}
		views = append(views, GateView{Gate: g,
			Approve: "huddle gate approve " + g.ID + ` --reason "<why>"`,
			Reject:  "huddle gate reject " + g.ID + ` --reason "<why>"`})
	}
	return map[string]any{"gates": views, "counts": counts, "why": GateWhy}, nil
}

func (s *Server) apiPersonas(t target, r *http.Request) (any, error) {
	roster, err := personas.ResolveDefault(t.info.Root)
	if err != nil {
		return nil, err
	}
	return map[string]any{"personas": roster.Personas, "overflow": roster.Overflow, "tasks": personas.TaskKeys()}, nil
}

func (s *Server) apiPersonaStyle(t target, r *http.Request) (any, error) {
	id := r.URL.Query().Get("id")
	roster, err := personas.ResolveDefault(t.info.Root)
	if err != nil {
		return nil, err
	}
	p, ok := roster.Get(id)
	if !ok {
		return nil, errStatus(http.StatusNotFound, "no persona "+id)
	}
	card, err := personas.Style(p, r.URL.Query().Get("for"))
	if err != nil {
		return nil, errStatus(http.StatusBadRequest, err.Error())
	}
	return map[string]any{"card": card, "markdown": card.Markdown()}, nil
}

// HookView is a configured hook with its effective timeout.
type HookView struct {
	config.Hook
	Scope   string `json:"scope"`
	Timeout string `json:"timeout"`
}

func (s *Server) apiHooks(t target, r *http.Request) (any, error) {
	if err := t.needRoot(); err != nil {
		return nil, err
	}
	cfg, err := t.config()
	if err != nil {
		return nil, err
	}
	byScope := map[string][]HookView{"global": {}, "repo": {}}
	for _, h := range cfg.Hooks {
		byScope[h.Scope] = append(byScope[h.Scope], HookView{Hook: h, Scope: h.Scope, Timeout: hooks.Timeout(h).String()})
	}
	changed, count, err := repoHooksChanged(t.info.Root, t.info.Name)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"byScope":          byScope,
		"repoHooksChanged": changed,
		"repoHooks":        count,
		"reviewCommand":    "huddle hooks list",
		"globalConfig":     paths.GlobalConfig(),
		"repoConfig":       paths.RepoConfig(t.info.Root),
	}, nil
}

// repoHooksChanged mirrors `huddle start`'s REPO_HOOKS_CHANGED: the sha256 of
// the canonical repo hooks array against the hash `huddle hooks list` recorded.
func repoHooksChanged(root, name string) (bool, int, error) {
	var hs []any
	b, err := os.ReadFile(paths.RepoConfig(root))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, 0, err
	}
	if len(bytes.TrimSpace(b)) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return false, 0, errStatus(http.StatusUnprocessableEntity, paths.RepoConfig(root)+" is not valid JSON")
		}
		if raw, ok := m["hooks"]; ok {
			_ = json.Unmarshal(raw, &hs)
		}
	}
	if hs == nil {
		hs = []any{}
	}
	canon, _ := json.Marshal(hs)
	sum := sha256.Sum256(canon)
	h := hex.EncodeToString(sum[:])
	var seen struct {
		SHA256 string `json:"sha256"`
	}
	sb, err := os.ReadFile(paths.HooksSeen(name))
	if err == nil && json.Unmarshal(sb, &seen) == nil && seen.SHA256 != "" {
		return seen.SHA256 != h, len(hs), nil
	}
	return len(hs) > 0, len(hs), nil
}
