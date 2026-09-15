// A small, dependency-free Markdown renderer for the artifact view (ADR 20 §2).
// It covers what Huddle's own artifacts use — headings, fences, lists, tables of
// links, emphasis, quotes, rules — and nothing more.
//
// Safety: every line is HTML-escaped BEFORE any tag is emitted, so no input can
// introduce markup. Link targets are restricted to http, https, mailto and
// relative paths, which keeps javascript: and data: URLs out by construction.
package serve

import (
	"html"
	"regexp"
	"strings"
)

var (
	mdInlineCode = regexp.MustCompile("`([^`]+)`")
	mdBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdItalic     = regexp.MustCompile(`\*([^*\n]+)\*`)
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
	mdHeading    = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mdListItem   = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	mdOrdered    = regexp.MustCompile(`^\s*\d+[.)]\s+(.*)$`)
)

// safeHref keeps only schemes that cannot execute script.
func safeHref(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", false
	}
	low := strings.ToLower(u)
	switch {
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"), strings.HasPrefix(low, "mailto:"):
		return u, true
	case strings.Contains(low[:min(len(low), 12)], ":"):
		return "", false // some other scheme — refuse it
	default:
		return u, true // relative path
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isRule reports a thematic break: three or more of -, * or _ and nothing else.
// RE2 has no backreferences, so this is a plain scan rather than a regexp.
func isRule(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	var mark byte
	n := 0
	for i := 0; i < len(t); i++ {
		switch c := t[i]; c {
		case '-', '*', '_':
			if mark == 0 {
				mark = c
			} else if c != mark {
				return false
			}
			n++
		case ' ', '\t':
		default:
			return false
		}
	}
	return n >= 3
}

// inline renders emphasis, code and links in an already-escaped line.
func inline(escaped string) string {
	s := mdInlineCode.ReplaceAllString(escaped, "<code>$1</code>")
	s = mdBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = mdItalic.ReplaceAllString(s, "<em>$1</em>")
	s = mdLink.ReplaceAllStringFunc(s, func(m string) string {
		p := mdLink.FindStringSubmatch(m)
		href, ok := safeHref(html.UnescapeString(p[2]))
		if !ok {
			return p[1]
		}
		return `<a href="` + html.EscapeString(href) + `" rel="noreferrer noopener">` + p[1] + `</a>`
	})
	return s
}

// RenderMarkdown converts Markdown to a safe HTML fragment.
func RenderMarkdown(src string) string {
	var b strings.Builder
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")

	inFence, listOpen, listTag, paraOpen := false, false, "", false
	closeList := func() {
		if listOpen {
			b.WriteString("</" + listTag + ">\n")
			listOpen = false
		}
	}
	closePara := func() {
		if paraOpen {
			b.WriteString("</p>\n")
			paraOpen = false
		}
	}

	for _, raw := range lines {
		esc := html.EscapeString(raw)

		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			closeList()
			closePara()
			if inFence {
				b.WriteString("</code></pre>\n")
			} else {
				b.WriteString("<pre><code>")
			}
			inFence = !inFence
			continue
		}
		if inFence {
			b.WriteString(esc + "\n")
			continue
		}

		if strings.TrimSpace(raw) == "" {
			closeList()
			closePara()
			continue
		}
		if isRule(raw) {
			closeList()
			closePara()
			b.WriteString("<hr>\n")
			continue
		}
		if m := mdHeading.FindStringSubmatch(esc); m != nil {
			closeList()
			closePara()
			n := len(m[1])
			b.WriteString("<h" + string(rune('0'+n)) + ">" + inline(strings.TrimSpace(m[2])) + "</h" + string(rune('0'+n)) + ">\n")
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(raw), ">") {
			closeList()
			closePara()
			t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(esc), "&gt;"))
			b.WriteString("<blockquote>" + inline(t) + "</blockquote>\n")
			continue
		}

		var item string
		tag := ""
		if m := mdListItem.FindStringSubmatch(esc); m != nil {
			item, tag = m[1], "ul"
		} else if m := mdOrdered.FindStringSubmatch(esc); m != nil {
			item, tag = m[1], "ol"
		}
		if tag != "" {
			closePara()
			if listOpen && listTag != tag {
				closeList()
			}
			if !listOpen {
				b.WriteString("<" + tag + ">\n")
				listOpen, listTag = true, tag
			}
			b.WriteString("<li>" + inline(item) + "</li>\n")
			continue
		}

		closeList()
		if !paraOpen {
			b.WriteString("<p>")
			paraOpen = true
		} else {
			b.WriteString(" ")
		}
		b.WriteString(inline(esc))
	}

	if inFence {
		b.WriteString("</code></pre>\n")
	}
	closeList()
	closePara()
	return b.String()
}
