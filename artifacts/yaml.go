package artifacts

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseYAML parses the small YAML subset huddle's own files use: nested block
// mappings by indentation, block lists ("- x", "- k: v"), inline lists
// ("[a, b]"), quoted and plain scalars, and "#" comments. Numbers become
// float64, true/false become bool. Anchors, multi-line strings, and flow
// mappings are not supported and return an error with the line number.
func ParseYAML(src []byte) (map[string]any, error) {
	var lines []yline
	for i, raw := range strings.Split(string(src), "\n") {
		text := stripComment(strings.TrimRight(raw, "\r"))
		if strings.TrimSpace(text) == "" || strings.TrimSpace(text) == "---" {
			continue
		}
		indent := len(text) - len(strings.TrimLeft(text, " "))
		if strings.HasPrefix(strings.TrimLeft(text, " "), "\t") {
			return nil, fmt.Errorf("line %d: tabs are not allowed for indentation", i+1)
		}
		lines = append(lines, yline{n: i + 1, indent: indent, text: strings.TrimSpace(text)})
	}
	p := &yparser{lines: lines}
	if len(lines) == 0 {
		return map[string]any{}, nil
	}
	v, err := p.block(lines[0].indent)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation", p.lines[p.pos].n)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top level must be a mapping")
	}
	return m, nil
}

type yline struct {
	n      int
	indent int
	text   string
}

type yparser struct {
	lines []yline
	pos   int
}

func (p *yparser) block(indent int) (any, error) {
	if p.pos >= len(p.lines) {
		return nil, nil
	}
	if strings.HasPrefix(p.lines[p.pos].text, "- ") || p.lines[p.pos].text == "-" {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func (p *yparser) mapping(indent int) (map[string]any, error) {
	out := map[string]any{}
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: unexpected indentation", l.n)
		}
		if strings.HasPrefix(l.text, "- ") {
			break
		}
		k, v, ok := splitYKV(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"key: value\"", l.n)
		}
		p.pos++
		if v != "" {
			out[k] = scalar(v)
			continue
		}
		if p.pos < len(p.lines) && (p.lines[p.pos].indent > indent ||
			p.lines[p.pos].indent == indent && strings.HasPrefix(p.lines[p.pos].text, "- ")) {
			child, err := p.block(p.lines[p.pos].indent)
			if err != nil {
				return nil, err
			}
			out[k] = child
			continue
		}
		out[k] = nil
	}
	return out, nil
}

func (p *yparser) list(indent int) ([]any, error) {
	var out []any
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent != indent || !(strings.HasPrefix(l.text, "- ") || l.text == "-") {
			break
		}
		item := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		if item == "" {
			p.pos++
			if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
				child, err := p.block(p.lines[p.pos].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, child)
			} else {
				out = append(out, nil)
			}
			continue
		}
		if _, _, ok := splitYKV(item); ok && !strings.HasPrefix(item, "\"") && !strings.HasPrefix(item, "[") {
			// "- k: v" starts a mapping whose other keys sit at indent+2.
			itemIndent := indent + (len(l.text) - len(item))
			p.lines[p.pos] = yline{n: l.n, indent: itemIndent, text: item}
			m, err := p.mapping(itemIndent)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
			continue
		}
		p.pos++
		out = append(out, scalar(item))
	}
	return out, nil
}

func splitYKV(s string) (string, string, bool) {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == ':' && (i+1 == len(s) || s[i+1] == ' '):
			k := strings.TrimSpace(s[:i])
			if k == "" {
				return "", "", false
			}
			return strings.Trim(k, `"'`), strings.TrimSpace(s[i+1:]), true
		}
	}
	return "", "", false
}

func stripComment(s string) string {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

func scalar(v string) any {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		if v[0] == '"' {
			if s, err := strconv.Unquote(v); err == nil {
				return s
			}
		}
		return v[1 : len(v)-1]
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		var out []any
		for _, part := range strings.Split(v[1:len(v)-1], ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, scalar(part))
			}
		}
		return out
	}
	switch strings.ToLower(v) {
	case "true", "yes", "on":
		return true
	case "false", "no", "off":
		return false
	case "null", "~":
		return nil
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}
