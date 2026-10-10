// Package sdlc reports where each change is in the AI-native SDLC and the
// single next action (ADR 16 §4–5). It reads files and the event log only —
// no git or network exec — so `huddle` with no args stays fast.
package sdlc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/repo"
)

// Stage is one artifact of a change.
type Stage struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Status string `json:"status,omitempty"`
}

// VerdictRef is the latest verdict for a change.
type VerdictRef struct {
	ArtifactID string `json:"artifactId"`
	Verdict    string `json:"verdict"`
	Stale      bool   `json:"stale"`
	EvalStatus string `json:"evalStatus,omitempty"`
}

// Change is one intent slug and its chain.
type Change struct {
	Slug         string       `json:"slug"`
	Intent       Stage        `json:"intent"`
	Spec         Stage        `json:"spec"`
	Plan         Stage        `json:"plan"`
	PendingGates []gates.Gate `json:"pendingGates,omitempty"`
	Verdict      *VerdictRef  `json:"verdict,omitempty"`
}

// Presence of a repo-level file.
type Presence struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Status string `json:"status,omitempty"`
	Block  bool   `json:"huddleBlock,omitempty"`
}

// Report is the whole status.
type Report struct {
	Repo         string       `json:"repo"`
	Branch       string       `json:"branch"`
	Changes      []Change     `json:"changes"`
	PendingGates []gates.Gate `json:"pendingGates"`
	OpenSpikes   []string     `json:"openSpikes"`
	Guardrails   Presence     `json:"guardrails"`
	AgentsMD     Presence     `json:"agentsMd"`
	ReviewPolicy Presence     `json:"reviewPolicy"`
	Bands        Presence     `json:"bands"`
	Evals        int          `json:"evals"`
	Next         Step         `json:"next"`
}

// Step is the single next action.
type Step struct {
	Stage   string `json:"stage"`
	Reason  string `json:"reason"`
	Command string `json:"command"`
}

// Status builds the report.
func Status(info repo.Info, cfg config.Config) (Report, error) {
	root := info.Root
	r := Report{Repo: info.Name, Branch: info.Branch, Changes: []Change{}, PendingGates: []gates.Gate{}, OpenSpikes: []string{}}

	byPath := map[string]artifacts.Artifact{}
	chains := map[string]*Change{}
	for _, kind := range []string{"intent", "spec", "plan"} {
		list, err := artifacts.Discover(root, cfg, kind)
		if err != nil {
			return r, err
		}
		for _, a := range list {
			byPath[a.Path] = a
			slug := a.Slug
			if slug == "" {
				slug = a.ID
			}
			c := chains[slug]
			if c == nil {
				c = &Change{Slug: slug}
				chains[slug] = c
			}
			st := Stage{Kind: kind, ID: a.ID, Path: a.Path, Exists: true, Status: orDash(a.Status)}
			switch kind {
			case "intent":
				c.Intent = st
			case "spec":
				c.Spec = st
			case "plan":
				c.Plan = st
			}
		}
	}
	slugs := make([]string, 0, len(chains))
	for s := range chains {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		c := chains[s]
		for _, st := range []*Stage{&c.Intent, &c.Spec, &c.Plan} {
			if !st.Exists {
				kind := [...]string{"intent", "spec", "plan"}[stageIndex(c, st)]
				k, _ := artifacts.Get(cfg, kind)
				st.Kind = kind
				st.Path = strings.ReplaceAll(k.Pattern, "{slug}", s)
			}
		}
	}

	spikes, err := artifacts.Discover(root, cfg, "spike")
	if err != nil {
		return r, err
	}
	for _, sp := range spikes {
		byPath[sp.Path] = sp
		if sec, ok := artifacts.FindSection(artifacts.Sections(string(sp.Body)), "Status"); ok {
			word := strings.ToLower(strings.Trim(strings.TrimSpace(sec.Content), "_*`"))
			if strings.HasPrefix(word, "open") || strings.HasPrefix(word, "running") {
				r.OpenSpikes = append(r.OpenSpikes, sp.ID)
			}
		}
	}

	r.Guardrails = presence(root, cfg, "guardrails", byPath)
	if r.Guardrails.Exists {
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(r.Guardrails.Path))); err == nil {
			r.Guardrails.Block = strings.Contains(string(b), artifacts.GuardrailsBegin)
		}
	}
	_, agentsErr := os.Stat(filepath.Join(root, "AGENTS.md"))
	r.AgentsMD = Presence{Path: "AGENTS.md", Exists: agentsErr == nil}
	r.ReviewPolicy = presence(root, cfg, "review-policy", byPath)
	r.Bands = presence(root, cfg, "bands", byPath)
	if evals, err := artifacts.Discover(root, cfg, "eval"); err == nil {
		r.Evals = len(evals)
	}

	all, err := gates.Load(info.Name)
	if err != nil {
		return r, err
	}
	gates.Annotate(all, byPath)
	for _, g := range all {
		if g.State != gates.StateRequested {
			continue
		}
		r.PendingGates = append(r.PendingGates, g)
		for _, s := range slugs {
			c := chains[s]
			if g.Path == c.Intent.Path || g.Path == c.Spec.Path || g.Path == c.Plan.Path {
				c.PendingGates = append(c.PendingGates, g)
			}
		}
	}

	verdicts, err := gates.Latest(info.Name)
	if err != nil {
		return r, err
	}
	head := ""
	if info.IsGit && len(verdicts) > 0 {
		head = gates.HeadCommit(root)
	}
	for _, s := range slugs {
		c := chains[s]
		var best *gates.Verdict
		for _, st := range []Stage{c.Plan, c.Spec, c.Intent} {
			if !st.Exists {
				continue
			}
			if v, ok := verdicts[st.ID]; ok && (best == nil || v.TS.After(best.TS)) {
				v := v
				best = &v
			}
		}
		if best != nil {
			c.Verdict = &VerdictRef{ArtifactID: best.ArtifactID, Verdict: best.Verdict, EvalStatus: best.EvalStatus,
				Stale: gates.CheapStale(*best, head, byPath)}
		}
		r.Changes = append(r.Changes, *c)
	}
	r.Next = Next(r, cfg)
	return r, nil
}

