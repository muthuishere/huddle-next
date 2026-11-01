package artifacts

import (
	"bytes"
	"strings"
)

// FrontMatter is a minimal, order-preserving YAML front matter block. Only the
// entries a command changes are re-rendered; every other line is kept byte for
// byte, so hand edits and comments survive a rewrite.
type FrontMatter struct {
	Present bool
	entries []*fmEntry
}

type fmEntry struct {
	Key      string // "" for comments / blank lines
	Value    string // raw value text after "key:"
	Children []*fmEntry
	raw      []string
	dirty    bool
}

// SplitFrontMatter separates a leading "---" block from the body.
func SplitFrontMatter(content []byte) (FrontMatter, []byte) {
	s := string(content)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return FrontMatter{}, content
	}
	nl := strings.IndexByte(s, '\n') + 1
	rest := s[nl:]
	end := -1
	off := 0
	for off <= len(rest) {
		i := strings.IndexByte(rest[off:], '\n')
		line := rest[off:]
		if i >= 0 {
			line = rest[off : off+i]
		}
		if strings.TrimRight(line, "\r ") == "---" {
			end = off
			break
		}
		if i < 0 {
			break
		}
		off += i + 1
	}
	if end < 0 {
		return FrontMatter{}, content
	}
	block := rest[:end]
	after := rest[end:]
	if i := strings.IndexByte(after, '\n'); i >= 0 {
		after = after[i+1:]
	} else {
		after = ""
	}
	return parseFrontMatter(block), []byte(after)
}

func parseFrontMatter(block string) FrontMatter {
	fm := FrontMatter{Present: true}
	lines := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	if block == "" {
		lines = nil
	}
	var cur *fmEntry
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if indented && cur != nil && trimmed != "" {
			cur.raw = append(cur.raw, line)
			if strings.HasPrefix(trimmed, "- ") {
				if n := len(cur.Children); n > 0 {
					c := cur.Children[n-1]
					item := strings.TrimSpace(trimmed[2:])
					c.Value = appendListValue(c.Value, unquote(item))
				}
				continue
			}
			if k, v, ok := splitKV(trimmed); ok {
				cur.Children = append(cur.Children, &fmEntry{Key: k, Value: v})
			}
			continue
		}
		if k, v, ok := splitKV(line); ok && !indented && !strings.HasPrefix(trimmed, "#") {
			cur = &fmEntry{Key: k, Value: v, raw: []string{line}}
			fm.entries = append(fm.entries, cur)
			continue
		}
		fm.entries = append(fm.entries, &fmEntry{raw: []string{line}})
		cur = nil
	}
	return fm
}

func splitKV(line string) (string, string, bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:i])
	if k == "" || strings.ContainsAny(k, " \t\"'") {
		return "", "", false
	}
	return k, strings.TrimSpace(line[i+1:]), true
}

func appendListValue(existing, item string) string {
	vals := parseList(existing)
	vals = append(vals, item)
	return renderList(vals)
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	// strip trailing comments on plain scalars
	if i := strings.Index(s, " #"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func parseList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "[]" || v == "{}" {
		return nil
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		var out []string
		for _, p := range strings.Split(v[1:len(v)-1], ",") {
			if p = unquote(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return []string{unquote(v)}
}

func renderList(vals []string) string {
	if len(vals) == 1 {
		return quoteIfNeeded(vals[0])
	}
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = quoteIfNeeded(v)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func quoteIfNeeded(v string) string {
	if v == "" || strings.ContainsAny(v, ",[]{}#\"'") || strings.Contains(v, ": ") || strings.HasPrefix(v, " ") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}

func (fm *FrontMatter) find(key string) *fmEntry {
	for _, e := range fm.entries {
		if e.Key == key {
			return e
		}
	}
	return nil
}

// Get returns an unquoted top-level scalar, or "".
func (fm *FrontMatter) Get(key string) string {
	if e := fm.find(key); e != nil {
		return unquote(e.Value)
	}
	return ""
}

// Set replaces or appends a top-level scalar.
func (fm *FrontMatter) Set(key, value string) {
	fm.Present = true
	if e := fm.find(key); e != nil {
		e.Value = quoteIfNeeded(value)
		e.Children = nil
		e.dirty = true
		return
	}
	fm.entries = append(fm.entries, &fmEntry{Key: key, Value: quoteIfNeeded(value), dirty: true})
}

// Links returns links as kind -> values.
func (fm *FrontMatter) Links() map[string][]string {
	out := map[string][]string{}
	e := fm.find("links")
	if e == nil {
		return out
	}
	for _, c := range e.Children {
		if vals := parseList(c.Value); len(vals) > 0 {
			out[c.Key] = vals
		}
	}
	return out
}

// AddLink appends value under links.<key> unless already present.
func (fm *FrontMatter) AddLink(key, value string) bool {
	fm.Present = true
	e := fm.find("links")
	if e == nil {
		e = &fmEntry{Key: "links"}
		fm.entries = append(fm.entries, e)
	}
	for _, c := range e.Children {
		if c.Key == key {
			vals := parseList(c.Value)
			for _, v := range vals {
				if v == value {
					return false
				}
			}
			c.Value = renderList(append(vals, value))
			e.dirty = true
			return true
		}
	}
	e.Children = append(e.Children, &fmEntry{Key: key, Value: quoteIfNeeded(value)})
	e.Value = ""
	e.dirty = true
	return true
}

// Render returns the block including both "---" fences.
func (fm *FrontMatter) Render() []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	for _, e := range fm.entries {
		if !e.dirty && e.raw != nil {
			for _, l := range e.raw {
				b.WriteString(l)
				b.WriteByte('\n')
			}
			continue
		}
		if len(e.Children) == 0 {
			v := e.Value
			if e.Key == "links" && v == "" {
				v = "{}"
			}
			b.WriteString(e.Key + ": " + v + "\n")
			continue
		}
		b.WriteString(e.Key + ":\n")
		for _, c := range e.Children {
			b.WriteString("  " + c.Key + ": " + c.Value + "\n")
		}
	}
	b.WriteString("---\n")
	return b.Bytes()
}
