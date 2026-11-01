package gates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/redact"
	"github.com/muthuishere/huddle-next/internal/repo"
)

// EvVerify is the verdict event type.
const EvVerify = "verify.recorded"

// Verdicts.
const (
	PASS     = "PASS"
	CONCERNS = "CONCERNS"
	FAIL     = "FAIL"
)

// Verdict is one recorded verification.
type Verdict struct {
	EventID     string    `json:"eventId"`
	ArtifactID  string    `json:"artifactId"`
	Kind        string    `json:"kind"`
	Path        string    `json:"path"`
	Verdict     string    `json:"verdict"`
	Requested   string    `json:"requested,omitempty"` // verdict asked for, when forced down
	EvalStatus  string    `json:"evalStatus,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Mode        string    `json:"mode"` // git | content
	Head        string    `json:"head,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	By          string    `json:"by,omitempty"`
	TS          time.Time `json:"ts"`
	Findings    int       `json:"findings"`
	Note        string    `json:"note,omitempty"`
	Stale       bool      `json:"stale"`
}

// Fingerprint hashes the working tree. In git: sha256 over `git ls-files -s`
// plus `git diff HEAD`. Outside git, or when git
// fails: sha256 over every artifact file's path and content.
func Fingerprint(root string, info repo.Info, cfg config.Config) (fp, mode string, err error) {
	if info.IsGit {
		if fp, err := gitFingerprint(root); err == nil {
			return fp, "git", nil
		}
	}
	return contentFingerprint(root, cfg)
}

func gitFingerprint(root string) (string, error) {
	h := sha256.New()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
	ls, err := run("ls-files", "-s")
	if err != nil {
		return "", err
	}
	h.Write([]byte("ls-files\x00"))
	h.Write(ls)
	diff, err := run("diff", "HEAD", "--no-color", "--no-ext-diff")
	if err != nil {
		// No commits yet: the index listing plus the cached diff still move on every change.
		if diff, err = run("diff", "--cached", "--no-color", "--no-ext-diff"); err != nil {
			return "", err
		}
	}
	h.Write([]byte("\x00diff\x00"))
	h.Write(diff)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func contentFingerprint(root string, cfg config.Config) (string, string, error) {
	all, err := artifacts.Discover(root, cfg)
	if err != nil {
		return "", "", err
	}
	h := sha256.New()
	for _, a := range all {
		h.Write([]byte(a.Kind + "\x00" + a.Path + "\x00" + a.SHA256 + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil)), "content", nil
}

// HeadCommit resolves HEAD by reading .git files (no exec) — cheap enough for
// `sdlc status`. Returns "" when unknown.
func HeadCommit(root string) string {
	gitPath := filepath.Join(root, ".git")
	st, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	if !st.IsDir() {
		b, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		gitDir = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(root, gitDir)
		}
	}
	common := gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		c := strings.TrimSpace(string(b))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitDir, c)
		}
		common = c
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(b))
	ref, ok := strings.CutPrefix(head, "ref: ")
	if !ok {
		return head
	}
	for _, dir := range []string{gitDir, common} {
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ref))); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	if b, err := os.ReadFile(filepath.Join(common, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if sha, name, ok := strings.Cut(strings.TrimSpace(line), " "); ok && name == ref {
				return sha
			}
		}
	}
	return ""
}

// RecordOptions are the flags of `verify record`.
type RecordOptions struct {
	Ref          string
	Verdict      string
	FindingsFile string
	EvalStatus   string // passed | unverified | blocked | failed | ""
	By           string
}

