package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muthuishere/huddle-next/internal/config"
)

// Defaults for the Claude hook pack when config sets nothing.
var (
	DefaultProtectedPaths = []string{".huddle/config.json", "**/generated/**"}
	DefaultDeployPatterns = []string{
		"kubectl apply", "kubectl rollout", "helm upgrade", "helm install",
		"terraform apply", "pulumi up", "fly deploy", "vercel --prod",
		"gcloud run deploy", "aws ecs update-service", "wrangler deploy",
		"npm publish", "goreleaser release", "task deploy", "make deploy",
	}
)

// Pack is a drafted harness hook pack. Files are repo-relative paths.
type Pack struct {
	Host     string            `json:"host"`
	Settings map[string]any    `json:"settings"`        // .claude/settings.json fragment
	Files    map[string]string `json:"files"`           // .claude/hooks/*.sh
	Managed  map[string]any    `json:"managedSettings"` // for platform teams, never written
	Notes    []string          `json:"notes,omitempty"`
}

// PackSettingsPath is where the settings fragment goes for Claude Code.
const PackSettingsPath = ".claude/settings.json"

// ExportClaude drafts the Claude Code hook pack from config (ADR 17 §8, ADR 19 row 9).
func ExportClaude(cfg config.Config) Pack {
	protected := stringList(cfg.Extra["protectedPaths"], DefaultProtectedPaths)
	deploy := stringList(cfg.Extra["deployPatterns"], DefaultDeployPatterns)
	innerLoop := stringList(cfg.Extra["innerLoop"], nil)
	format := stringValue(cfg.Extra["formatCommand"])
	lint := stringValue(cfg.Extra["lintCommand"])

	p := Pack{Host: "claude", Files: map[string]string{}}
	cmd := func(name string) map[string]any {
		return map[string]any{"type": "command", "command": `sh "$CLAUDE_PROJECT_DIR/.claude/hooks/` + name + `"`}
	}
	editTools := "Edit|Write|MultiEdit|NotebookEdit"

	p.Files[".claude/hooks/huddle-lib.sh"] = libScript
	p.Files[".claude/hooks/huddle-protected-paths.sh"] = strings.Replace(protectedScript, "@@PATTERNS@@", lines(protected), 1)
	p.Files[".claude/hooks/huddle-fix-mode-tests.sh"] = fixModeScript
	p.Files[".claude/hooks/huddle-plan-drift.sh"] = planDriftScript
	p.Files[".claude/hooks/huddle-production-gate.sh"] = strings.Replace(deployScript, "@@PATTERNS@@", lines(deploy), 1)

	pre := []any{
		map[string]any{"matcher": editTools, "hooks": []any{cmd("huddle-protected-paths.sh"), cmd("huddle-fix-mode-tests.sh")}},
		map[string]any{"matcher": "Bash", "hooks": []any{cmd("huddle-production-gate.sh")}},
	}
	postHooks := []any{cmd("huddle-plan-drift.sh")}
	if format != "" || lint != "" {
		script := strings.Replace(postEditScript, "@@FORMAT@@", shQuote(format), 1)
		script = strings.Replace(script, "@@LINT@@", shQuote(lint), 1)
		p.Files[".claude/hooks/huddle-post-edit.sh"] = script
		postHooks = append(postHooks, cmd("huddle-post-edit.sh"))
	} else {
		p.Notes = append(p.Notes, "no formatCommand/lintCommand in config — post-edit format/lint hook skipped (run: huddle config set formatCommand \"gofmt -w\")")
	}
	post := []any{map[string]any{"matcher": editTools, "hooks": postHooks}}

	p.Settings = map[string]any{"hooks": map[string]any{"PreToolUse": pre, "PostToolUse": post}}
	if len(innerLoop) > 0 {
		allow := make([]any, 0, len(innerLoop))
		for _, c := range innerLoop {
			allow = append(allow, "Bash("+c+")")
		}
		p.Settings["permissions"] = map[string]any{"allow": allow}
	} else {
		p.Notes = append(p.Notes, `no innerLoop in config — permissions.allow skipped (run: huddle config set innerLoop '["go test:*","go build:*"]')`)
	}
	p.Managed = map[string]any{
		"permissions": map[string]any{
			"deny":                         []any{"Read(.env*)", "Read(**/.env*)", "Read(**/secrets/**)"},
			"disableBypassPermissionsMode": "disable",
		},
	}
	return p
}

