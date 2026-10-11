package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/specframework"
)

// fakeOpenspecDir writes an openspec/ layout without needing openspec
// installed, and empties PATH so no real binary is ever executed by a test.
func fakeOpenspecDir(t *testing.T, root string) {
	t.Helper()
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o700); err != nil {
		t.Fatal(err)
	}
	artWrite(t, filepath.Join(root, "openspec", "config.yaml"), "schema: spec-driven\n")
}

// Doctor always reports the framework, never as a warning or failure.
func TestDoctorSpecFrameworkIsInformational(t *testing.T) {
	root := coreEnv(t)
	t.Setenv("CLAUDE_SKILLS_DIR", t.TempDir())
	find := func() DoctorCheck {
		t.Helper()
		out, err := runCore(t, root, "doctor", "--json")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		var res struct {
			Checks []DoctorCheck `json:"checks"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err, out)
		}
		for _, c := range res.Checks {
			if c.Name == "spec-framework" {
				return c
			}
		}
		t.Fatal("no spec-framework check")
		return DoctorCheck{}
	}
	absent := find()
	if absent.Status != "ok" || !strings.Contains(absent.Detail, "native") {
		t.Fatalf("absent: %+v", absent)
	}
	fakeOpenspecDir(t, root)
	present := find()
	if present.Status != "ok" || !strings.Contains(present.Detail, "openspec") {
		t.Fatalf("present: %+v", present)
	}
}

// sdlc status says nothing about spec frameworks when none is there, and
// reports the directory (without execing anything) when one is.
func TestSdlcStatusReportsSpecFramework(t *testing.T) {
	root := artTestEnv(t)
	out := artMust(t, root, "sdlc", "status")
	if strings.Contains(out, "SPEC FRAMEWORK") {
		t.Fatalf("absent framework must stay out of the output:\n%s", out)
	}
	var r struct {
		SpecFramework *specframework.Detection `json:"specFramework"`
		Changes       []specframework.Change   `json:"specFrameworkChanges"`
	}
	if err := json.Unmarshal([]byte(artMust(t, root, "sdlc", "status", "--json")), &r); err != nil {
		t.Fatal(err)
	}
	if r.SpecFramework != nil || r.Changes != nil {
		t.Fatalf("absent: %+v %+v", r.SpecFramework, r.Changes)
	}

	fakeOpenspecDir(t, root)
	out = artMust(t, root, "sdlc", "status")
	if !strings.Contains(out, "SPEC FRAMEWORK: openspec/ present") {
		t.Fatalf("present framework not reported:\n%s", out)
	}
	if err := json.Unmarshal([]byte(artMust(t, root, "sdlc", "status", "--json")), &r); err != nil {
		t.Fatal(err)
	}
	if r.SpecFramework == nil || r.SpecFramework.Name != "openspec" || r.SpecFramework.Dir != "openspec" {
		t.Fatalf("detection: %+v", r.SpecFramework)
	}
	if r.SpecFramework.CLI != "" || r.Changes != nil {
		t.Fatalf("no CLI on PATH, so nothing may be read: %+v %+v", r.SpecFramework, r.Changes)
	}
	// The native chain still drives everything.
	if !strings.Contains(out, "NEXT:") {
		t.Fatalf("native next action lost:\n%s", out)
	}
}
