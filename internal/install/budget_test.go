package install

import (
	"bufio"
	"bytes"
	"io/fs"
	"path"
	"strings"
	"testing"
)

// Byte budgets (ADR 13 §C.15, ADR 19 value gate). A budget may go down in a PR;
// raising one needs an explicit override line here with a reason.
const (
	skillMDBudget     = 6 * 1024
	descriptionBudget = 260
	lazyFileBudget    = 8 * 1024
	routerLineBudget  = 150
)

// requiredLazy lists the lazy files each skill must ship (ADR 15 §D, ADR 16).
var requiredLazy = map[string][]string{
	"huddle":          {"modes/discussion.md", "modes/verification.md", "modes/wrap-up.md"},
	"huddle-sdlc":     {"stages/setup.md", "stages/plan.md", "stages/design.md", "stages/build.md", "stages/test.md", "stages/deploy.md", "stages/maintain.md"},
	"huddle-personas": {"flows/create.md", "flows/import-ingest.md", "flows/style-brief.md"},
}

func TestSkillByteBudgets(t *testing.T) {
	fsys := Skills()
	names, err := SkillNames(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		b, err := fs.ReadFile(fsys, path.Join(name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > skillMDBudget {
			t.Errorf("%s/SKILL.md is %d bytes, budget %d", name, len(b), skillMDBudget)
		}
		if n := bytes.Count(b, []byte("\n")); n > routerLineBudget {
			t.Errorf("%s/SKILL.md has %d lines, budget %d", name, n, routerLineBudget)
		}
		fm := frontMatter(t, name, b)
		if fm["name"] != name {
			t.Errorf("%s/SKILL.md front matter name = %q", name, fm["name"])
		}
		if d := fm["description"]; d == "" || len(d) > descriptionBudget {
			t.Errorf("%s description is %d bytes, budget %d", name, len(d), descriptionBudget)
		}
		if !bytes.Contains(b, []byte("huddle start")) || !bytes.Contains(b, []byte("HUDDLE_PROTO")) {
			t.Errorf("%s/SKILL.md must call `huddle start` and branch on HUDDLE_PROTO", name)
		}
		for _, lazy := range requiredLazy[name] {
			if _, err := fs.Stat(fsys, path.Join(name, lazy)); err != nil {
				t.Errorf("%s missing lazy file %s", name, lazy)
			}
		}
		_ = fs.WalkDir(fsys, name, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || p == path.Join(name, "SKILL.md") {
				return err
			}
			if info, _ := d.Info(); info != nil && info.Size() > lazyFileBudget {
				t.Errorf("%s is %d bytes, budget %d", p, info.Size(), lazyFileBudget)
			}
			return nil
		})
	}
	for name := range requiredLazy {
		if _, err := fs.Stat(fsys, path.Join(name, "SKILL.md")); err != nil {
			t.Errorf("skill %s missing", name)
		}
	}
}

// frontMatter parses the simple `key: value` YAML header (single-line values).
func frontMatter(t *testing.T, name string, b []byte) map[string]string {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(b))
	if !sc.Scan() || sc.Text() != "---" {
		t.Fatalf("%s/SKILL.md has no front matter", name)
	}
	out := map[string]string{}
	for sc.Scan() {
		line := sc.Text()
		if line == "---" {
			return out
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s/SKILL.md front matter line %q: use single-line key: value", name, line)
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[strings.TrimSpace(k)] = v
	}
	t.Fatalf("%s/SKILL.md front matter not closed", name)
	return nil
}
