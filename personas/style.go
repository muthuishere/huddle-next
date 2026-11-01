package personas

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaxStyleCardBytes is the ADR 13 budget for one rendered style card.
const MaxStyleCardBytes = 1600

// TaskFocus is the one-line focus added by `style --for <task>`.
var TaskFocus = map[string]string{
	"review":  "Task focus: review — flag what you would block on vs. nits, in your own voice.",
	"spec":    "Task focus: spec — push on scope, acceptance criteria, and what is left unsaid.",
	"intent":  "Task focus: intent — test whether the problem and outcome are real and specific.",
	"pr":      "Task focus: PR description — say what changed, why, and how it was verified.",
	"email":   "Task focus: email — keep your stance, match the reader, short and concrete.",
	"standup": "Task focus: standup — done, next, blocked; one line each.",
}

// StyleCard is a prompt-ready summary of how a persona thinks and speaks.
type StyleCard struct {
	ID                 string   `json:"id"`
	DisplayName        string   `json:"displayName"`
	Icon               string   `json:"icon,omitempty"`
	Title              string   `json:"title,omitempty"`
	PrimaryLens        string   `json:"primaryLens,omitempty"`
	CommunicationStyle string   `json:"communicationStyle,omitempty"`
	Principles         string   `json:"principles,omitempty"`
	SignaturePhrases   []string `json:"signaturePhrases,omitempty"`
	WhenUseful         string   `json:"whenUseful,omitempty"`
	BlindSpots         []string `json:"blindSpots,omitempty"`
	BasedOn            string   `json:"basedOn,omitempty"`
	For                string   `json:"for,omitempty"`
	Focus              string   `json:"focus,omitempty"`
}

// fieldBudgets (bytes) are applied before the whole-card check; tuned so every
// built-in fits MaxStyleCardBytes with room for a task focus line.
var fieldBudgets = struct{ lens, style, principles, phrase, when, blind int }{200, 340, 300, 90, 220, 160}

// Style builds the card for p. forTask is "" or a key of TaskFocus.
func Style(p *Persona, forTask string) (StyleCard, error) {
	if forTask != "" {
		if _, ok := TaskFocus[forTask]; !ok {
			return StyleCard{}, fmt.Errorf("unknown --for %q — use one of: %s", forTask, strings.Join(TaskKeys(), ", "))
		}
	}
	body, err := p.Body()
	if err != nil {
		return StyleCard{}, err
	}
	secs := Sections(body)
	name := p.DisplayName
	if name == "" {
		name = p.ID
	}
	card := StyleCard{
		ID: p.ID, DisplayName: name, Icon: p.Icon, Title: p.Title,
		PrimaryLens:        p.PrimaryLens,
		CommunicationStyle: p.CommunicationStyle,
		Principles:         p.Principles,
		SignaturePhrases:   firstN(bullets(secs["Signature Phrases"]), 4),
		WhenUseful:         flatten(secs["When Useful"]),
		BlindSpots:         firstN(itemsOrSentences(secs["Blind Spots"]), 2),
		For:                forTask,
		Focus:              TaskFocus[forTask],
	}
	if v, ok := p.Extra["basedOn"]; ok && v != "" {
		card.BasedOn = "A style modelled on " + v + " — not that person speaking."
	}
	fitCard(&card)
	return card, nil
}

// TaskKeys lists valid --for values, sorted.
func TaskKeys() []string {
	keys := make([]string, 0, len(TaskFocus))
	for k := range TaskFocus {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fitCard(c *StyleCard) {
	b := fieldBudgets
	for attempt := 0; attempt < 6; attempt++ {
		c.PrimaryLens = trimTo(c.PrimaryLens, b.lens)
		c.CommunicationStyle = trimTo(c.CommunicationStyle, b.style)
		c.Principles = trimTo(c.Principles, b.principles)
		for i := range c.SignaturePhrases {
			c.SignaturePhrases[i] = trimTo(c.SignaturePhrases[i], b.phrase)
		}
		c.WhenUseful = trimTo(c.WhenUseful, b.when)
		for i := range c.BlindSpots {
			c.BlindSpots[i] = trimTo(c.BlindSpots[i], b.blind)
		}
		if len(c.Markdown()) <= MaxStyleCardBytes {
			return
		}
		b.lens, b.style, b.principles, b.when, b.blind, b.phrase = b.lens*4/5, b.style*4/5, b.principles*4/5, b.when*4/5, b.blind*4/5, b.phrase*4/5
	}
}

// Markdown renders the card as a compact prompt fragment.
func (c StyleCard) Markdown() string {
	var s strings.Builder
	head := strings.TrimSpace(c.Icon + " " + c.DisplayName)
	if c.Title != "" {
		head += " — " + c.Title
	}
	fmt.Fprintf(&s, "### %s (%s)\n", head, c.ID)
	if c.BasedOn != "" {
		s.WriteString(c.BasedOn + "\n")
	}
	line := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&s, "- %s: %s\n", k, v)
		}
	}
	line("Lens", c.PrimaryLens)
	line("Voice", c.CommunicationStyle)
	line("Believes", c.Principles)
	if len(c.SignaturePhrases) > 0 {
		s.WriteString("- Says: " + strings.Join(c.SignaturePhrases, " / ") + "\n")
	}
	line("Pull in when", c.WhenUseful)
	if len(c.BlindSpots) > 0 {
		s.WriteString("- Watch for: " + strings.Join(c.BlindSpots, " ") + "\n")
	}
	if c.Focus != "" {
		s.WriteString(c.Focus + "\n")
	}
	return s.String()
}

