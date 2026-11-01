package personas_test

import (
	"fmt"

	"github.com/muthuishere/huddle-next/personas"
)

// An embedding host resolves the built-in roster with no disk access at all:
// BuiltinSource is backed by go:embed, so this works in a container with no
// HOME, no ~/.config/.huddle and no repo checked out.
func ExampleResolve() {
	r, err := personas.Resolve([]personas.Source{personas.BuiltinSource()})
	if err != nil {
		panic(err)
	}
	p, ok := r.Get("elanchezian")
	if !ok {
		panic("elanchezian missing from the built-in roster")
	}
	card, err := personas.Style(p, "review")
	if err != nil {
		panic(err)
	}
	fmt.Println(p.ID, "|", card.PrimaryLens != "")
	// Output: elanchezian | true
}

// Resolve layers builtin < global < repo. A host that wants the user's own
// overrides passes Sources(repoRoot) instead; a host that wants only what it
// ships passes BuiltinSource alone, as above.
func ExampleMakeBrief() {
	r, err := personas.Resolve([]personas.Source{personas.BuiltinSource()})
	if err != nil {
		panic(err)
	}
	b, err := personas.MakeBrief(r, []string{"elanchezian"}, "Pick a lead-gen channel", "review", "")
	if err != nil {
		panic(err)
	}
	fmt.Println(len(b.Cards))
	// Output: 1
}
