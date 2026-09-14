package artifacts

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/muthuishere/huddle/assets"
)

// CIDrafts maps a draft name to its template and default workflow path.
var CIDrafts = map[string]struct{ Template, Path string }{
	"eval":   {"ci-eval.yml", ".github/workflows/agent-evals.yml"},
	"triage": {"ci-triage.yml", ".github/workflows/agent-failure-triage.yml"},
	"spec":   {"ci-spec.yml", ".github/workflows/intent-spec-draft.yml"},
}

// RenderCI fills a CI workflow template. vars keys are {{name}} placeholders;
// GitHub's own ${{ … }} expressions are left alone. A multi-line value is
// indented to match the placeholder's column.
func RenderCI(name string, vars map[string]string) (string, error) {
	d, ok := CIDrafts[name]
	if !ok {
		return "", fmt.Errorf("unknown CI draft %q — choose eval, triage, or spec", name)
	}
	b, err := fs.ReadFile(assets.Templates, "templates/"+d.Template)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		for k, v := range vars {
			ph := "{{" + k + "}}"
			j := strings.Index(line, ph)
			if j < 0 || (j > 0 && line[j-1] == '$') {
				continue
			}
			indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " ")))
			v = strings.ReplaceAll(strings.TrimRight(v, "\n"), "\n", "\n"+indent)
			line = strings.ReplaceAll(line, ph, v)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n"), nil
}
