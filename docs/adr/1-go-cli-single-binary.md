# ADR 1: One CGO-free Go Binary, npm-Wrapped, Installing Its Own Skill

- Status: accepted — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw decision: README Inputs)
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **Huddle ships as one Go binary named `huddle`**, module `github.com/muthuishere/huddle-next`. All eight Python helpers (`global_state`, `project_state`, `session_state`, `synced_assets`, `config_helper`, `repo_context`, `huddle_writer`, `migrate`) become subcommands. No Python at runtime.
2. **CGO is off** (`CGO_ENABLED=0`). Any SQLite need (ADR 8) uses `modernc.org/sqlite`, as crossmem does.
3. **Distribution copies crossmemcli exactly:** goreleaser builds darwin/linux × amd64/arm64 + windows/amd64; a post-build hook drops each binary into `npm/platforms/<os>-<arch>/bin/`; `@muthuishere/huddle` is a thin JS launcher whose `optionalDependencies` are the platform packages; release on `v*` tag via npm OIDC trusted publishing, platform packages first; Homebrew cask in `muthuishere/homebrew-tap`. The npm package is an installer, never a second implementation.
4. **The skill is embedded in the binary** (`//go:embed`: `SKILL.md`, `references/**`, built-in personas). `huddle install --skills` writes it atomically (temp dir → rename) to every detected harness target; `huddle uninstall --skills` removes only those directories and never touches state.
5. **Skill targets are a registry, not a switch:** `~/.claude/skills/huddle` (always; `$CLAUDE_SKILLS_DIR`), `~/.agents/skills/huddle` (when `codex` is on PATH or `--agents`), and further harnesses (`.github/skills`, OpenCode) added as one table row each. Copies, not symlinks — symlinks break on Windows.
6. **Transitional contract:** each replaced script keeps its JSON stdout shape byte-compatible (verified by ADR 9 fixtures) until the step files are rewritten to call `huddle` directly. `install.sh` / `install.cmd` symlink installs are retired once the npm path ships.

## Context

- The skill "feels slow". Measured on 2026-09-13: `global_state` 56 ms, `project_state` 94 ms, `session_state` 717 ms, all three in parallel 703 ms, `gh pr list` alone 746 ms. Go alone would recover only ~50–80 ms; the speed fix is ADR 2.
- The owner chose Go for **product shape** — a distributable, versioned CLI installed like crossmem and ctx-optimize — not for micro-latency.
- Today Huddle is a repo symlinked into skill dirs, depends on a Python interpreter being discoverable (`step-00-preflight.md` blocks when it is not), and has no release artifact.
- crossmemcli already proves the full pipeline (goreleaser, platform packages, OIDC publish, embedded skill, atomic install) for this owner.

## Details

```
cmd/huddle/main.go            thin: app.Run(args, stdout, stderr)
internal/app/                 flag parsing, one file per command group
internal/state/               ADR 6
internal/personas/            ADR 3, 4, 5
internal/context/             git + gh + crossmem (ADR 2, 8)
internal/skillinstall/        embedded FS → harness targets
internal/version/             ldflags Version/Commit/Date
assets/                       go:embed root (SKILL.md, references/, personas/)
npm/  .goreleaser.yaml  Taskfile.yml  .github/workflows/{ci,npm-publish}.yml
```

Command surface (initial):

| Command | Replaces |
|---|---|
| `huddle start --json` | the three parallel preflight scripts (ADR 2, 13) |
| `huddle state read\|write\|check` | `project_state.py check/read/write` |
| `huddle log append\|list`, `huddle timeline` | `huddle_writer.py` (ADR 14 §1) |
| `huddle config get\|set\|show` (global + repo `config.json`, ADR 19) | `config_helper.py` |
| `huddle context [--gh] [--agents]` | `repo_context.py`, lazy probes, crossmem (ADR 8) |
| `huddle persona list\|resolve\|new\|lint` | `synced_assets.py` + new flows (ADR 3–5) |
| `huddle persona ingest\|import\|discover\|propose\|analyze\|find\|style\|brief` | new — ADR 10 |
| `huddle <noun> new\|list\|show\|status\|lint` (intent, spec, plan, adr, …) | new — ADR 12, 18 |
| `huddle doctor` | new — environment + asset health |
| `huddle migrate` | `migrate.py` (also run implicitly, idempotent) |
| `huddle install\|uninstall --skills`, `huddle --version` | `install.sh`, `uninstall.sh`, `.cmd` |

## Consequences

- One install command on every OS; the skill and its helpers are always the same version.
- `SKILL.md` and step files lose every `{PYTHON_BIN}` invocation; `python_bin` detection is removed from global `config.json` (legacy `userconfig.json` read once on migrate).
- Release cost: six `package.json` version bumps per release — automate in the Taskfile (crossmem does it by hand today).
- `e2e/run.py` is replaced by `go test` (ADR 9).

## Open questions

- None open. (Analytics: not ported — ADR 11.)
