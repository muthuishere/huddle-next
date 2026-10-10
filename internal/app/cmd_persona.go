package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/personas"
)

func init() {
	register(Command{Name: "persona", Summary: "List, resolve, style, brief, create, import, ingest and lint personas", Run: runPersona})
}

// personaStdin is read by import/ingest when the input is "-" or absent.
var personaStdin io.Reader = os.Stdin

const personaHelp = `Usage:
  huddle persona list [--layer builtin|global|repo] [--json]
  huddle persona resolve [<id>] [--explain] [--json]
  huddle persona style <id> [--for review|spec|intent|pr|email|standup] [--json|--md]
  huddle persona brief <id>[,<id>…] --task "<text>" [--for x] [--json]
  huddle persona find "<query>" [--json]
  huddle persona new <id> --scope global|repo (--from <file> | --template) [--force] [--dry-run]
  huddle persona import <file|-> --scope global|repo --id <id> [--force] [--dry-run]
  huddle persona ingest <id> [--file <f>] [--label <text>] [--json]   (stdin when no --file)
  huddle persona ingest --schema
  huddle persona lint [<id>] [--json]

Layers merge builtin < global (~/.config/.huddle/personas) < repo (<repo>/.huddle/personas).
`

func runPersona(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		_, err := io.WriteString(c.Stdout, personaHelp)
		return err
	}
	sub, rest := args[0], args[1:]
	handlers := map[string]func(*Ctx, []string) error{
		"list": personaList, "resolve": personaResolve, "style": personaStyle, "brief": personaBrief,
		"find": personaFind, "new": personaNew, "import": personaImport, "ingest": personaIngest, "lint": personaLint,
	}
	h, ok := handlers[sub]
	if !ok {
		return Usagef("unknown persona action %q — run: huddle persona --help", sub)
	}
	for _, a := range rest {
		if a == "--help" || a == "-h" {
			_, err := io.WriteString(c.Stdout, personaHelp)
			return err
		}
	}
	return h(c, rest)
}

// personaArgs is a flag parser allowing flags and positionals in any order.
type personaArgs struct {
	vals  map[string]string
	bools map[string]bool
	pos   []string
}

func parsePersonaArgs(args []string, valueFlags, boolFlags string) (personaArgs, error) {
	isVal := map[string]bool{}
	for _, f := range strings.Fields(valueFlags) {
		isVal[f] = true
	}
	isBool := map[string]bool{}
	for _, f := range strings.Fields(boolFlags) {
		isBool[f] = true
	}
	out := personaArgs{vals: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out.pos = append(out.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "--") {
			out.pos = append(out.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		switch {
		case isBool[name] && !hasVal:
			out.bools[name] = true
		case isVal[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return out, Usagef("--%s needs a value — run: huddle persona --help", name)
				}
				i++
				val = args[i]
			}
			out.vals[name] = val
		default:
			return out, Usagef("unknown flag --%s — run: huddle persona --help", name)
		}
	}
	return out, nil
}

func personaRepoRoot(c *Ctx) string {
	info, err := c.Repo()
	if err != nil {
		return ""
	}
	return info.Root
}

func personaRoster(c *Ctx) (*personas.Roster, error) {
	return personas.ResolveDefault(personaRepoRoot(c))
}

func personaList(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "layer", "json")
	if err != nil {
		return err
	}
	var roster *personas.Roster
	if layer := a.vals["layer"]; layer != "" {
		var src []personas.Source
		for _, s := range personas.Sources(personaRepoRoot(c)) {
			if string(s.Layer) == layer {
				src = append(src, s)
			}
		}
		if len(src) == 0 {
			return Usagef("--layer must be builtin, global or repo, got %q", layer)
		}
		roster, err = personas.Resolve(src)
	} else {
		roster, err = personaRoster(c)
	}
	if err != nil {
		return err
	}
	if a.bools["json"] {
		type row struct {
			ID          string           `json:"id"`
			Icon        string           `json:"icon,omitempty"`
			DisplayName string           `json:"displayName,omitempty"`
			Title       string           `json:"title,omitempty"`
			Domains     []string         `json:"domains,omitempty"`
			Selectable  bool             `json:"selectable"`
			Silent      bool             `json:"silent,omitempty"`
			Layers      []personas.Layer `json:"layers"`
			Knowledge   int              `json:"knowledge"`
		}
		rows := []row{}
		for _, p := range roster.Personas {
			rows = append(rows, row{p.ID, p.Icon, p.DisplayName, p.Title, p.Domains, p.Selectable, p.Silent, p.Layers, len(p.Knowledge)})
		}
		return output.JSON(c.Stdout, map[string]any{"personas": rows, "overflow": roster.Overflow})
	}
	rows := [][]string{}
	for _, p := range roster.Personas {
		sel := "yes"
		if !p.Selectable {
			sel = "no"
		}
		rows = append(rows, []string{p.ID, p.Icon, p.DisplayName, p.Title, personaLayers(p.Layers), sel, fmt.Sprint(len(p.Knowledge))})
	}
	output.Table(c.Stdout, []string{"ID", "ICON", "NAME", "TITLE", "LAYERS", "SELECTABLE", "KNOWLEDGE"}, rows)
	for _, o := range roster.Overflow {
		fmt.Fprintln(c.Stderr, "warning:", o)
	}
	return nil
}

