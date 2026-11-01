package personas

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/redact"
)

// redactString is the redaction choke point; a var so tests can observe calls.
var redactString = redact.String

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidID checks a persona id (the directory name).
func ValidID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid persona id %q: use lowercase letters, digits, '-' or '_' (max 64), e.g. rahul-sre", id)
	}
	return nil
}

// ScopeDir returns the persona directory for id at scope global|repo.
func ScopeDir(scope, repoRoot, id string) (string, error) {
	switch scope {
	case string(Global):
		return filepath.Join(paths.GlobalPersonas(), id), nil
	case string(Repo):
		if repoRoot == "" {
			return "", errors.New("--scope repo needs a repository")
		}
		return filepath.Join(paths.RepoPersonas(repoRoot), id), nil
	}
	return "", fmt.Errorf("--scope must be global or repo, got %q", scope)
}

// Template returns the scaffold for a new persona, pre-filled with any values
// parsed from draft (may be nil).
func Template(id string, draft []byte) string {
	fm := ParseFrontMatter(draft)
	secs := map[string]string{}
	if fm.Present && fm.BodyAt <= len(draft) {
		secs = Sections(string(draft[fm.BodyAt:]))
	}
	val := func(key, placeholder string) string {
		if v, ok := fm.Fields[key]; ok && v.String() != "" {
			if v.IsList {
				return "[" + strings.Join(v.List, ", ") + "]"
			}
			return quoteYAML(v.Str)
		}
		return placeholder
	}
	if id == "" {
		id = "persona"
	}
	display := strings.ToUpper(id[:1]) + id[1:]
	if i := strings.IndexAny(display, "-_"); i > 0 {
		display = display[:i]
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", val("name", "huddle-"+id))
	fmt.Fprintf(&b, "displayName: %s\n", val("displayName", display))
	fmt.Fprintf(&b, "title: %s\n", val("title", `"TODO: job title"`))
	fmt.Fprintf(&b, "icon: %s\n", val("icon", `"🧩"`))
	fmt.Fprintf(&b, "role: %s\n", val("role", `"TODO: one line — what they own"`))
	fmt.Fprintf(&b, "domains: %s\n", val("domains", "[todo-domain, another-domain]"))
	fmt.Fprintf(&b, "capabilities: %s\n", val("capabilities", `"TODO: comma list of what they are good at"`))
	fmt.Fprintf(&b, "identity: %s\n", val("identity", `"TODO: background — their win and their scar"`))
	fmt.Fprintf(&b, "primaryLens: %s\n", val("primaryLens", `"TODO: the one question they always ask"`))
	fmt.Fprintf(&b, "communicationStyle: %s\n", val("communicationStyle", `"TODO: how they speak and push back"`))
	fmt.Fprintf(&b, "principles: %s\n", val("principles", `"TODO: short declarative beliefs"`))
	fmt.Fprintf(&b, "source: huddle\n")
	b.WriteString("---\n")
	section := func(title, placeholder string) {
		body := strings.TrimSpace(secs[title])
		if body == "" {
			body = placeholder
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", title, body)
	}
	section("Signature Phrases", "- \"TODO: a phrase they say\"")
	section("Common Disagreements", "- With <persona>: \"TODO\"")
	section("Expertise Areas", "TODO")
	section("Voices "+strings.Trim(val("displayName", display), `"`)+" Has Absorbed", "- **TODO** — book or person that shaped them")
	section("Non-Goals", "TODO")
	section("Blind Spots", "TODO")
	section("When Useful", "TODO")
	return b.String()
}

func quoteYAML(s string) string {
	if strings.ContainsAny(s, ":#[]{},\"'") || strings.TrimSpace(s) != s {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// WriteFile writes data to path (0644 for team-visible persona files, dirs 0755
// in repo scope; 0600/0700 in global scope). It refuses to overwrite unless force.
func WriteFile(path string, data []byte, private, force bool) error {
	dirMode, fileMode := os.FileMode(0o755), os.FileMode(0o644)
	if private {
		dirMode, fileMode = 0o700, 0o600
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if !force {
		flags = os.O_CREATE | os.O_WRONLY | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, fileMode)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists — pass --force to overwrite", path)
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ImportPath is where `persona import` stores the raw definition.
func ImportPath(dir string) string { return filepath.Join(dir, "source", "imported.md") }

// ImportRecord prefixes the raw text with provenance so the rewrite knows its origin.
func ImportRecord(ref string, raw []byte, now time.Time) []byte {
	head := fmt.Sprintf("<!-- importedFrom: %s importedAt: %s — raw source; rewrite into built-in persona style, then run: huddle persona new --from -->\n",
		ref, now.UTC().Format(time.RFC3339))
	return append([]byte(head), redactString(string(raw))...)
}
