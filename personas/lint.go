package personas

import (
	"fmt"
	"strings"
)

// RequiredKeys are the front-matter keys every persona needs (ADR 4).
var RequiredKeys = []string{"name", "displayName", "title", "icon", "role", "domains",
	"identity", "primaryLens", "communicationStyle", "principles"}

// RequiredSections are the body sections every persona needs (ADR 4).
var RequiredSections = []string{"Signature Phrases", "Expertise Areas", "Blind Spots", "When Useful"}

// Severity of a lint issue.
const (
	SevError   = "error"
	SevWarning = "warning"
	SevWaived  = "waived"
)

// Issue is one lint finding.
type Issue struct {
	ID       string `json:"id,omitempty"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

func (i Issue) String() string {
	loc := i.Path
	if loc == "" {
		loc = i.ID
	}
	return fmt.Sprintf("%s: %s: %s", i.Severity, loc, i.Message)
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SevError {
			return true
		}
	}
	return false
}

// LintDefinition validates one standalone definition file (used by `persona new`).
func LintDefinition(id, path string, data []byte) []Issue {
	var out []Issue
	add := func(sev, msg string) { out = append(out, Issue{ID: id, Severity: sev, Path: path, Message: msg}) }
	fm := ParseFrontMatter(data)
	if !fm.Present {
		add(SevError, "missing front matter: the file must start with a --- line")
	}
	for _, prob := range fm.Problems {
		add(SevError, "front matter "+prob)
	}
	for _, key := range RequiredKeys {
		if v, ok := fm.Fields[key]; !ok || strings.TrimSpace(v.String()) == "" {
			add(SevError, fmt.Sprintf("missing required front-matter key %q", key))
		}
	}
	if v, ok := fm.Fields["domains"]; ok && !v.IsList {
		add(SevWarning, `domains should be an inline list, e.g. domains: [reliability, oncall]`)
	}
	body := ""
	if fm.BodyAt <= len(data) {
		body = string(data[fm.BodyAt:])
	}
	secs := Sections(body)
	for _, s := range RequiredSections {
		if strings.TrimSpace(secs[s]) == "" {
			add(SevError, fmt.Sprintf("missing required section \"## %s\"", s))
		}
	}
	return out
}

// Lint validates resolved personas across all layers. id "" lints everything.
func (r *Roster) Lint(id string) ([]Issue, error) {
	var targets []*Persona
	if id != "" {
		p, ok := r.Get(id)
		if !ok {
			return nil, fmt.Errorf("no persona %q in any layer — run: huddle persona list", id)
		}
		targets = []*Persona{p}
	} else {
		targets = r.Personas
	}
	var out []Issue
	if id == "" {
		for _, o := range r.Overflow {
			out = append(out, Issue{Severity: SevWarning, Message: o})
		}
	}
	for _, p := range targets {
		out = append(out, lintPersona(p)...)
	}
	return out, nil
}

func lintPersona(p *Persona) []Issue {
	var out []Issue
	add := func(sev, path, msg string) {
		out = append(out, Issue{ID: p.ID, Severity: sev, Path: path, Message: msg})
	}
	for _, prob := range p.Problems {
		add(SevError, "", prob)
	}
	if p.Definition == nil {
		add(SevError, "", fmt.Sprintf("no %s.md definition in any layer (%s) — add one with: huddle persona new %s", p.ID, joinLayers(p.Layers), p.ID))
		return out
	}
	for _, key := range RequiredKeys {
		if v, ok := p.fields[key]; !ok || strings.TrimSpace(v.String()) == "" {
			add(SevError, p.Definition.Path, fmt.Sprintf("missing required front-matter key %q (no layer sets it)", key))
		}
	}
	body, err := p.Body()
	if err != nil {
		add(SevError, p.Definition.Path, err.Error())
		return out
	}
	secs := Sections(body)
	for _, s := range RequiredSections {
		if strings.TrimSpace(secs[s]) != "" {
			continue
		}
		if p.Definition.Layer == Builtin && waivedSection(p.ID, s) {
			add(SevWaived, p.Definition.Path, fmt.Sprintf("section \"## %s\" absent in built-in worker persona", s))
			continue
		}
		add(SevError, p.Definition.Path, fmt.Sprintf("missing required section \"## %s\"", s))
	}
	return out
}

func joinLayers(ls []Layer) string {
	s := make([]string, len(ls))
	for i, l := range ls {
		s[i] = string(l)
	}
	return strings.Join(s, ", ")
}