func personaLayers(ls []personas.Layer) string {
	s := make([]string, len(ls))
	for i, l := range ls {
		s[i] = string(l)
	}
	return strings.Join(s, ",")
}

func personaResolve(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "", "explain json")
	if err != nil {
		return err
	}
	roster, err := personaRoster(c)
	if err != nil {
		return err
	}
	list := roster.Personas
	if len(a.pos) > 0 {
		p, ok := roster.Get(a.pos[0])
		if !ok {
			return fmt.Errorf("no persona %q in any layer — run: huddle persona list", a.pos[0])
		}
		list = []*personas.Persona{p}
	}
	if !a.bools["explain"] {
		stripped := make([]personas.Persona, len(list))
		for i, p := range list {
			stripped[i] = *p
			stripped[i].Provenance = nil
		}
		if a.bools["json"] {
			return output.JSON(c.Stdout, map[string]any{"personas": stripped, "overflow": roster.Overflow})
		}
	} else if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"personas": list, "overflow": roster.Overflow})
	}
	for _, p := range list {
		fmt.Fprintf(c.Stdout, "%s %s — %s (%s) layers=%s selectable=%v\n", p.Icon, p.ID, p.DisplayName, p.Title, personaLayers(p.Layers), p.Selectable)
		if p.Definition != nil {
			fmt.Fprintf(c.Stdout, "  definition: %s [%s]\n", p.Definition.Path, p.Definition.Layer)
		}
		if a.bools["explain"] {
			keys := []string{"name", "displayName", "title", "icon", "role", "domains", "capabilities", "identity",
				"primaryLens", "communicationStyle", "principles", "selectable", "body"}
			var extra []string
			for k := range p.Provenance {
				if !personaContains(keys, k) {
					extra = append(extra, k)
				}
			}
			sort.Strings(extra)
			keys = append(keys, extra...)
			for _, k := range keys {
				if l, ok := p.Provenance[k]; ok {
					fmt.Fprintf(c.Stdout, "  %-20s ← %s\n", k, l)
				}
			}
		}
		for _, k := range p.Knowledge {
			fmt.Fprintf(c.Stdout, "  %-9s %s — %s [%s]\n", k.Kind, k.Rel, k.Title, k.Layer)
		}
		for _, prob := range p.Problems {
			fmt.Fprintf(c.Stdout, "  problem: %s\n", prob)
		}
	}
	for _, o := range roster.Overflow {
		fmt.Fprintln(c.Stderr, "warning:", o)
	}
	return nil
}

func personaContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func personaStyle(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "for", "json md")
	if err != nil {
		return err
	}
	if len(a.pos) != 1 {
		return Usagef("style needs one persona id — run: huddle persona style <id>")
	}
	roster, err := personaRoster(c)
	if err != nil {
		return err
	}
	p, ok := roster.Get(a.pos[0])
	if !ok {
		return fmt.Errorf("no persona %q in any layer — run: huddle persona list", a.pos[0])
	}
	card, err := personas.Style(p, a.vals["for"])
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, card)
	}
	_, err = io.WriteString(c.Stdout, card.Markdown())
	return err
}