// WriteReport lists what WritePack did per file: "written", "skipped (exists)", "merged".
type WriteReport struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// WritePack writes the pack under repoRoot. Existing files are never replaced
// unless force; settings.json is merged key-wise (hooks/permissions replaced
// only with force when already present).
func WritePack(p Pack, repoRoot string, force bool) ([]WriteReport, error) {
	var out []WriteReport
	names := make([]string, 0, len(p.Files))
	for n := range p.Files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, rel := range names {
		dst := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(dst); err == nil && !force {
			out = append(out, WriteReport{rel, "skipped (exists — use --force)"})
			continue
		}
		if err := writeFile(dst, []byte(p.Files[rel]), 0o755); err != nil {
			return out, err
		}
		out = append(out, WriteReport{rel, "written"})
	}

	dst := filepath.Join(repoRoot, filepath.FromSlash(PackSettingsPath))
	existing := map[string]any{}
	b, err := os.ReadFile(dst)
	switch {
	case err == nil:
		if len(strings.TrimSpace(string(b))) > 0 {
			if err := json.Unmarshal(b, &existing); err != nil {
				return out, fmt.Errorf("%s is not valid JSON: %w — fix it, then rerun: huddle hooks export --host claude --write", PackSettingsPath, err)
			}
		}
	case !errors.Is(err, os.ErrNotExist):
		return out, err
	}
	changed, skipped := false, []string{}
	for k, v := range p.Settings {
		if _, has := existing[k]; has && !force {
			skipped = append(skipped, k)
			continue
		}
		existing[k] = v
		changed = true
	}
	switch {
	case !changed:
		out = append(out, WriteReport{PackSettingsPath, "skipped (" + strings.Join(skipped, ",") + " exist — merge the draft by hand or use --force)"})
	default:
		data, _ := json.MarshalIndent(existing, "", "  ")
		if err := writeFile(dst, append(data, '\n'), 0o644); err != nil {
			return out, err
		}
		action := "written"
		if len(skipped) > 0 {
			action = "merged (kept existing " + strings.Join(skipped, ",") + ")"
		}
		out = append(out, WriteReport{PackSettingsPath, action})
	}
	return out, nil
}

