package personas

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/muthuishere/huddle-next/assets"
	"github.com/muthuishere/huddle-next/internal/paths"
)

// Layer names a persona layer, lowest precedence first (ADR 3).
type Layer string

const (
	Builtin Layer = "builtin"
	Global  Layer = "global"
	Repo    Layer = "repo"
)

// Caps per layer (ADR 3 §5). Overflow is reported, never silently read.
const (
	MaxDirsPerLayer  = 200
	MaxFilesPerDir   = 200
	frontMatterHead  = 8 << 10
	frontMatterLimit = 64 << 10
	knowledgeHead    = 2 << 10
)

// Source is one layer to scan: an fs rooted at the layer's personas dir.
type Source struct {
	Layer Layer
	FS    fs.FS
	Dir   string // display path ("builtin:" for the embed)
}

// Sources returns builtin, global and (when repoRoot != "") repo layers.
func Sources(repoRoot string) []Source {
	srcs := []Source{BuiltinSource(), DirSource(Global, paths.GlobalPersonas())}
	if repoRoot != "" {
		srcs = append(srcs, DirSource(Repo, paths.RepoPersonas(repoRoot)))
	}
	return srcs
}

// BuiltinSource is the embedded persona set.
func BuiltinSource() Source {
	sub, err := fs.Sub(assets.Personas, "personas")
	if err != nil {
		sub = assets.Personas
	}
	return Source{Layer: Builtin, FS: sub, Dir: "builtin:personas"}
}

// DirSource is an on-disk layer; a missing directory is an empty layer.
func DirSource(layer Layer, dir string) Source {
	return Source{Layer: layer, FS: os.DirFS(dir), Dir: dir}
}

// FileRef points at one file inside a layer.
type FileRef struct {
	Layer Layer  `json:"source"`
	Path  string `json:"path"` // absolute on disk, or builtin:personas/<id>/<file>
	Rel   string `json:"rel"`  // relative to the persona dir
	src   *Source
}

// Knowledge is one indexed markdown file other than the definition.
type Knowledge struct {
	FileRef
	Kind  string   `json:"kind"` // memory | style | example | knowledge
	Title string   `json:"title"`
	Tags  []string `json:"tags,omitempty"`
}

// Persona is the merged view of one id across layers.
type Persona struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name,omitempty"`
	DisplayName        string             `json:"displayName,omitempty"`
	Title              string             `json:"title,omitempty"`
	Icon               string             `json:"icon,omitempty"`
	Role               string             `json:"role,omitempty"`
	Domains            []string           `json:"domains,omitempty"`
	Capabilities       string             `json:"capabilities,omitempty"`
	Identity           string             `json:"identity,omitempty"`
	PrimaryLens        string             `json:"primaryLens,omitempty"`
	CommunicationStyle string             `json:"communicationStyle,omitempty"`
	Principles         string             `json:"principles,omitempty"`
	Selectable         bool               `json:"selectable"`
	Silent             bool               `json:"silent,omitempty"`
	Subtask            bool               `json:"subtask,omitempty"`
	Extra              map[string]string  `json:"extra,omitempty"`
	Definition         *FileRef           `json:"definition,omitempty"`
	Knowledge          []Knowledge        `json:"knowledge"`
	Layers             []Layer            `json:"layers"`
	Provenance         map[string]Layer   `json:"provenance,omitempty"`
	Problems           []string           `json:"problems,omitempty"`
	defs               map[Layer]*FileRef // every layer's definition, for lint
	fields             map[string]Value
}

// Roster is the resolved, merged persona set, sorted by id.
type Roster struct {
	Personas []*Persona `json:"personas"`
	Overflow []string   `json:"overflow,omitempty"`
	byID     map[string]*Persona
}

// Get returns a persona by id (case-insensitive).
func (r *Roster) Get(id string) (*Persona, bool) {
	p, ok := r.byID[strings.ToLower(id)]
	return p, ok
}

// ResolveDefault resolves builtin, global and repo layers for repoRoot.
func ResolveDefault(repoRoot string) (*Roster, error) { return Resolve(Sources(repoRoot)) }

// Resolve scans every source in order and merges them deterministically.
// Only front matter is read; bodies of definitions and knowledge are not.
func Resolve(srcs []Source) (*Roster, error) {
	r := &Roster{byID: map[string]*Persona{}}
	for i := range srcs {
		src := &srcs[i]
		if err := r.scanLayer(src); err != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := r.byID[id]
		sort.SliceStable(p.Knowledge, func(a, b int) bool {
			la, lb := layerRank(p.Knowledge[a].Layer), layerRank(p.Knowledge[b].Layer)
			if la != lb {
				return la < lb
			}
			return p.Knowledge[a].Rel < p.Knowledge[b].Rel
		})
		r.Personas = append(r.Personas, p)
	}
	return r, nil
}

func layerRank(l Layer) int {
	switch l {
	case Builtin:
		return 0
	case Global:
		return 1
	case Repo:
		return 2
	}
	return 3
}