func personaBrief(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "task for", "json")
	if err != nil {
		return err
	}
	if len(a.pos) != 1 || strings.TrimSpace(a.vals["task"]) == "" {
		return Usagef(`brief needs ids and a task — run: huddle persona brief shaama,nina --task "review this diff"`)
	}
	var ids []string
	for _, id := range strings.Split(a.pos[0], ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	roster, err := personaRoster(c)
	if err != nil {
		return err
	}
	br, err := personas.MakeBrief(roster, ids, a.vals["task"], a.vals["for"], personaRepoRoot(c))
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, br)
	}
	_, err = io.WriteString(c.Stdout, br.Markdown())
	return err
}

func personaFind(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "", "json")
	if err != nil {
		return err
	}
	query := strings.Join(a.pos, " ")
	if strings.TrimSpace(query) == "" {
		return Usagef(`find needs a query — run: huddle persona find "payments reviewer"`)
	}
	roster, err := personaRoster(c)
	if err != nil {
		return err
	}
	matches := personas.Find(roster, query)
	if a.bools["json"] {
		if matches == nil {
			matches = []personas.Match{}
		}
		return output.JSON(c.Stdout, map[string]any{"query": query, "matches": matches})
	}
	if len(matches) == 0 {
		fmt.Fprintln(c.Stdout, "no matching personas")
		return nil
	}
	rows := [][]string{}
	for _, m := range matches {
		rows = append(rows, []string{m.ID, m.DisplayName, m.Title, fmt.Sprint(m.Score), strings.Join(m.Matched, ",")})
	}
	output.Table(c.Stdout, []string{"ID", "NAME", "TITLE", "SCORE", "MATCHED"}, rows)
	return nil
}

func personaNew(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "scope from", "template dry-run force json")
	if err != nil {
		return err
	}
	if len(a.pos) != 1 {
		return Usagef("new needs one id — run: huddle persona new <id> --scope global|repo --from draft.md")
	}
	id := a.pos[0]
	if err := personas.ValidID(id); err != nil {
		return err
	}
	if a.vals["from"] == "" && !a.bools["template"] {
		return Usagef("new needs --from <file> or --template")
	}
	root := personaRepoRoot(c)
	dir, err := personas.ScopeDir(a.vals["scope"], root, id)
	if err != nil {
		return Usagef("%v", err)
	}
	target := filepath.Join(dir, id+".md")
	var draft []byte
	if from := a.vals["from"]; from != "" {
		if draft, err = personaReadInput(from); err != nil {
			return err
		}
		issues := personas.LintDefinition(id, from, draft)
		for _, is := range issues {
			fmt.Fprintln(c.Stderr, is.String())
		}
		if personas.HasErrors(issues) && !a.bools["template"] {
			return fmt.Errorf("%s is not a valid persona definition (nothing written) — fix the errors above, or rerun with --template to write a scaffold pre-filled with what parsed", from)
		}
		if !personas.HasErrors(issues) {
			return personaWrite(c, a, id, target, draft, "definition")
		}
	}
	scaffold := []byte(personas.Template(id, draft))
	if err := personaWrite(c, a, id, target, scaffold, "template"); err != nil {
		return err
	}
	if !a.bools["dry-run"] {
		fmt.Fprintf(c.Stdout, "next: fill every TODO, then run: huddle persona lint %s\n", id)
	}
	return nil
}

func personaWrite(c *Ctx, a personaArgs, id, target string, data []byte, kind string) error {
	scope := a.vals["scope"]
	if a.bools["dry-run"] {
		fmt.Fprintf(c.Stdout, "dry-run: would write %s (%d bytes, %s scope)\n", target, len(data), scope)
		_, err := c.Stdout.Write(data)
		return err
	}
	if err := personas.WriteFile(target, data, scope == string(personas.Global), a.bools["force"]); err != nil {
		return err
	}
	personaLog(c, "persona.written", id, map[string]any{"id": id, "scope": scope, "path": target, "kind": kind})
	output.KV(c.Stdout, "WROTE", target, "SCOPE", scope)
	if scope == string(personas.Repo) {
		fmt.Fprintln(c.Stdout, "note: repo-scope persona files are committed with the code — review before committing")
	}
	return nil
}

