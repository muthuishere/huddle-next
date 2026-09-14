package personas

// builtinOverride carries roster flags the built-in files do not express in
// front matter (they came from persona-roster.xml). Applied at the builtin
// layer only, so a global/repo definition can still set these keys itself.
type builtinOverride struct {
	selectable bool
	silent     bool
	subtask    bool
	// waive lists required sections a built-in lacks by design (worker
	// personas with their own structure). Lint reports them as waived, not
	// as errors; the files are shipped unchanged.
	waive []string
}

var builtinOverrides = map[string]builtinOverride{
	// ADR 11 removes the builder flow: keep the files, never offer them for a room.
	"sreyash":      {selectable: false, subtask: true, waive: []string{"Signature Phrases", "Expertise Areas", "Blind Spots"}},
	"hari":         {selectable: false, subtask: true, waive: []string{"Signature Phrases", "Expertise Areas", "Blind Spots", "When Useful"}},
	"harshvardhan": {selectable: false, subtask: true, waive: []string{"Signature Phrases", "Expertise Areas", "Blind Spots", "When Useful"}},
	// Silent background state worker.
	"elango": {selectable: true, silent: true, waive: []string{"Expertise Areas", "When Useful"}},
	// Sub-task facilitator.
	"elanchezian": {selectable: true, subtask: true},
	// Vel's file uses What Vel Is / When Vel Activates / Boundaries instead.
	"vel": {selectable: true, waive: []string{"Expertise Areas", "Blind Spots", "When Useful"}},
}

func applyBuiltinOverride(p *Persona) {
	o, ok := builtinOverrides[p.ID]
	if !ok {
		return
	}
	p.Selectable = o.selectable
	p.Silent = o.silent
	p.Subtask = o.subtask
	p.Provenance["selectable"] = Builtin
}

func waivedSection(id, section string) bool {
	for _, s := range builtinOverrides[id].waive {
		if s == section {
			return true
		}
	}
	return false
}
