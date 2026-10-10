package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/install"
	"github.com/muthuishere/huddle-next/internal/output"
	"github.com/muthuishere/huddle-next/internal/paths"
	"github.com/muthuishere/huddle-next/internal/version"
)

func init() {
	register(Command{Name: "doctor", Summary: "Check binary, config, hooks, installed skills and environment", Run: runDoctor})
}

const doctorHelp = `
Usage:
  huddle doctor [--json]

Checks: binary version, global root writable, legacy roots (suggests huddle
migrate), config JSON valid at both scopes, hook commands resolvable, installed
skills match this binary's version, git present (optional).
Exits non-zero when any check fails; warnings don't fail.
`

// DoctorCheck is one doctor finding. Status: ok, warn, fail.
type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

func runDoctor(c *Ctx, args []string) error {
	a, err := parseCoreArgs("doctor", args, nil, []string{"json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, doctorHelp)
	}
	root := c.Cwd
	if info, err := c.Repo(); err == nil {
		root = info.Root
	}
	checks := doctorChecks(root)
	failed := 0
	for _, ch := range checks {
		if ch.Status == "fail" {
			failed++
		}
	}
	if a.bools["json"] {
		if err := output.JSON(c.Stdout, map[string]any{"ok": failed == 0, "checks": checks}); err != nil {
			return err
		}
	} else {
		rows := make([][]string, 0, len(checks))
		for _, ch := range checks {
			detail := ch.Detail
			if ch.Fix != "" {
				detail += " — " + ch.Fix
			}
			rows = append(rows, []string{strings.ToUpper(ch.Status), ch.Name, detail})
		}
		output.Table(c.Stdout, []string{"STATUS", "CHECK", "DETAIL"}, rows)
	}
	if failed > 0 {
		return fmt.Errorf("%d doctor check(s) failed", failed)
	}
	return nil
}

func doctorChecks(root string) []DoctorCheck {
	var out []DoctorCheck
	add := func(name, status, detail, fix string) {
		out = append(out, DoctorCheck{Name: name, Status: status, Detail: detail, Fix: fix})
	}

	add("binary", "ok", version.String(), "")

	// Global root writable.
	g := paths.Global()
	if err := os.MkdirAll(g, 0o700); err != nil {
		add("global-root", "fail", g+": "+err.Error(), "check permissions or set HUDDLE_HOME")
	} else if f, err := os.CreateTemp(g, ".doctor-*"); err != nil {
		add("global-root", "fail", g+" not writable: "+err.Error(), "check permissions or set HUDDLE_HOME")
	} else {
		f.Close()
		os.Remove(f.Name())
		add("global-root", "ok", g+" writable", "")
	}

	// Legacy roots.
	var legacy []string
	for _, l := range paths.LegacyRoots() {
		if st, err := os.Stat(l); err == nil && st.IsDir() {
			legacy = append(legacy, l)
		}
	}
	if len(legacy) > 0 {
		add("legacy-roots", "warn", strings.Join(legacy, ", ")+" present", "run: huddle migrate (copies, never overwrites)")
	} else {
		add("legacy-roots", "ok", "none", "")
	}

	// Config JSON at both scopes.
	for _, scope := range []config.Scope{config.Global, config.Repo} {
		p := config.Path(scope, root)
		b, err := os.ReadFile(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			add("config-"+string(scope), "ok", p+" (absent)", "")
		case err != nil:
			add("config-"+string(scope), "fail", err.Error(), "")
		case len(strings.TrimSpace(string(b))) == 0:
			add("config-"+string(scope), "ok", p+" (empty)", "")
		default:
			var m map[string]json.RawMessage
			if err := json.Unmarshal(b, &m); err != nil {
				add("config-"+string(scope), "fail", p+" is not a valid JSON object: "+err.Error(), "fix it, then run: huddle config show")
			} else {
				add("config-"+string(scope), "ok", p+" valid", "")
			}
		}
	}

	// Hooks resolvable.
	if cfg, err := config.Load(root); err == nil {
		var missing []string
		n := 0
		for _, h := range cfg.Hooks {
			if strings.TrimSpace(h.Run) == "" {
				continue
			}
			n++
			if !hookResolvable(h.Run, root) {
				missing = append(missing, h.Scope+" "+h.On+": "+firstWord(h.Run))
			}
		}
		switch {
		case len(missing) > 0:
			add("hooks", "fail", "not found: "+strings.Join(missing, "; "), "install the command or fix run: in config.json (huddle hooks list)")
		default:
			add("hooks", "ok", fmt.Sprintf("%d command hook(s) resolvable", n), "")
		}
	}

	// Installed skills match binary version.
	if targets, err := install.DefaultTargets(false); err == nil {
		names, _ := install.SkillNames(install.Skills())
		for _, t := range targets {
			var missing, stale []string
			for _, name := range names {
				dir := filepath.Join(t.Dir, name)
				m, err := install.ReadMarker(dir)
				switch {
				case err != nil:
					missing = append(missing, name)
				case m.Version != version.Version:
					stale = append(stale, name+"@"+m.Version)
				}
			}
			switch {
			case len(missing) == len(names):
				add("skills-"+t.Host, "warn", "not installed in "+t.Dir, "run: huddle install --skills")
			case len(missing) > 0 || len(stale) > 0:
				add("skills-"+t.Host, "warn", fmt.Sprintf("missing %v, stale %v (binary %s)", missing, stale, version.Version), "run: huddle install --skills")
			default:
				add("skills-"+t.Host, "ok", fmt.Sprintf("%d skills @ %s in %s", len(names), version.Version, t.Dir), "")
			}
		}
	}

	// git (optional).
	if p, err := exec.LookPath("git"); err == nil {
		add("git", "ok", p, "")
	} else {
		add("git", "warn", "git not on PATH — local-folder mode only", "")
	}
	return out
}

func firstWord(run string) string {
	f := strings.Fields(run)
	if len(f) == 0 {
		return ""
	}
	return strings.Trim(f[0], `"'`)
}

// hookResolvable reports whether a hook's command exists: on PATH, a shell
// builtin-ish word, or a file relative to the repo root.
func hookResolvable(run, root string) bool {
	w := firstWord(run)
	if w == "" {
		return false
	}
	switch w {
	case "echo", "exit", "true", "false", "cd", "test", "[", "printf", ":", "exec", "set", "export":
		return true
	}
	if strings.ContainsAny(w, `/\`) {
		p := w
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		_, err := os.Stat(p)
		return err == nil
	}
	_, err := exec.LookPath(w)
	return err == nil
}
