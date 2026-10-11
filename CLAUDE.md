---
id: guardrails
kind: guardrails
status: draft
author: Muthukumaran Navaneethakrishnan
links:
  intent: intent-v1-readiness
---
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

<!-- Huddle convention: shared content also lives in AGENTS.md for non-Claude harnesses; keep the two in sync. -->
<!-- huddle:context -->
## Guardrails (huddle-managed block — `huddle guardrails lint`)

Huddle owns only this block. `AGENTS.md` stays authoritative for the rules; this block is
the per-stage, evidence-based detail an agent needs while editing, and the mistakes made here.

### Commands

Verified to exist in this repo (`Taskfile.yml`, `./bin/huddle --help`):

- `task build` → `bin/huddle` · `task test` → `go test ./...` · `task lint` → gofmt check + `go vet ./...`
- `task budget` → `TestSkillByteBudgets` (SKILL.md ≤ 6 KB, description ≤ 260 B, lazy file ≤ 8 KB)
- `task bench` → benchmarks, incl. `BenchmarkStart` · `task ci` → lint + `go test -short ./...` + budget + build + `--version` + `--help`
- Its own artifacts: `huddle sdlc status` · `huddle <noun> new|list|show|status|lint|link`
  for `intent spec plan adr spike guardrails review-policy eval bands skill lessons`
- `huddle bands check --series <csv>` · `huddle eval ci-draft` · `huddle stats` · `huddle log list` · `huddle serve`

### Conventions

- Stdlib only. No dependency is added for convenience; crossmem links only with `-tags crossmem`.
- `internal/paths` is the only place global paths are built; `internal/redact` is the only
  place user text is sanitised. Do not reimplement either at a call site.
- A new command group registers itself from `init()` in `internal/app/cmd_<group>.go`.
- Artifact writers never overwrite (`artifacts.ErrExists`) and never touch another tool's
  managed block. Every write path supports `--dry-run`.
- The committed artifact is the source of truth; `huddle-state.json` is a rebuildable cache.
- Tests set `HUDDLE_HOME`, `CLAUDE_SKILLS_DIR`, `AGENTS_SKILLS_DIR` to `t.TempDir()`.
  Never run `install --skills` or `migrate` against the real `~/.claude` while developing.
- Every `huddle <cmd> --flag` written in `skills/**` must exist in the registry — a test checks it.
- v1 scope is `docs/adr/19-v1-scope-and-value-gate.md`; deferred items need an accepted ADR change.

### Architecture

`cmd/huddle` is a thin main over `internal/app.Run`, which times and records one local usage
line per invocation and dispatches to a command registry populated by `init()`. Commands read
the repo identity from `internal/repo` (parses `.git` files, no `git` exec), merge global and
repo config in `internal/config`, and then work through `internal/artifacts` (kinds, templates,
discovery, structural lint), `internal/gates` (human approval with separation of duties),
`internal/sdlc` (where each change is), `internal/bands` (sigma tiering), `internal/personas`
(builtin < global < repo) and `internal/hooks`. `internal/eventlog` is the append-only audit
trail; `internal/usage` the local-only analytics `huddle stats` and `internal/serve` render.
`assets/` embeds personas and templates, `skills/` the skill family. Nothing on the
`huddle start` path calls a model or the network.

### Things Claude gets wrong

Each line is a mistake made in this repo, with the correct behaviour and the eval that now covers it.

- **Reported work done without building.** `internal/serve` was committed with five handlers
  registered on the mux but never defined, and an import of a `web` package absent from the
  tree. Run `go build ./...` and `go vet ./...` — the whole module, not the touched package —
  before saying done. Eval: `evals/tree-compiles.json`.
- **Ported a skill and dropped behaviour silently.** Six huddle modes existed in the legacy
  Python skill; three were carried over, and `grep -ri research skills/` was empty, so research
  requests fell back to the discussion flow. When porting, diff the behaviour list both ways.
  Eval: `evals/skill-mode-parity.json`.
- **Prepared a publish with no license.** `npm/package.json` and all five
  `npm/platforms/*/package.json` had no `license` field while the OIDC publish workflow was
  already wired. Check every manifest, not just the top-level one. Eval:
  `evals/npm-manifests-licensed.json`.
- **Preached the SDLC without practising it.** Until `intent/v1-readiness/`, this repo had no
  intent, no `REVIEW.md`, no `bands.yaml` and zero evals. Run `huddle sdlc status` on this repo
  before claiming the product demonstrates its own thesis.
- **Hand-writing an artifact the CLI owns.** Use `huddle <noun> new` and fill the file against
  its lint contract (`internal/artifacts/kinds.go` lists the required headings). If a verb
  cannot do the job, that is a finding to report, not a file to hand-roll.
- **Expecting `huddle guardrails new` to edit an existing CLAUDE.md.** It returns
  `ErrExists`; huddle only edits its own block. Take the body from
  `huddle guardrails new --dry-run`, place it between the `huddle:context` markers, then
  `huddle guardrails lint`.
- **Isolating HOME and assuming the real agent dirs are out of reach.** `huddle start`
  syncs the skills, and `runCore(t, root, "start")` goes through that path. `coreEnv`
  isolates `HOME`, but `CLAUDE_CONFIG_DIR` is an absolute path read from the ambient
  environment, so `HOME` cannot contain it: running the suite replaced the developer's
  own installed skills. A fallback that reads an absolute path from the environment
  escapes `HOME`-based isolation — neutralise the variable in `TestMain`
  (`internal/app/core_test.go`), not per test.
- **Raising a limit to make a change fit.** The skill byte budget, `huddle start` p50 ≤ 60 ms
  and the 2 KB start output are decisions (ADR 13, 19). Fix the cause.
<!-- /huddle:context -->