func bullets(sec string) []string {
	var out []string
	for _, line := range strings.Split(sec, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			out = append(out, strings.TrimSpace(t[2:]))
		}
	}
	return out
}

func itemsOrSentences(sec string) []string {
	if b := bullets(sec); len(b) > 0 {
		return b
	}
	text := flatten(sec)
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') && (i+1 == len(text) || text[i+1] == ' ') {
			out = append(out, strings.TrimSpace(text[start:i+1]))
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

func flatten(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

// trimTo shortens s to at most n bytes at a word boundary, adding "…".
func trimTo(s string, n int) string {
	s = flatten(s)
	if len(s) <= n || n <= 0 {
		return s
	}
	cut := n - len("…")
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut-- // don't split a UTF-8 sequence
	}
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > n/2 {
		cut = sp
	}
	return strings.TrimRight(s[:cut], " ,;:.—-") + "…"
}

// Brief is the fragment for handing a task to any agent (ADR 10 §7).
type Brief struct {
	Task       string       `json:"task"`
	Cards      []StyleCard  `json:"cards"`
	Memories   []BriefEntry `json:"memories,omitempty"`
	Guardrails []string     `json:"guardrails,omitempty"`
}

// BriefEntry is one relevant knowledge/memory file (title + path, no body).
type BriefEntry struct {
	Persona string `json:"persona"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Source  Layer  `json:"source"`
}

// maxBriefMemories caps relevant memory entries per persona.
const maxBriefMemories = 5

// MakeBrief builds a brief for ids and task; repoRoot locates guardrail files.
func MakeBrief(r *Roster, ids []string, task, forTask, repoRoot string) (Brief, error) {
	br := Brief{Task: task}
	q := tokens(task)
	for _, id := range ids {
		p, ok := r.Get(id)
		if !ok {
			return br, fmt.Errorf("no persona %q in any layer — run: huddle persona list", id)
		}
		card, err := Style(p, forTask)
		if err != nil {
			return br, err
		}
		br.Cards = append(br.Cards, card)
		type scored struct {
			k Knowledge
			s int
		}
		var hits []scored
		for _, k := range p.Knowledge {
			if s := overlap(q, tokens(k.Title+" "+k.Rel+" "+strings.Join(k.Tags, " "))); s > 0 {
				hits = append(hits, scored{k, s})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].s > hits[b].s })
		for i, h := range hits {
			if i >= maxBriefMemories {
				break
			}
			br.Memories = append(br.Memories, BriefEntry{Persona: p.ID, Kind: h.k.Kind, Title: h.k.Title, Path: h.k.Path, Source: h.k.Layer})
		}
	}
	br.Guardrails = GuardrailFiles(repoRoot)
	return br, nil
}

// GuardrailFiles lists repo guardrail files that exist (paths only).
func GuardrailFiles(repoRoot string) []string {
	if repoRoot == "" {
		return nil
	}
	var out []string
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", filepath.Join(".claude", "CLAUDE.md")} {
		p := filepath.Join(repoRoot, rel)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// Markdown renders the brief.
func (b Brief) Markdown() string {
	var s strings.Builder
	fmt.Fprintf(&s, "## Task\n%s\n\n## Personas\n", b.Task)
	for _, c := range b.Cards {
		s.WriteString(c.Markdown())
		s.WriteString("\n")
	}
	if len(b.Memories) > 0 {
		s.WriteString("## Relevant memories (read on demand)\n")
		for _, m := range b.Memories {
			fmt.Fprintf(&s, "- [%s/%s] %s — %s\n", m.Persona, m.Source, m.Title, m.Path)
		}
		s.WriteString("\n")
	}
	if len(b.Guardrails) > 0 {
		s.WriteString("## Repo guardrails (read before acting)\n")
		for _, g := range b.Guardrails {
			s.WriteString("- " + g + "\n")
		}
	}
	return s.String()
}