func stageIndex(c *Change, st *Stage) int {
	switch st {
	case &c.Intent:
		return 0
	case &c.Spec:
		return 1
	}
	return 2
}

func presence(root string, cfg config.Config, kind string, byPath map[string]artifacts.Artifact) Presence {
	k, _ := artifacts.Get(cfg, kind)
	p := Presence{Path: k.Pattern}
	if a, err := artifacts.Load(root, k, k.Pattern); err == nil {
		p.Exists, p.Status = true, orDash(a.Status)
		byPath[a.Path] = a
	}
	return p
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Next picks the lowest incomplete stage and the exact command to run.
func Next(r Report, cfg config.Config) Step {
	if !r.Guardrails.Exists {
		return Step{Stage: "setup", Reason: "no CLAUDE.md guardrails", Command: "huddle guardrails new"}
	}
	for _, c := range r.Changes {
		if c.Intent.Status == "superseded" {
			continue
		}
		if len(c.PendingGates) > 0 {
			g := c.PendingGates[0]
			return Step{Stage: stageOf(g.Kind), Reason: fmt.Sprintf("%s waits for a human %s owner who is not its author", g.ArtifactID, g.Role),
				Command: fmt.Sprintf("huddle gate approve %s", g.ID)}
		}
		chain := []struct {
			st   Stage
			role string
		}{{c.Intent, role(cfg, "intent")}, {c.Spec, role(cfg, "spec")}, {c.Plan, role(cfg, "plan")}}
		for _, link := range chain {
			st := link.st
			if !st.Exists {
				return Step{Stage: stageOf(st.Kind), Reason: fmt.Sprintf("%s has no %s", c.Slug, st.Kind),
					Command: fmt.Sprintf("huddle %s new %s", st.Kind, c.Slug)}
			}
			if st.Status != "approved" {
				return Step{Stage: stageOf(st.Kind), Reason: fmt.Sprintf("%s is %s, not approved", st.ID, st.Status),
					Command: fmt.Sprintf("huddle %s lint %s && huddle gate request %s --role %s", st.Kind, st.ID, st.ID, link.role)}
			}
		}
		if c.Verdict == nil {
			return Step{Stage: "test", Reason: fmt.Sprintf("%s has no verdict", c.Plan.ID),
				Command: fmt.Sprintf("huddle verify record %s --verdict PASS|CONCERNS|FAIL", c.Plan.ID)}
		}
		if c.Verdict.Stale {
			return Step{Stage: "test", Reason: fmt.Sprintf("verdict for %s is stale (tree changed)", c.Verdict.ArtifactID),
				Command: fmt.Sprintf("huddle verify record %s --verdict PASS|CONCERNS|FAIL", c.Verdict.ArtifactID)}
		}
		if c.Verdict.Verdict != gates.PASS {
			return Step{Stage: "test", Reason: fmt.Sprintf("latest verdict for %s is %s", c.Verdict.ArtifactID, c.Verdict.Verdict),
				Command: fmt.Sprintf("huddle verify status %s", c.Verdict.ArtifactID)}
		}
	}
	if len(r.Changes) == 0 {
		return Step{Stage: "plan", Reason: "no intents yet", Command: "huddle intent new \"<problem in one line>\""}
	}
	if !r.ReviewPolicy.Exists {
		return Step{Stage: "deploy", Reason: "no REVIEW.md review policy", Command: "huddle review-policy new"}
	}
	if !r.Bands.Exists {
		return Step{Stage: "maintain", Reason: "no bands.yaml for production metrics", Command: "huddle bands new <metric>"}
	}
	return Step{Stage: "plan", Reason: "every change is verified", Command: "huddle intent new \"<next problem>\""}
}

func role(cfg config.Config, kind string) string {
	k, _ := artifacts.Get(cfg, kind)
	return k.GateRole
}

func stageOf(kind string) string {
	k, ok := artifacts.Get(config.Config{}, kind)
	if !ok {
		return kind
	}
	return k.Stage
}
