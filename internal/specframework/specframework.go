// Package specframework notices that a repo also uses an external spec
// framework (today: OpenSpec) and reports where that framework keeps its
// files. It is detection only, by decision of ADR 12 and ADR 23: huddle has no
// dependency on, adapter for, or validation against any spec framework, never
// writes a framework's files, and never parses their markdown. Absence is the
// normal case and is never an error — with no framework present, huddle reports
// its native playbook paths and everything works unchanged.
//
// Detect reads the filesystem only (plus exec.LookPath, which stats PATH). The
// one process huddle ever starts is the framework's own read-only status/list
// command in Changes, which is called from human-invoked commands only — never
// from the `huddle start` path, which stays exec-free and network-free.
// Everything that command prints is untrusted data: it is decoded into narrow
// structs, redacted, and bounded, never executed and never read as instruction.
package specframework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/redact"
)

// ExecTimeout bounds every single call into an external framework CLI, and
// Budget bounds a whole Changes walk — one change per process means a repo with
// many of them must not turn a status line into a stall. A walk that runs out
// of budget returns what it has.
const (
	ExecTimeout = 3 * time.Second
	Budget      = 6 * time.Second
)

// maxOutput caps what we read from the external CLI; maxChanges caps how many
// changes we ask about, so a huge repo cannot turn a status line into a stall.
const (
	maxOutput  = 1 << 20
	maxChanges = 25
)

// Detection is a framework huddle noticed in a repo. Dir is repo-relative;
// CLI is the resolved binary path, empty when the tool is not installed.
type Detection struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
	CLI  string `json:"cli,omitempty"`
}

// known is the table of frameworks huddle can recognise. Adding one is a data
// change: a directory to stat and a binary name to look for, nothing more.
var known = []struct {
	name   string
	dir    string
	marker string // a file inside dir that confirms it, "" to accept the dir
	bin    string
}{
	{name: "openspec", dir: "openspec", marker: "config.yaml", bin: "openspec"},
}

// Detect returns the framework present in root, or nil. No process is started.
func Detect(root string) *Detection {
	for _, k := range known {
		dir := filepath.Join(root, k.dir)
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			continue
		}
		if k.marker != "" {
			if _, err := os.Stat(filepath.Join(dir, k.marker)); err != nil {
				continue
			}
		}
		d := &Detection{Name: k.name, Dir: k.dir}
		if p, err := exec.LookPath(k.bin); err == nil {
			d.CLI = p
		}
		return d
	}
	return nil
}

// Line is the one informational line huddle shows a human. Absence is normal,
// so it never reads as a defect.
func Line(d *Detection) string {
	if d == nil {
		return "none — huddle artifacts use the native playbook layout (intent/<slug>/…)"
	}
	if d.CLI == "" {
		return d.Name + "/ present, " + d.Name + " CLI not on PATH — huddle stays on its native layout and reports native paths"
	}
	return d.Name + "/ present, CLI " + d.CLI + " — huddle can locate its files; huddle's own artifacts stay native"
}

// Artifact is one file the framework owns, with the huddle stage it lines up
// with. The stage is a label for a human reading the two chains side by side;
// huddle neither reads nor writes the file.
type Artifact struct {
	Kind   string `json:"kind"`
	Native string `json:"native,omitempty"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// Change is one unit of work in the external framework.
type Change struct {
	ID        string     `json:"id"`
	Dir       string     `json:"dir"`
	Complete  bool       `json:"complete"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
}

// nativeOf maps an OpenSpec artifact id onto the nearest huddle artifact. It
// is descriptive only — no path mapping, no conversion (ADR 23).
var nativeOf = map[string]string{
	"proposal": "intent",
	"specs":    "spec",
	"design":   "adr",
	"tasks":    "plan",
}

// Runner runs an external command and returns its stdout. Tests substitute it.
type Runner func(ctx context.Context, bin, dir string, args []string) ([]byte, error)

// Client reads an external framework's layout. Zero exec happens until Changes.
type Client struct {
	Root string
	Det  Detection
	Run  Runner
}

// New returns a client for a detected framework, or nil when there is nothing
// to read (no framework, or its CLI is not installed).
func New(root string, d *Detection) *Client {
	if d == nil || d.CLI == "" {
		return nil
	}
	return &Client{Root: root, Det: *d, Run: execRunner}
}

var errUnsupported = errors.New("specframework: unsupported framework")