func (r *Roster) scanLayer(src *Source) error {
	entries, err := fs.ReadDir(src.FS, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			return nil
		}
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return nil // unreadable layer dir behaves as empty
		}
		return err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !isDir(src.FS, e) {
			continue
		}
		if len(names) >= MaxDirsPerLayer {
			r.Overflow = append(r.Overflow, fmt.Sprintf("%s layer: more than %d persona dirs in %s; %q and later skipped", src.Layer, MaxDirsPerLayer, src.Dir, name))
			break
		}
		names = append(names, name)
	}
	// Scanning is I/O-bound (open/readdir syscalls): fan out, then merge in
	// directory order so the result stays deterministic.
	scans := make([]dirScan, len(names))
	// Measured on APFS: more than 4 workers is slower (kernel lock contention).
	workers := runtime.GOMAXPROCS(0)
	if workers > 4 {
		workers = 4
	}
	if workers > len(names) {
		workers = len(names)
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var buf []byte
			for {
				i := int(next.Add(1) - 1)
				if i >= len(names) {
					return
				}
				scans[i] = scanDir(src, names[i], &buf)
			}
		}()
	}
	wg.Wait()
	for i := range scans {
		r.merge(src, &scans[i])
	}
	return nil
}

func isDir(fsys fs.FS, e fs.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&fs.ModeSymlink != 0 {
		st, err := fs.Stat(fsys, e.Name())
		return err == nil && st.IsDir()
	}
	return false
}

// dirScan is what one persona directory in one layer contributes.
type dirScan struct {
	id        string
	def       *FileRef
	fm        FrontMatter
	hasBody   bool
	knowledge []Knowledge
	problems  []string
	overflow  string
}

func scanDir(src *Source, dirName string, buf *[]byte) dirScan {
	ds := dirScan{id: strings.ToLower(dirName)}
	defName := dirName + ".md"
	files := 0
	_ = fs.WalkDir(src.FS, dirName, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if fp != dirName && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(path.Ext(fp), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if files >= MaxFilesPerDir {
			ds.overflow = fmt.Sprintf("%s layer: persona %q has more than %d files; %s and later skipped", src.Layer, ds.id, MaxFilesPerDir, fp)
			return fs.SkipAll
		}
		files++
		rel := strings.TrimPrefix(fp, dirName+"/")
		ref := FileRef{Layer: src.Layer, Path: displayPath(src, fp), Rel: rel, src: src}
		if rel == defName {
			ds.readDefinition(ref, fp, buf)
			return nil
		}
		kind := knowledgeKind(rel)
		if kind == "source" {
			return nil // raw imports awaiting rewrite are not knowledge
		}
		k := Knowledge{FileRef: ref, Kind: kind}
		k.Title, k.Tags = knowledgeTitle(src.FS, fp, rel, buf)
		ds.knowledge = append(ds.knowledge, k)
		return nil
	})
	return ds
}

func (ds *dirScan) readDefinition(ref FileRef, fp string, buf *[]byte) {
	head, err := readHead(ref.src.FS, fp, frontMatterHead, buf)
	if err != nil {
		ds.problems = append(ds.problems, fmt.Sprintf("%s: %v", ref.Path, err))
		return
	}
	ds.fm = ParseFrontMatter(head)
	// A front-matter-only file overrides metadata but never erases a lower body.
	ds.hasBody = len(head) >= frontMatterHead || !ds.fm.Present
	if !ds.hasBody && ds.fm.BodyAt <= len(head) {
		ds.hasBody = len(strings.TrimSpace(string(head[ds.fm.BodyAt:]))) > 0
	}
	if ds.fm.Present && !ds.fm.Closed && len(head) >= frontMatterHead {
		if more, err := readHead(ref.src.FS, fp, frontMatterLimit, buf); err == nil {
			ds.fm = ParseFrontMatter(more)
		}
	}
	for _, prob := range ds.fm.Problems {
		ds.problems = append(ds.problems, fmt.Sprintf("%s: %s", ref.Path, prob))
	}
	cp := ref
	ds.def = &cp
}

// merge applies one layer's scan of a persona dir onto the roster (ADR 3 §4).
func (r *Roster) merge(src *Source, ds *dirScan) {
	p, ok := r.byID[ds.id]
	if !ok {
		p = &Persona{ID: ds.id, Selectable: true, Provenance: map[string]Layer{}, defs: map[Layer]*FileRef{}, fields: map[string]Value{}}
		r.byID[ds.id] = p
	}
	p.Layers = append(p.Layers, src.Layer)
	if src.Layer == Builtin {
		applyBuiltinOverride(p)
	}
	if ds.overflow != "" {
		r.Overflow = append(r.Overflow, ds.overflow)
	}
	p.Problems = append(p.Problems, ds.problems...)
	if ds.def != nil {
		p.defs[src.Layer] = ds.def
		if ds.hasBody || p.Definition == nil {
			p.Definition = ds.def
			p.Provenance["body"] = src.Layer
		}
		for _, key := range ds.fm.Keys {
			v := ds.fm.Fields[key]
			if !v.IsList && v.Str == "" {
				continue // an empty value does not "set" the field
			}
			p.fields[key] = v
			p.Provenance[key] = src.Layer
			setField(p, key, v)
		}
	}
	if len(ds.knowledge) > 0 {
		index := make(map[string]int, len(p.Knowledge))
		for i, k := range p.Knowledge {
			index[k.Rel] = i
		}
		for _, k := range ds.knowledge {
			if i, dup := index[k.Rel]; dup {
				p.Knowledge[i] = k // higher layer wins the relative path
			} else {
				index[k.Rel] = len(p.Knowledge)
				p.Knowledge = append(p.Knowledge, k)
			}
		}
	}
}

