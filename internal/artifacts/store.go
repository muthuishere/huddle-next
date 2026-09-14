package artifacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle/assets"
	"github.com/muthuishere/huddle/internal/config"
)

// Artifact is one discovered file. Status, Author and Links come from the file.
type Artifact struct {
	ID     string              `json:"id"`
	Kind   string              `json:"kind"`
	Slug   string              `json:"slug,omitempty"`
	N      int                 `json:"n,omitempty"`
	Path   string              `json:"path"` // repo-relative, slash separated
	Status string              `json:"status"`
	Author string              `json:"author,omitempty"`
	Title  string              `json:"title,omitempty"`
	Links  map[string][]string `json:"links,omitempty"`
	SHA256 string              `json:"sha256"`

	Front FrontMatter `json:"-"`
	Body  []byte      `json:"-"`
	Raw   []byte      `json:"-"`
}

// Statuses allowed in front matter.
var Statuses = []string{"draft", "proposed", "approved", "superseded"}

// SHA256 hashes content as lowercase hex.
func SHA256(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type matcher struct {
	glob string
	re   *regexp.Regexp
}

func compile(pattern string) matcher {
	glob := pattern
	for _, ph := range []string{"{n}-{slug}", "{date}-{slug}", "{n}", "{slug}", "{date}"} {
		glob = strings.ReplaceAll(glob, ph, "*")
	}
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\{n\}`, `(?P<n>\d+)`)
	re = strings.ReplaceAll(re, `\{date\}`, `(?P<date>\d{4}-\d{2}-\d{2})`)
	re = strings.ReplaceAll(re, `\{slug\}`, `(?P<slug>[^/]+?)`)
	return matcher{glob: glob, re: regexp.MustCompile("^" + re + "$")}
}

// Discover lists artifacts of the given kinds (all kinds when empty), sorted by path.
func Discover(root string, cfg config.Config, names ...string) ([]Artifact, error) {
	if len(names) == 0 {
		names = Names()
	}
	var out []Artifact
	seen := map[string]bool{}
	for _, name := range names {
		k, ok := Get(cfg, name)
		if !ok {
			return nil, fmt.Errorf("unknown artifact kind %q — kinds: %s", name, strings.Join(Names(), ", "))
		}
		m := compile(k.Pattern)
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(m.glob)))
		if err != nil {
			return nil, err
		}
		sort.Strings(matches)
		for _, abs := range matches {
			rel, _ := filepath.Rel(root, abs)
			rel = filepath.ToSlash(rel)
			if seen[name+"\x00"+rel] || !m.re.MatchString(rel) {
				continue
			}
			seen[name+"\x00"+rel] = true
			a, err := Load(root, k, rel)
			if err != nil {
				continue // unreadable file: skip, never fatal for listing
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// Load reads one artifact file of kind k at rel.
func Load(root string, k Kind, rel string) (Artifact, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return Artifact{}, err
	}
	a := Artifact{Kind: k.Name, Path: rel, Raw: raw, SHA256: SHA256(raw)}
	m := compile(k.Pattern)
	if sub := m.re.FindStringSubmatch(rel); sub != nil {
		for i, name := range m.re.SubexpNames() {
			switch name {
			case "slug":
				a.Slug = sub[i]
			case "n":
				a.N, _ = strconv.Atoi(sub[i])
			}
		}
	}
	switch k.Format {
	case FormatMD:
		a.Front, a.Body = SplitFrontMatter(raw)
		a.ID = a.Front.Get("id")
		a.Status = strings.ToLower(a.Front.Get("status"))
		a.Author = a.Front.Get("author")
		a.Links = a.Front.Links()
		if a.Status == "" {
			a.Status = bodyStatus(a.Body)
		}
		a.Title = firstH1(a.Body)
	case FormatJSON:
		var doc map[string]any
		if json.Unmarshal(raw, &doc) == nil {
			a.ID, _ = doc["id"].(string)
			a.Status, _ = doc["status"].(string)
			a.Author, _ = doc["author"].(string)
			a.Title, _ = doc["prompt"].(string)
		}
	case FormatYAML:
		if doc, err := ParseYAML(raw); err == nil {
			a.ID, _ = doc["id"].(string)
			a.Status, _ = doc["status"].(string)
			a.Author, _ = doc["author"].(string)
			a.Title, _ = doc["metric"].(string)
		}
	}
	if a.ID == "" {
		a.ID = DefaultID(k, a.Slug, a.N)
	}
	return a, nil
}

// DefaultID derives the id used when the file does not declare one.
func DefaultID(k Kind, slug string, n int) string {
	switch {
	case k.Singleton():
		return k.Name
	case k.Numbered() && n > 0:
		return k.Name + "-" + strconv.Itoa(n)
	default:
		return k.Name + "-" + slug
	}
}

var statusLine = regexp.MustCompile(`(?mi)^\s*[-*]?\s*\**status\**\s*:\s*\**\s*([a-z]+)`)

func bodyStatus(body []byte) string {
	if m := statusLine.FindSubmatch(body); m != nil {
		return strings.ToLower(string(m[1]))
	}
	return ""
}

func firstH1(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if t, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// Resolve finds an artifact by id, slug, number, or path. kind may be "" (any kind).
func Resolve(root string, cfg config.Config, kind, ref string) (Artifact, error) {
	var names []string
	if kind != "" {
		names = []string{kind}
	}
	all, err := Discover(root, cfg, names...)
	if err != nil {
		return Artifact{}, err
	}
	ref = strings.TrimSpace(ref)
	var hits []Artifact
	for _, pass := range []func(a Artifact) bool{
		func(a Artifact) bool { return a.ID == ref || a.Path == filepath.ToSlash(ref) },
		func(a Artifact) bool { return a.Slug != "" && a.Slug == ref },
		func(a Artifact) bool { return a.N > 0 && strconv.Itoa(a.N) == ref },
		func(a Artifact) bool { return ref == "" && len(all) == 1 },
	} {
		for _, a := range all {
			if pass(a) {
				hits = append(hits, a)
			}
		}
		if len(hits) > 0 {
			break
		}
	}
	switch len(hits) {
	case 0:
		noun := kind
		if noun == "" {
			noun = "<kind>"
		}
		return Artifact{}, fmt.Errorf("no artifact %q — run: huddle %s list", ref, noun)
	case 1:
		return hits[0], nil
	}
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.ID
	}
	return Artifact{}, fmt.Errorf("%q is ambiguous (%s) — pass the full id", ref, strings.Join(ids, ", "))
}

// NewOptions controls New.
type NewOptions struct {
	Author string
	Title  string // slug or title given by the user
	Now    time.Time
}

// Draft is a rendered, not yet written artifact.
type Draft struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Path    string `json:"path"`
	Content string `json:"-"`
	SHA256  string `json:"sha256"`
}

// ErrExists is returned when New would overwrite a file.
var ErrExists = errors.New("artifact already exists")

// Render builds the file New would write, without touching disk.
func Render(root string, cfg config.Config, kindName string, o NewOptions) (Draft, error) {
	k, ok := Get(cfg, kindName)
	if !ok {
		return Draft{}, fmt.Errorf("unknown artifact kind %q", kindName)
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	title := strings.TrimSpace(o.Title)
	slug := Slugify(title)
	if !k.Singleton() && strings.Contains(k.Pattern, "{slug}") && slug == "" {
		return Draft{}, fmt.Errorf("missing name — run: huddle %s new <slug|\"title\">", k.Name)
	}
	if title == "" {
		title = strings.ToUpper(k.Name[:1]) + k.Name[1:]
	}
	if slug == "" {
		slug = k.Name
	}
	if title == slug {
		title = strings.ReplaceAll(slug, "-", " ")
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	date := o.Now.Format("2006-01-02")
	n := 0
	if k.Numbered() {
		var err error
		if n, err = nextNumber(root, k); err != nil {
			return Draft{}, err
		}
	}
	rel := strings.NewReplacer("{slug}", slug, "{n}", strconv.Itoa(n), "{date}", date).Replace(k.Pattern)
	id := DefaultID(k, slug, n)
	tmpl, err := fs.ReadFile(assets.Templates, "templates/"+k.Template)
	if err != nil {
		return Draft{}, fmt.Errorf("template %s missing: %w", k.Template, err)
	}
	author := o.Author
	if author == "" {
		author = "unknown"
	}
	content := strings.NewReplacer(
		"{{id}}", id, "{{title}}", escapeFor(k.Format, title), "{{slug}}", slug,
		"{{author}}", author, "{{date}}", date, "{{n}}", strconv.Itoa(n),
	).Replace(string(tmpl))
	return Draft{Kind: k.Name, ID: id, Path: rel, Content: content, SHA256: SHA256([]byte(content))}, nil
}

func escapeFor(format, s string) string {
	if format == FormatJSON {
		b, _ := json.Marshal(s)
		return string(b[1 : len(b)-1])
	}
	return strings.ReplaceAll(s, `"`, `'`)
}

func nextNumber(root string, k Kind) (int, error) {
	m := compile(k.Pattern)
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(m.glob)))
	if err != nil {
		return 0, err
	}
	// Also count numbered directories whose file is not written yet.
	dirPat := k.Pattern
	if i := strings.Index(dirPat, "{n}"); i >= 0 {
		if j := strings.Index(dirPat[i:], "/"); j >= 0 {
			dm := compile(dirPat[:i+j])
			more, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(dm.glob)))
			for _, d := range more {
				rel, _ := filepath.Rel(root, d)
				if dm.re.MatchString(filepath.ToSlash(rel)) {
					matches = append(matches, d)
				}
			}
		}
	}
	max := 0
	numRe := regexp.MustCompile(`^(\d+)-`)
	for _, p := range matches {
		rel, _ := filepath.Rel(root, p)
		for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
			if sm := numRe.FindStringSubmatch(seg); sm != nil {
				if v, _ := strconv.Atoi(sm[1]); v > max {
					max = v
				}
			}
		}
	}
	return max + 1, nil
}