func personaImport(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "scope id", "dry-run force")
	if err != nil {
		return err
	}
	if len(a.pos) != 1 || a.vals["id"] == "" {
		return Usagef("import needs a file (or -) and --id — run: huddle persona import agent.md --scope repo --id rahul-sre")
	}
	id := a.vals["id"]
	if err := personas.ValidID(id); err != nil {
		return err
	}
	dir, err := personas.ScopeDir(a.vals["scope"], personaRepoRoot(c), id)
	if err != nil {
		return Usagef("%v", err)
	}
	raw, err := personaReadInput(a.pos[0])
	if err != nil {
		return err
	}
	target := personas.ImportPath(dir)
	data := personas.ImportRecord(a.pos[0], raw, time.Now())
	if err := personaWrite(c, a, id, target, data, "import"); err != nil {
		return err
	}
	fmt.Fprintf(c.Stdout, "next: have your agent rewrite %s into the built-in persona style (see: huddle persona style shaama), then run: huddle persona new %s --scope %s --from <rewritten.md>\n",
		target, id, a.vals["scope"])
	return nil
}

func personaIngest(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "file label", "schema json")
	if err != nil {
		return err
	}
	if a.bools["schema"] {
		_, err := io.WriteString(c.Stdout, personas.EvidenceSchema)
		return err
	}
	if len(a.pos) != 1 {
		return Usagef("ingest needs one persona id — run: huddle persona ingest <id> --file dump.ndjson")
	}
	name := a.vals["file"]
	if name == "" {
		name = "-"
	}
	data, err := personaReadInput(name)
	if err != nil {
		return err
	}
	res, err := personas.Ingest(a.pos[0], name, data, a.vals["label"], time.Now())
	if err != nil {
		return err
	}
	personaLog(c, "persona.ingested", res.ID, map[string]any{"id": res.ID, "records": res.Records, "format": res.Format, "label": a.vals["label"]})
	if a.bools["json"] {
		return output.JSON(c.Stdout, res)
	}
	output.KV(c.Stdout, "INGESTED", fmt.Sprint(res.Records), "FORMAT", res.Format, "PATH", res.Path)
	return nil
}

func personaLint(c *Ctx, args []string) error {
	a, err := parsePersonaArgs(args, "", "json")
	if err != nil {
		return err
	}
	roster, err := personaRoster(c)
	if err != nil {
		return err
	}
	id := ""
	if len(a.pos) > 0 {
		id = a.pos[0]
	}
	issues, err := roster.Lint(id)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		if issues == nil {
			issues = []personas.Issue{}
		}
		if err := output.JSON(c.Stdout, map[string]any{"ok": !personas.HasErrors(issues), "issues": issues}); err != nil {
			return err
		}
	} else {
		for _, is := range issues {
			if is.Severity != personas.SevWaived {
				fmt.Fprintln(c.Stdout, is.String())
			}
		}
	}
	if personas.HasErrors(issues) {
		return fmt.Errorf("persona lint failed")
	}
	if !a.bools["json"] {
		n := len(roster.Personas)
		if id != "" {
			n = 1
		}
		waived := 0
		for _, is := range issues {
			if is.Severity == personas.SevWaived {
				waived++
			}
		}
		fmt.Fprintf(c.Stdout, "ok: %d persona(s) pass lint (%d built-in worker sections waived; --json lists them)\n", n, waived)
	}
	return nil
}

func personaReadInput(name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(io.LimitReader(personaStdin, 32<<20))
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", name, err)
	}
	return b, nil
}

// personaLog appends an audit event; logging never fails the command.
func personaLog(c *Ctx, typ, id string, data map[string]any) {
	info, err := c.Repo()
	if err != nil {
		return
	}
	_, _ = eventlog.Append(info.Name, eventlog.Event{Type: typ, Branch: info.Branch, Refs: []string{id}, Data: data})
}
