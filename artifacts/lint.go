package artifacts

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Finding is one lint problem.
type Finding struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Section is one markdown heading and the content under it (up to the next
// heading of the same or higher level).
type Section struct {
	Level   int
	Title   string
	Content string
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// Sections parses markdown headings, ignoring fenced code blocks.
func Sections(body string) []Section {
	type head struct {
		level int
		title string
		line  int
	}
	lines := strings.Split(body, "\n")
	var heads []head
	fence := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = !fence
			continue
		}
		if fence || !strings.HasPrefix(l, "#") {
			continue
		}
		lvl := len(l) - len(strings.TrimLeft(l, "#"))
		if lvl > 6 || len(l) == lvl || l[lvl] != ' ' {
			continue
		}
		heads = append(heads, head{lvl, strings.TrimSpace(strings.TrimRight(l[lvl:], "#")), i})
	}
	out := make([]Section, len(heads))
	for i, h := range heads {
		end := len(lines)
		for _, n := range heads[i+1:] {
			if n.level <= h.level {
				end = n.line
				break
			}
		}
		out[i] = Section{Level: h.level, Title: h.title, Content: strings.Join(lines[h.line+1:end], "\n")}
	}
	return out
}

// Meaningful reports whether text has content beyond whitespace and HTML comments.
func Meaningful(s string) bool {
	return strings.TrimSpace(htmlComment.ReplaceAllString(s, "")) != ""
}

// FindSection returns the first section whose title matches (case-insensitive).
func FindSection(secs []Section, title string) (Section, bool) {
	for _, s := range secs {
		if strings.EqualFold(s.Title, title) {
			return s, true
		}
	}
	return Section{}, false
}

// GuardrailsBegin and GuardrailsEnd delimit huddle's own block in CLAUDE.md.
const (
	GuardrailsBegin = "<!-- huddle:context -->"
	GuardrailsEnd   = "<!-- /huddle:context -->"
)

var acceptanceLine = regexp.MustCompile(`(?mi)^\s*(?:[-*]\s*)?\**acceptance\**\s*:\**\s*(\S.*)$`)

// Lint checks one artifact's structure.
func Lint(a Artifact, k Kind) []Finding {
	var out []Finding
	add := func(rule, format string, args ...any) {
		out = append(out, Finding{ID: a.ID, Path: a.Path, Rule: rule, Message: fmt.Sprintf(format, args...)})
	}
	switch k.Format {
	case FormatJSON:
		var doc map[string]any
		if err := json.Unmarshal(a.Raw, &doc); err != nil {
			add("json", "not valid JSON: %v", err)
			return out
		}
		if k.Name == "eval" {
			if p, _ := doc["prompt"].(string); strings.TrimSpace(p) == "" {
				add("eval.prompt", "\"prompt\" must be a non-empty string")
			}
			if c, _ := doc["checks"].([]any); len(c) == 0 {
				add("eval.checks", "\"checks\" must be a non-empty array")
			}
		}
		return out
	case FormatYAML:
		doc, err := ParseYAML(a.Raw)
		if err != nil {
			add("yaml", "not valid YAML: %v", err)
			return out
		}
		if k.Name == "bands" {
			if m, _ := doc["metric"].(string); strings.TrimSpace(m) == "" {
				add("bands.metric", "\"metric\" must name the metric")
			}
			for _, key := range []string{"baseline", "rules", "tiers"} {
				if _, ok := doc[key]; !ok {
					add("bands."+key, "missing %q", key)
				}
			}
		}
		return out
	}

	if !a.Front.Present {
		add("front-matter", "missing front matter (id, kind, status, author, links)")
	} else {
		for _, key := range []string{"id", "kind", "status", "author"} {
			if a.Front.Get(key) == "" {
				add("front-matter."+key, "front matter %q is empty", key)
			}
		}
		if st := a.Front.Get("status"); st != "" && !validStatus(st) {
			add("front-matter.status", "status %q must be one of %s", st, strings.Join(statuses, "|"))
		}
		if kind := a.Front.Get("kind"); kind != "" && kind != k.Name {
			add("front-matter.kind", "kind %q does not match %s", kind, k.Name)
		}
	}

	body := string(a.Body)
	if k.Name == "guardrails" {
		i := strings.Index(body, GuardrailsBegin)
		j := strings.Index(body, GuardrailsEnd)
		if i < 0 || j < i {
			add("guardrails.block", "missing %s … %s block", GuardrailsBegin, GuardrailsEnd)
		} else {
			body = body[i+len(GuardrailsBegin) : j]
		}
	}
	secs := Sections(body)
	for _, h := range k.Required {
		s, ok := FindSection(secs, h)
		switch {
		case !ok:
			add("heading", "missing heading %q", h)
		case !Meaningful(s.Content):
			add("heading.empty", "section %q is empty", h)
		}
	}

	switch k.Name {
	case "spec":
		if req, ok := FindSection(secs, "Requirements"); ok {
			subs := Sections(req.Content)
			n := 0
			for _, s := range subs {
				if s.Level < 3 {
					continue
				}
				n++
				if m := acceptanceLine.FindStringSubmatch(s.Content); m == nil || !Meaningful(m[1]) {
					add("spec.acceptance", "requirement %q has no \"Acceptance:\" line", s.Title)
				}
			}
			if n == 0 {
				add("spec.requirements", "Requirements needs at least one \"### R1: …\" requirement with an \"Acceptance:\" line")
			}
		}
	case "review-policy":
		if p, ok := FindSection(secs, "Passes"); ok {
			for _, pass := range []string{"Bugs", "Security", "Compliance"} {
				if !regexp.MustCompile(`(?mi)^#{3,}\s+` + pass).MatchString(p.Content) {
					add("review-policy.passes", "Passes is missing the %q pass", pass)
				}
			}
		}
	case "skill":
		name := a.Front.Get("name")
		desc := a.Front.Get("description")
		if strings.TrimSpace(name) == "" {
			add("skill.name", "front matter \"name\" is empty")
		}
		if strings.TrimSpace(desc) == "" {
			add("skill.description", "front matter \"description\" is empty")
		}
		if len(desc) > 260 {
			add("skill.description.length", "description is %d bytes; keep it ≤ 260", len(desc))
		}
		if !strings.Contains(strings.ToLower(desc), "use when") {
			add("skill.trigger", "description needs a trigger phrase (\"Use when …\")")
		}
	}
	return out
}

func validStatus(s string) bool {
	for _, v := range statuses {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
