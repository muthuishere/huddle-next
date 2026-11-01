package artifacts_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/muthuishere/huddle-next/artifacts"
)

// A host writes an ADR the huddle CLI can then read, lint and link. The zero
// Config means "no overrides", so no huddle installation has to exist.
func TestExampleWriteADR(t *testing.T) {
	root := t.TempDir()
	var cfg artifacts.Config

	d, err := artifacts.Render(root, cfg, "adr", artifacts.NewOptions{
		Author: "karya",
		Title:  "Embed the room in the host",
		Now:    time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := artifacts.Create(root, d); err != nil {
		t.Fatal(err)
	}

	// Render is dry: Create is the only thing that touched disk, and a second
	// Create refuses rather than overwriting what a human may have edited.
	if err := artifacts.Create(root, d); !errors.Is(err, artifacts.ErrExists) {
		t.Fatalf("second Create: want ErrExists, got %v", err)
	}

	found, err := artifacts.Discover(root, cfg, "adr")
	if err != nil || len(found) != 1 {
		t.Fatalf("Discover: %v %d", err, len(found))
	}
	if filepath.Base(found[0].Path) != filepath.Base(d.Path) {
		t.Fatalf("discovered %s, wrote %s", found[0].Path, d.Path)
	}

	// Lint is structural: it checks that the required headings are present,
	// not that anyone filled them in. A freshly rendered template therefore
	// passes clean. This is pinned deliberately — an embedding host must NOT
	// read "lint ok" as "this decision has content", and if the structural
	// lint ever grows an emptiness check this test is where that shows up.
	k, ok := artifacts.Get(cfg, "adr")
	if !ok {
		t.Fatal("adr kind missing")
	}
	if n := len(artifacts.Lint(found[0], k)); n != 0 {
		t.Fatalf("unfilled template now reports %d findings — lint gained an emptiness check; update the host contract", n)
	}
}