// Create writes a draft; it never overwrites an existing file.
func Create(root string, d Draft) error {
	abs := filepath.Join(root, filepath.FromSlash(d.Path))
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("%w: %s — edit it by hand, or run: huddle %s show %s", ErrExists, d.Path, d.Kind, d.ID)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrExists, d.Path)
		}
		return err
	}
	if _, err := f.WriteString(d.Content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// WriteAtomic replaces a file via temp + rename, keeping its permissions.
func WriteAtomic(abs string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(abs); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), "."+filepath.Base(abs)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), abs)
}

// UpdateFrontMatter re-reads the file, applies mutate to its front matter only,
// and writes it atomically. The body is kept byte for byte.
func UpdateFrontMatter(root string, cfg config.Config, a Artifact, mutate func(*FrontMatter)) (Artifact, error) {
	k, ok := Get(cfg, a.Kind)
	if !ok || k.Format != FormatMD {
		return a, fmt.Errorf("%s is a %s file without front matter — edit it by hand", a.Path, a.Kind)
	}
	abs := filepath.Join(root, filepath.FromSlash(a.Path))
	raw, err := os.ReadFile(abs)
	if err != nil {
		return a, err
	}
	fm, body := SplitFrontMatter(raw)
	if !fm.Present {
		fm = FrontMatter{Present: true}
		fm.Set("id", a.ID)
		fm.Set("kind", a.Kind)
	}
	mutate(&fm)
	out := append(fm.Render(), body...)
	if bytes.Equal(out, raw) {
		return a, nil
	}
	if err := WriteAtomic(abs, out); err != nil {
		return a, err
	}
	b, err := Load(root, k, a.Path)
	if err == nil && b.Slug == "" && b.N == 0 {
		b.Slug, b.N = a.Slug, a.N
	}
	return b, err
}
