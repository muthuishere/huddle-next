package personas

import (
	"sort"
	"strings"
	"unicode"
)

// Match is one `persona find` result.
type Match struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName,omitempty"`
	Title       string   `json:"title,omitempty"`
	Score       int      `json:"score"`
	Selectable  bool     `json:"selectable"`
	Matched     []string `json:"matched"`             // which fields matched
	Knowledge   []string `json:"knowledge,omitempty"` // matching knowledge titles
}

// Find ranks personas by token overlap with query. Weights: id/name 3,
// title/role 2, domains/capabilities 1, knowledge titles 1.
func Find(r *Roster, query string) []Match {
	q := tokens(query)
	if len(q) == 0 {
		return nil
	}
	var out []Match
	for _, p := range r.Personas {
		m := Match{ID: p.ID, DisplayName: p.DisplayName, Title: p.Title, Selectable: p.Selectable}
		score := func(field string, weight int, text string) {
			if n := overlap(q, tokens(text)); n > 0 {
				m.Score += n * weight
				m.Matched = append(m.Matched, field)
			}
		}
		score("id", 3, p.ID+" "+p.Name+" "+p.DisplayName)
		score("title", 2, p.Title)
		score("role", 2, p.Role)
		score("domains", 1, strings.Join(p.Domains, " "))
		score("capabilities", 1, p.Capabilities)
		for _, k := range p.Knowledge {
			if n := overlap(q, tokens(k.Title+" "+strings.Join(k.Tags, " "))); n > 0 {
				m.Score += n
				m.Knowledge = append(m.Knowledge, k.Title)
			}
		}
		if len(m.Knowledge) > 0 {
			m.Matched = append(m.Matched, "knowledge")
		}
		if m.Score > 0 {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return out[a].ID < out[b].ID
	})
	return out
}

// tokens lowercases and splits on anything that is not a letter or digit,
// dropping very short and stop words. The result is a set.
func tokens(s string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(f) < 2 || stopWords[f] {
			continue
		}
		set[f] = struct{}{}
	}
	return set
}

var stopWords = map[string]bool{"the": true, "and": true, "for": true, "of": true, "to": true, "in": true,
	"on": true, "a": true, "an": true, "is": true, "this": true, "that": true, "with": true, "or": true, "it": true}

func overlap(q, t map[string]struct{}) int {
	n := 0
	for w := range q {
		if _, ok := t[w]; ok {
			n++
		}
	}
	return n
}