func displayPath(src *Source, fp string) string {
	if src.Layer == Builtin {
		return "builtin:personas/" + fp
	}
	return src.Dir + string(os.PathSeparator) + fp
}

func knowledgeKind(rel string) string {
	first, _, nested := strings.Cut(rel, "/")
	switch {
	case nested && (first == "memories" || first == "memory"):
		return "memory"
	case nested && first == "examples":
		return "example"
	case nested && first == "source":
		return "source"
	case strings.EqualFold(rel, "style.md"):
		return "style"
	}
	return "knowledge"
}

func setField(p *Persona, key string, v Value) {
	switch key {
	case "name":
		p.Name = v.String()
	case "displayName":
		p.DisplayName = v.String()
	case "title":
		p.Title = v.String()
	case "icon":
		p.Icon = v.String()
	case "role":
		p.Role = v.String()
	case "domains":
		p.Domains = v.Items()
	case "capabilities":
		p.Capabilities = v.String()
	case "identity":
		p.Identity = v.String()
	case "primaryLens":
		p.PrimaryLens = v.String()
	case "communicationStyle":
		p.CommunicationStyle = v.String()
	case "principles":
		p.Principles = v.String()
	case "selectable":
		p.Selectable = parseBool(v.Str, p.Selectable)
	case "silent":
		p.Silent = parseBool(v.Str, p.Silent)
	case "subtask":
		p.Subtask = parseBool(v.Str, p.Subtask)
	default:
		if p.Extra == nil {
			p.Extra = map[string]string{}
		}
		p.Extra[key] = v.String()
	}
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0":
		return false
	}
	return def
}

// readHead reads up to n bytes of name into *buf (grown as needed); the
// result is valid until the next read with the same buffer.
func readHead(fsys fs.FS, name string, n int, buf *[]byte) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if cap(*buf) < n {
		*buf = make([]byte, n)
	}
	b := (*buf)[:n]
	m, err := io.ReadFull(f, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:m], nil
}

// fsPath is ref's path inside its layer fs (<dir>/<rel>).
func fsPath(ref FileRef) string {
	if ref.Layer == Builtin {
		return strings.TrimPrefix(ref.Path, "builtin:personas/")
	}
	rel := strings.TrimPrefix(ref.Path, ref.src.Dir+string(os.PathSeparator))
	return strings.ReplaceAll(rel, string(os.PathSeparator), "/")
}

// knowledgeTitle reads a small head: front-matter title/tags, else first heading, else file name.
func knowledgeTitle(fsys fs.FS, fp, rel string, buf *[]byte) (string, []string) {
	fallback := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	head, err := readHead(fsys, fp, knowledgeHead, buf)
	if err != nil {
		return fallback, nil
	}
	fm := ParseFrontMatter(head)
	var tags []string
	if v, ok := fm.Fields["tags"]; ok {
		tags = v.Items()
	}
	if v, ok := fm.Fields["title"]; ok && v.String() != "" {
		return v.String(), tags
	}
	body := head
	if fm.Present && fm.BodyAt <= len(head) {
		body = head[fm.BodyAt:]
	}
	for _, line := range strings.Split(string(body), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			if title := strings.TrimSpace(strings.TrimLeft(t, "#")); title != "" {
				return title, tags
			}
		}
	}
	return fallback, tags
}

// ReadDefinition returns the full text of the winning definition file.
func (p *Persona) ReadDefinition() (string, error) {
	if p.Definition == nil {
		return "", fmt.Errorf("persona %q has no definition file in any layer", p.ID)
	}
	return readFile(*p.Definition)
}

// ReadFile returns the full text of a referenced file.
func readFile(ref FileRef) (string, error) {
	if ref.src == nil {
		return "", fmt.Errorf("%s: no source", ref.Path)
	}
	b, err := fs.ReadFile(ref.src.FS, fsPath(ref))
	return string(b), err
}

// Body returns the definition's markdown body without front matter.
func (p *Persona) Body() (string, error) {
	text, err := p.ReadDefinition()
	if err != nil {
		return "", err
	}
	fm := ParseFrontMatter([]byte(text))
	if fm.Present && fm.Closed {
		return text[fm.BodyAt:], nil
	}
	return text, nil
}

// Field returns a raw merged front-matter value.
func (p *Persona) Field(key string) (Value, bool) {
	v, ok := p.fields[key]
	return v, ok
}

// DefinitionIn returns the persona's definition file in one layer, if any.
func (p *Persona) DefinitionIn(l Layer) *FileRef { return p.defs[l] }