func writeFile(dst string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".huddle-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func stringList(raw json.RawMessage, def []string) []string {
	if len(raw) == 0 {
		return def
	}
	var list []string
	if json.Unmarshal(raw, &list) != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil || strings.TrimSpace(one) == "" {
			return def
		}
		list = []string{one}
	}
	var out []string
	for _, s := range list {
		s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func stringValue(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

func lines(list []string) string { return strings.Join(list, "\n") }

// shQuote single-quotes s for POSIX sh.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

const libScript = `#!/bin/sh
# huddle hook helpers (drafted by: huddle hooks export --host claude). POSIX sh.
# Reads Claude Code hook JSON from stdin once into HOOK_INPUT.
HOOK_INPUT=$(cat)

# has_jq: jq on PATH (HUDDLE_HOOK_NO_JQ=1 forces the sed/tr fallback).
has_jq() {
  [ "${HUDDLE_HOOK_NO_JQ:-}" != "1" ] && command -v jq >/dev/null 2>&1
}

# json_field <name>: first string value of "name" in HOOK_INPUT (jq if present).
json_field() {
  if has_jq; then
    printf '%s' "$HOOK_INPUT" | jq -r --arg k "$1" '[.. | objects | .[$k]? | strings] | first // empty' 2>/dev/null
  else
    printf '%s' "$HOOK_INPUT" | tr '\n' ' ' | sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n 1
  fi
}

# rel_path <abs>: path relative to the project dir.
rel_path() {
  root=${CLAUDE_PROJECT_DIR:-$(pwd)}
  case "$1" in
    "$root"/*) printf '%s' "${1#"$root"/}" ;;
    *) printf '%s' "$1" ;;
  esac
}

# deny <reason>: block the tool call; Claude sees the reason.
deny() {
  printf '%s\n' "$1" >&2
  exit 2
}
`

const protectedScript = `#!/bin/sh
# PreToolUse Edit|Write: deny edits to protected paths (config: protectedPaths).
. "$(dirname "$0")/huddle-lib.sh"
file=$(json_field file_path)
[ -z "$file" ] && file=$(json_field notebook_path)
[ -z "$file" ] && exit 0
rel=$(rel_path "$file")
while IFS= read -r pat; do
  [ -z "$pat" ] && continue
  # shellcheck disable=SC2254
  case "$rel" in $pat) deny "huddle: $rel is protected ($pat). Ask the user to edit it, or change protectedPaths in .huddle/config.json." ;; esac
  case "/$rel" in $pat) deny "huddle: $rel is protected ($pat). Ask the user to edit it, or change protectedPaths in .huddle/config.json." ;; esac
done <<'HUDDLE_PATTERNS'
@@PATTERNS@@
HUDDLE_PATTERNS
exit 0
`

const fixModeScript = `#!/bin/sh
# PreToolUse Edit|Write: during a fix task (HUDDLE_FIX_MODE=1) the failing test
# is already committed — do not edit tests to make them pass.
[ "${HUDDLE_FIX_MODE:-}" = "1" ] || exit 0
. "$(dirname "$0")/huddle-lib.sh"
file=$(json_field file_path)
[ -z "$file" ] && exit 0
rel=$(rel_path "$file")
case "/$rel" in
  *_test.go|*.test.*|*.spec.*|*_test.py|*/test_*.py|*Test.java|*Tests.java|*Test.kt|*_spec.rb|*/test/*|*/tests/*|*/__tests__/*|*/spec/*)
    deny "huddle: fix mode — $rel is a test file. Fix the code, not the test. Unset HUDDLE_FIX_MODE only with the user's approval." ;;
esac
exit 0
`

const planDriftScript = `#!/bin/sh
# PostToolUse Edit|Write: remind when an edited file is not listed under
# "Files that change" in any intent/*/plan.md (plan <-> diff drift).
. "$(dirname "$0")/huddle-lib.sh"
file=$(json_field file_path)
[ -z "$file" ] && exit 0
root=${CLAUDE_PROJECT_DIR:-$(pwd)}
rel=$(rel_path "$file")
plans=$(ls "$root"/intent/*/plan.md 2>/dev/null)
[ -z "$plans" ] && exit 0
case "$rel" in intent/*) exit 0 ;; esac
listed=$(awk '
  /^#+[[:space:]]/ { inside = ($0 ~ /[Ff]iles that change/) ; next }
  inside && /^[[:space:]]*[-*][[:space:]]/ {
    line = $0
    sub(/^[[:space:]]*[-*][[:space:]]+/, "", line)
    gsub(/[` + "`" + `_*]/, "", line)
    split(line, parts, /[[:space:]]/)
    sub(/\/+$/, "", parts[1])
    if (parts[1] != "") print parts[1]
  }' $plans)
for p in $listed; do
  # shellcheck disable=SC2254
  case "$rel" in $p|$p/*) exit 0 ;; esac
done
msg="huddle: $rel is not in any plan's 'Files that change'. Update intent/<slug>/plan.md in the same commit, or tell the user the build departed from the plan (see: huddle sdlc status)."
printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "$(printf '%s' "$msg" | sed 's/\\/\\\\/g; s/"/\\"/g')"
exit 0
`

const deployScript = `#!/bin/sh
# PreToolUse Bash: production gate. Commands matching deployPatterns need an
# approved deploy gate (a human runs huddle gate approve; agents never do).
. "$(dirname "$0")/huddle-lib.sh"
cmd=$(json_field command)
[ -z "$cmd" ] && exit 0
hit=""
while IFS= read -r pat; do
  [ -z "$pat" ] && continue
  # shellcheck disable=SC2254
  case "$cmd" in *$pat*) hit=$pat; break ;; esac
done <<'HUDDLE_PATTERNS'
@@PATTERNS@@
HUDDLE_PATTERNS
[ -z "$hit" ] && exit 0
command -v huddle >/dev/null 2>&1 || deny "huddle: '$hit' is a production command and huddle is not on PATH to check the deploy gate."
status=$(huddle gate status --json 2>/dev/null)
# Contract: [{id, name, artifact, role, state, stale, approvedBy, ts}]. Open only on a
# gate named deploy|production|release that is approved and not stale.
approved=""
if has_jq; then
  approved=$(printf '%s' "$status" | jq -r '[.[]? | select(((.name // "") | test("^(deploy|production|release)$")) and .state == "approved" and .stale == false)] | length' 2>/dev/null)
  [ "$approved" = "0" ] && approved=""
else
  approved=$(printf '%s' "$status" | tr -d ' \n\t\r' | tr '}' '\n' | grep -E '"name":"(deploy|production|release)"' | grep '"state":"approved"' | grep '"stale":false')
fi
[ -n "$approved" ] && exit 0
deny "huddle: '$hit' deploys to production and no deploy gate is approved (or it went stale). Ask the user to run: huddle gate request <artifact-id> --role <owner-role> --name deploy — a human owner approves in their terminal; the agent never runs huddle gate approve."
`

const postEditScript = `#!/bin/sh
# PostToolUse Edit|Write: run the repo's formatter and linter (config:
# formatCommand, lintCommand) on the edited file. Lint failures are fed back.
. "$(dirname "$0")/huddle-lib.sh"
file=$(json_field file_path)
[ -z "$file" ] && exit 0
[ -f "$file" ] || exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0
format=@@FORMAT@@
lint=@@LINT@@
if [ -n "$format" ]; then
  sh -c "$format \"\$1\"" huddle-format "$file" >/dev/null 2>&1 || true
fi
if [ -n "$lint" ]; then
  out=$(sh -c "$lint \"\$1\"" huddle-lint "$file" 2>&1) || {
    printf 'huddle: lint failed for %s:\n%s\n' "$file" "$(printf '%s' "$out" | tail -n 20)" >&2
    exit 2
  }
fi
exit 0
`