// Changes lists the framework's changes and where each artifact lives. A nil
// client, a failing CLI or unparseable output all yield no changes and no
// error worth stopping for — the caller falls back to native paths.
func (c *Client) Changes(ctx context.Context) ([]Change, error) {
	if c == nil {
		return nil, nil
	}
	if c.Det.Name != "openspec" {
		return nil, errUnsupported
	}
	run := c.Run
	if run == nil {
		run = execRunner
	}
	out, err := run(ctx, c.Det.CLI, c.Root, []string{"list", "--json"})
	if err != nil {
		return nil, err
	}
	var listed struct {
		Changes []struct {
			Name string `json:"name"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		return nil, fmt.Errorf("specframework: %s list --json: %w", c.Det.Name, err)
	}
	var ids []string
	for _, ch := range listed.Changes {
		if id := strings.TrimSpace(ch.Name); safeID(id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > maxChanges {
		ids = ids[:maxChanges]
	}
	ctx, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	changes := make([]Change, 0, len(ids))
	for _, id := range ids {
		if ctx.Err() != nil {
			break // out of budget: report the changes we did read
		}
		ch, err := c.change(ctx, run, id)
		if err != nil {
			continue // one unreadable change must not hide the others
		}
		changes = append(changes, ch)
	}
	return changes, nil
}

func (c *Client) change(ctx context.Context, run Runner, id string) (Change, error) {
	out, err := run(ctx, c.Det.CLI, c.Root, []string{"status", "--change", id, "--json"})
	if err != nil {
		return Change{}, err
	}
	var st struct {
		ChangeName    string `json:"changeName"`
		ChangeRoot    string `json:"changeRoot"`
		IsComplete    bool   `json:"isComplete"`
		ArtifactPaths map[string]struct {
			ResolvedOutputPath  string   `json:"resolvedOutputPath"`
			ExistingOutputPaths []string `json:"existingOutputPaths"`
		} `json:"artifactPaths"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return Change{}, fmt.Errorf("specframework: %s status --json: %w", c.Det.Name, err)
	}
	ch := Change{ID: id, Dir: c.rel(st.ChangeRoot), Complete: st.IsComplete}
	kinds := make([]string, 0, len(st.ArtifactPaths))
	for kind := range st.ArtifactPaths {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		p := st.ArtifactPaths[kind]
		path, exists := c.rel(p.ResolvedOutputPath), len(p.ExistingOutputPaths) > 0
		if len(p.ExistingOutputPaths) == 1 {
			if r := c.rel(p.ExistingOutputPaths[0]); r != "" {
				path = r
			}
		}
		if path == "" {
			continue
		}
		ch.Artifacts = append(ch.Artifacts, Artifact{Kind: clean(kind), Native: nativeOf[kind], Path: path, Exists: exists})
	}
	return ch, nil
}

// rel makes a path the external tool printed safe to show: cleaned, scrubbed,
// repo-relative in slash form, and dropped when it points outside the repo.
func (c *Client) rel(p string) string {
	p = clean(p)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		return filepath.ToSlash(filepath.Clean(p))
	}
	root, err := filepath.Abs(c.Root)
	if err != nil {
		return ""
	}
	if r, ok := within(root, p); ok {
		return r
	}
	// The repo root and the path the tool printed can name the same directory
	// by different routes — on macOS every mktemp dir is under /var, a symlink
	// to /private/var, and the tool resolves it while huddle does not. Compare
	// again with both sides resolved before calling it an escape, or a correct
	// path is silently dropped and the user sees an empty list.
	if r, ok := within(evalExisting(root), evalExisting(p)); ok {
		return r
	}
	return ""
}

// evalExisting resolves symlinks in the deepest part of p that exists and
// re-appends the rest. EvalSymlinks alone is not enough: it fails outright on a
// path that does not exist, and an artifact the tool reports as *missing* has no
// file on disk, so resolving both sides would drop exactly the entries the
// caller needs to be told about. Returns "" when nothing resolves.
func evalExisting(p string) string {
	rest := ""
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return r
			}
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// within reports p as a slash-form path relative to root, and whether p is
// inside root at all.
func within(root, p string) (string, bool) {
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

var ctrl = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// clean treats the external tool's text as data: control characters out,
// secrets masked by the one redaction choke point, length bounded.
func clean(s string) string {
	s = redact.String(ctrl.ReplaceAllString(strings.TrimSpace(s), " "))
	if len(s) > 512 {
		s = s[:512]
	}
	return strings.TrimSpace(s)
}

var idOK = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// safeID rejects anything that could act as a flag or a path when it is passed
// as an argument. Arguments are never interpolated into a shell.
func safeID(s string) bool { return idOK.MatchString(s) }

// execRunner starts the external CLI directly (no shell), in the repo root,
// with a bounded environment, a timeout and a capped read.
func execRunner(ctx context.Context, bin, dir string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, ExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = minimalEnv()
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	out, err := cmd.Output()
	if len(out) > maxOutput {
		out = out[:maxOutput]
	}
	if err != nil {
		return out, fmt.Errorf("specframework: %s %s: %w", filepath.Base(bin), strings.Join(args, " "), err)
	}
	return out, nil
}

// minimalEnv passes only what a CLI needs to start, so huddle never hands a
// third-party process the whole environment (which holds secrets).
func minimalEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "USERPROFILE", "SystemRoot", "TMPDIR", "TEMP", "LANG", "APPDATA"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}
