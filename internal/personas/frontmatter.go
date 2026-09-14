// Package personas resolves huddle's three persona layers (builtin < global <
// repo, ADR 3), serves style cards and briefs (ADR 10), validates persona
// definitions (ADR 4), and stores ingested evidence.
package personas

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Value is one front-matter value: a scalar string or an inline/block list.
type Value struct {
	Str    string   `json:"str,omitempty"`
	List   []string `json:"list,omitempty"`
	IsList bool     `json:"isList,omitempty"`
}

// String renders the value as text (lists comma-joined).
func (v Value) String() string {
	if v.IsList {
		return strings.Join(v.List, ", ")
	}
	return v.Str
}

// Items returns the value as a list (a scalar is split on commas).
func (v Value) Items() []string {
	if v.IsList {
		return v.List
	}
	var out []string
	for _, p := range strings.Split(v.Str, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// FrontMatter is a parsed `---` block. Keys keeps source order.
type FrontMatter struct {
	Fields   map[string]Value
	Keys     []string
	Present  bool     // the document opened with a --- delimiter
	Closed   bool     // a closing --- delimiter was found
	BodyAt   int      // byte offset of the body after the closing delimiter
	Problems []string // tolerated syntax problems, with line numbers
}

// ParseFrontMatter parses a leading YAML-ish front matter block. It supports the
// subset huddle personas use — `key: value`, quoted strings, inline lists
// `[a, b]`, `- item` block lists, indented continuation lines — and never
// panics: anything it cannot read is recorded in Problems and skipped.
func ParseFrontMatter(data []byte) FrontMatter {
	fm := FrontMatter{Fields: map[string]Value{}}
	offset := 0
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		offset = 3
	}
	total := len(data)
	data = data[offset:]
	first, rest, ok := cutLine(data)
	if !ok && len(first) == 0 {
		return fm
	}
	if strings.TrimSpace(string(first)) != "---" {
		return fm
	}
	fm.Present = true
	offset += len(data) - len(rest)
	lineNo := 1
	lastKey := ""
	for len(rest) > 0 {
		var line []byte
		before := len(rest)
		line, rest, _ = cutLine(rest)
		offset += before - len(rest)
		lineNo++
		text := strings.TrimRight(string(line), " \t\r")
		trimmed := strings.TrimSpace(text)
		if trimmed == "---" || trimmed == "..." {
			fm.Closed = true
			fm.BodyAt = offset
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := text != "" && (text[0] == ' ' || text[0] == '\t')
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if lastKey == "" {
				fm.Problems = append(fm.Problems, fmt.Sprintf("line %d: list item without a key", lineNo))
				continue
			}
			v := fm.Fields[lastKey]
			if !v.IsList && v.Str != "" {
				fm.Problems = append(fm.Problems, fmt.Sprintf("line %d: list item under scalar key %q", lineNo, lastKey))
				continue
			}
			v.IsList = true
			v.Str = ""
			v.List = append(v.List, unquote(stripComment(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))))
			fm.Fields[lastKey] = v
			continue
		}
		key, raw, hasColon := strings.Cut(trimmed, ":")
		key = strings.TrimSpace(key)
		if indented && lastKey != "" && (!hasColon || strings.ContainsAny(key, " \"'")) {
			// continuation of a folded scalar
			v := fm.Fields[lastKey]
			if !v.IsList {
				v.Str = strings.TrimSpace(v.Str + " " + unquote(trimmed))
				fm.Fields[lastKey] = v
			}
			continue
		}
		if !hasColon || key == "" || strings.ContainsAny(key, " \t\"'[]{}") {
			fm.Problems = append(fm.Problems, fmt.Sprintf("line %d: expected `key: value`, got %q", lineNo, clip(trimmed, 60)))
			continue
		}
		if indented {
			continue // nested mapping keys (e.g. basedOn: children) are kept raw on the parent only
		}
		fm.Fields[key] = parseScalar(strings.TrimSpace(raw))
		if _, seen := indexOf(fm.Keys, key); !seen {
			fm.Keys = append(fm.Keys, key)
		}
		lastKey = key
	}
	if !fm.Closed {
		fm.Problems = append(fm.Problems, "front matter is not closed with ---")
		fm.BodyAt = total
	}
	return fm
}

func indexOf(xs []string, s string) (int, bool) {
	for i, x := range xs {
		if x == s {
			return i, true
		}
	}
	return -1, false
}

func cutLine(b []byte) (line, rest []byte, found bool) {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i], b[i+1:], true
	}
	return b, nil, false
}

func parseScalar(raw string) Value {
	if raw == "" || raw == "|" || raw == ">" || raw == "|-" || raw == ">-" {
		return Value{}
	}
	if strings.HasPrefix(raw, "[") {
		inner := raw[1:]
		if end := strings.LastIndex(inner, "]"); end >= 0 {
			inner = inner[:end]
		}
		items := splitList(inner)
		return Value{IsList: true, List: items}
	}
	return Value{Str: unquote(stripComment(raw))}
}

// splitList splits on commas outside quotes.
func splitList(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if item := unquote(strings.TrimSpace(cur.String())); item != "" {
			out = append(out, item)
		}
		cur.Reset()
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
			cur.WriteRune(r)
		case r == ',':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	if out == nil {
		out = []string{}
	}
	return out
}

// stripComment removes a trailing ` # comment` from an unquoted scalar.
func stripComment(s string) string {
	if strings.HasPrefix(s, `"`) || strings.HasPrefix(s, "'") {
		return s
	}
	if i := strings.Index(s, " #"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' {
		if end := strings.LastIndex(s, `"`); end > 0 {
			if u, err := strconv.Unquote(s[:end+1]); err == nil {
				return u
			}
			return strings.ReplaceAll(s[1:end], `\"`, `"`)
		}
	}
	if len(s) >= 2 && s[0] == '\'' {
		if end := strings.LastIndex(s, "'"); end > 0 {
			return strings.ReplaceAll(s[1:end], "''", "'")
		}
	}
	return s
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Sections splits a markdown body into `## Heading` → content (trimmed).
func Sections(body string) map[string]string {
	out := map[string]string{}
	cur := ""
	var b strings.Builder
	inFence := false
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(b.String())
		}
		b.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(t), "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(t, "## ") {
			flush()
			cur = strings.TrimSpace(t[3:])
			continue
		}
		b.WriteString(t)
		b.WriteByte('\n')
	}
	flush()
	return out
}
