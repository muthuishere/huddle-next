// Package artifacts implements the playbook artifact catalog (ADR 12, 18, 19):
// kinds, default locations, templates, discovery, and structural lint. The
// committed file is the source of truth — status is always read from it.
package artifacts

import (
	"sort"
	"strings"

	"github.com/muthuishere/huddle/internal/config"
)

// Format of an artifact file.
const (
	FormatMD   = "md"
	FormatJSON = "json"
	FormatYAML = "yaml"
)

// Kind describes one artifact noun.
type Kind struct {
	Name     string
	Pattern  string   // default path pattern; {slug}, {n}, {date} placeholders
	Template string   // file under assets/templates
	Format   string   // md | json | yaml
	Required []string // required headings (md)
	GateRole string   // default owner role that approves it
	Stage    string
	Summary  string
}

// Singleton reports whether the kind has exactly one file (no placeholders).
func (k Kind) Singleton() bool { return !strings.Contains(k.Pattern, "{") }

// Numbered reports whether new files get an auto-incremented number.
func (k Kind) Numbered() bool { return strings.Contains(k.Pattern, "{n}") }

var kinds = map[string]Kind{
	"intent": {Name: "intent", Pattern: "intent/{slug}/intent.md", Template: "intent.md", Format: FormatMD, GateRole: "product", Stage: "plan",
		Summary:  "Problem, outcome, affected users and systems (intent/<slug>/intent.md)",
		Required: []string{"Author", "Status", "Problem", "Proposed outcome", "Affected users and systems", "Constraints", "Open questions"}},
	"spec": {Name: "spec", Pattern: "intent/{slug}/spec.md", Template: "spec.md", Format: FormatMD, GateRole: "tech-lead", Stage: "design",
		Summary:  "Requirements with acceptance, design, policy concerns (intent/<slug>/spec.md)",
		Required: []string{"Requirements", "Design", "Policy concerns", "Non-goals", "Links"}},
	"plan": {Name: "plan", Pattern: "intent/{slug}/plan.md", Template: "plan.md", Format: FormatMD, GateRole: "tech-lead", Stage: "build",
		Summary:  "Files that change, order of work, risks, proof (intent/<slug>/plan.md)",
		Required: []string{"Files that change", "Order of work", "Risks", "Proof"}},
	"adr": {Name: "adr", Pattern: "docs/adr/{n}-{slug}.md", Template: "adr.md", Format: FormatMD, GateRole: "tech-lead", Stage: "design",
		Summary:  "Architecture decision record (docs/adr/<n>-<slug>.md)",
		Required: []string{"Status", "Context", "Decision", "Consequences", "binds", "prevents"}},
	"spike": {Name: "spike", Pattern: "spikes/{n}-{slug}/spike.md", Template: "spike.md", Format: FormatMD, GateRole: "tech-lead", Stage: "plan",
		Summary:  "Time-boxed investigation (spikes/<n>-<slug>/spike.md)",
		Required: []string{"Question", "Why it matters", "Hypothesis", "Timebox", "Approach", "Findings", "Recommendation", "Follow-ups", "Status"}},
	"guardrails": {Name: "guardrails", Pattern: "CLAUDE.md", Template: "guardrails.md", Format: FormatMD, GateRole: "tech-lead", Stage: "setup",
		Summary:  "CLAUDE.md guardrails (+ AGENTS.md mirror)",
		Required: []string{"Commands", "Conventions", "Architecture", "Things Claude gets wrong"}},
	"review-policy": {Name: "review-policy", Pattern: "REVIEW.md", Template: "review-policy.md", Format: FormatMD, GateRole: "code", Stage: "deploy",
		Summary:  "REVIEW.md review policy",
		Required: []string{"Passes", "What Important means here", "Cap the nits", "Do not report"}},
	"eval": {Name: "eval", Pattern: "evals/{slug}.json", Template: "eval.json", Format: FormatJSON, GateRole: "tech-lead", Stage: "test",
		Summary: "Eval case: prompt + checks (evals/<slug>.json)"},
	"bands": {Name: "bands", Pattern: "bands.yaml", Template: "bands.yaml", Format: FormatYAML, GateRole: "service", Stage: "maintain",
		Summary: "Control bands: metric, baseline, rules, tiers (bands.yaml)"},
	"skill": {Name: "skill", Pattern: ".claude/skills/{slug}/SKILL.md", Template: "skill.md", Format: FormatMD, GateRole: "policy", Stage: "design",
		Summary:  "Team policy as a skill (.claude/skills/<name>/SKILL.md)",
		Required: []string{"When to use", "Policy"}},
	"lessons": {Name: "lessons", Pattern: "docs/lessons/{date}-{slug}.md", Template: "lessons.md", Format: FormatMD, GateRole: "service", Stage: "maintain",
		Summary:  "Post-mortem linked to an eval and an intent (docs/lessons/<date>-<slug>.md)",
		Required: []string{"What happened", "Impact", "Root cause", "Eval added", "Intent links"}},
}

// Get returns a kind with any config path override applied.
func Get(cfg config.Config, name string) (Kind, bool) {
	k, ok := kinds[name]
	if !ok {
		return Kind{}, false
	}
	if p := strings.TrimSpace(cfg.Artifacts[name]); p != "" {
		k.Pattern = strings.TrimPrefix(p, "./")
	}
	return k, true
}

// Names lists every kind name, sorted.
func Names() []string {
	out := make([]string, 0, len(kinds))
	for n := range kinds {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Slugify turns a title into a path-safe slug.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}