// Record stamps a verdict with the tree fingerprint. When the eval runner was
// blocked or unverified, PASS is forced down to CONCERNS: nothing unverified
// can pass.
func Record(root string, env artifacts.Env, o RecordOptions) (Verdict, error) {
	verdict := strings.ToUpper(strings.TrimSpace(o.Verdict))
	switch verdict {
	case PASS, CONCERNS, FAIL:
	default:
		return Verdict{}, fmt.Errorf("--verdict must be PASS, CONCERNS, or FAIL (got %q)", o.Verdict)
	}
	a, err := artifacts.Resolve(root, env.Cfg, "", o.Ref)
	if err != nil {
		return Verdict{}, err
	}
	evalStatus := strings.ToLower(strings.TrimSpace(o.EvalStatus))
	switch evalStatus {
	case "", "passed", "pass", "unverified", "blocked", "failed", "fail":
	default:
		return Verdict{}, fmt.Errorf("--eval-status must be passed, unverified, blocked, or failed (got %q)", o.EvalStatus)
	}
	v := Verdict{ArtifactID: a.ID, Kind: a.Kind, Path: a.Path, Verdict: verdict, EvalStatus: evalStatus, SHA256: a.SHA256, By: o.By}
	if verdict == PASS && (evalStatus == "blocked" || evalStatus == "unverified") {
		v.Requested, v.Verdict = PASS, CONCERNS
		v.Note = "eval runner " + evalStatus + ": PASS needs verified evals, recorded as CONCERNS"
	}
	if verdict == PASS && (evalStatus == "failed" || evalStatus == "fail") {
		v.Requested, v.Verdict = PASS, FAIL
		v.Note = "evals failed: PASS recorded as FAIL"
	}
	var findings any
	if o.FindingsFile != "" {
		var b []byte
		if o.FindingsFile == "-" {
			b, err = readAllStdin()
		} else {
			b, err = os.ReadFile(o.FindingsFile)
		}
		if err != nil {
			return Verdict{}, err
		}
		clean := redact.String(string(b))
		if err := json.Unmarshal([]byte(clean), &findings); err != nil {
			return Verdict{}, fmt.Errorf("--findings %s is not valid JSON: %w", o.FindingsFile, err)
		}
		if list, ok := findings.([]any); ok {
			v.Findings = len(list)
		} else if m, ok := findings.(map[string]any); ok {
			if list, ok := m["findings"].([]any); ok {
				v.Findings = len(list)
			}
		}
	}
	v.Fingerprint, v.Mode, err = Fingerprint(root, env.Repo, env.Cfg)
	if err != nil {
		return Verdict{}, err
	}
	if env.Repo.IsGit {
		v.Head = HeadCommit(root)
	}
	data := map[string]any{
		"artifactId": v.ArtifactID, "verdict": v.Verdict, "fingerprint": v.Fingerprint, "mode": v.Mode,
		"head": v.Head, "evalStatus": v.EvalStatus, "findingsCount": v.Findings,
	}
	if v.Requested != "" {
		data["requested"], data["note"] = v.Requested, v.Note
	}
	if findings != nil {
		data["findings"] = findings
	}
	ev, err := eventlog.Append(env.Repo.Name, eventlog.Event{
		Type: EvVerify, By: o.By, Branch: env.Repo.Branch, Refs: []string{a.ID},
		Artifact: &eventlog.ArtifactRef{Kind: a.Kind, Path: a.Path, SHA256: a.SHA256},
		Data:     data,
	})
	v.EventID, v.TS = ev.ID, ev.TS
	return v, err
}

// Latest returns the newest verdict per artifact id.
func Latest(repoName string) (map[string]Verdict, error) {
	evs, err := eventlog.List(repoName, eventlog.Filter{Types: map[string]bool{EvVerify: true}})
	if err != nil {
		return nil, err
	}
	out := map[string]Verdict{}
	for _, e := range evs {
		v := Verdict{EventID: e.ID, By: e.By, TS: e.TS}
		v.ArtifactID, _ = e.Data["artifactId"].(string)
		v.Verdict, _ = e.Data["verdict"].(string)
		v.Fingerprint, _ = e.Data["fingerprint"].(string)
		v.Mode, _ = e.Data["mode"].(string)
		v.Head, _ = e.Data["head"].(string)
		v.EvalStatus, _ = e.Data["evalStatus"].(string)
		v.Requested, _ = e.Data["requested"].(string)
		v.Note, _ = e.Data["note"].(string)
		if n, ok := e.Data["findingsCount"].(float64); ok {
			v.Findings = int(n)
		}
		if e.Artifact != nil {
			v.Kind, v.Path, v.SHA256 = e.Artifact.Kind, e.Artifact.Path, e.Artifact.SHA256
		}
		if v.ArtifactID != "" {
			out[v.ArtifactID] = v
		}
	}
	return out, nil
}

// MarkStale compares each verdict's fingerprint to fp (full check).
func MarkStale(vs map[string]Verdict, fp string) {
	for id, v := range vs {
		v.Stale = v.Fingerprint != fp
		vs[id] = v
	}
}

// CheapStale is the no-exec staleness check `sdlc status` uses: HEAD moved or
// the verified artifact's file changed. `verify status` runs the full check.
func CheapStale(v Verdict, head string, current map[string]artifacts.Artifact) bool {
	if v.Head != "" && head != "" && v.Head != head {
		return true
	}
	a, ok := current[v.Path]
	return !ok || (v.SHA256 != "" && a.SHA256 != v.SHA256)
}

var readAllStdin = func() ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(os.Stdin)
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return nil, err
	}
	return buf.Bytes(), nil
}
