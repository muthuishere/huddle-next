# CLAUDE.md

Read `AGENTS.md` first — it is authoritative.

## What this is

`huddle` is the AI-native SDLC orchestrator: a Go CLI and a skill family (`huddle`, `huddle-sdlc`, `huddle-personas`) that take a team through setup → intent → spec → plan → build → test → deploy review → maintain, with expert personas for judgment, subagents for the work, and a human gate at every stage. Artifacts use the AI-native SDLC playbook's own files (`intent/<slug>/{intent,spec,plan}.md`, `CLAUDE.md`, `REVIEW.md`, `evals/`, `bands.yaml`), plus ADRs and spikes.

## Commands

```bash
task build      # bin/huddle
task test       # go test ./...
task lint       # gofmt + go vet
task budget     # skill byte budgets
task bench      # start / resolve benchmarks
task ci         # what .github/workflows/ci.yml runs
```

Linking the crossmem library locally: keep a gitignored `go.work` with `use (. ../../crossmem-workspace/crossmemcli)` and build with `go build -tags crossmem ./cmd/huddle`.

## Architecture

- `cmd/huddle` → `internal/app.Run`. Each command group registers itself from `init()` in `internal/app/cmd_<group>.go`.
- `internal/paths` is the only place global paths are built (`~/.config/.huddle`, override `HUDDLE_HOME`).
- `internal/repo` identifies the repo by reading `.git` files — no `git` exec on the hot path.
- `internal/config` merges global and repo `config.json` (repo wins; hooks concatenate).
- `internal/eventlog` is the append-only audit log; `internal/redact` is the single secret choke point.
- `internal/personas` (builtin < global < repo), `internal/artifacts`, `internal/gates`, `internal/sdlc`, `internal/bands`, `internal/hooks`, `internal/install`.
- `assets/` embeds personas and templates; `skills/` embeds the skill family.

## Distribution

GoReleaser builds darwin/linux amd64+arm64 and windows amd64, copies binaries into `npm/platforms/*/bin/`, and the `v*` tag workflow publishes `@muthuishere/huddle-<os>-<arch>` then `@muthuishere/huddle` with npm OIDC provenance. Bump versions with `scripts/npm-set-version.sh`.

## Decisions

`docs/adr/` — start with `README.md`, `16-ai-native-sdlc-orchestrator.md`, and `19-v1-scope-and-value-gate.md`.
