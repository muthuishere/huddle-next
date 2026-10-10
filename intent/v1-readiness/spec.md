---
id: spec-v1-readiness
kind: spec
status: draft
author: Muthukumaran Navaneethakrishnan
links:
  intent: intent-v1-readiness
  adrs: adr-19
  eval: eval-tree-compiles
---
# v1 readiness — spec

## Requirements

### R1: huddle's own repo passes its own status check

`huddle sdlc status` reports a huddle block in CLAUDE.md, REVIEW.md present,
bands.yaml present, evals ≥ 3, and one intent with intent/spec/plan all present.

Acceptance: `./bin/huddle sdlc status` shows `CLAUDE.md: present (huddle block)`,
`REVIEW.md: present`, `bands.yaml: present`, `evals: 3` or more, and the
`v1-readiness` intent row with intent, spec and plan non-missing.

### R2: every artifact is created by the CLI, and every lint is clean

Each artifact comes from `huddle <noun> new`; nothing in the set is a hand-created file
that the CLI could have produced. Any verb that cannot do the job is written up as a
finding rather than worked around.

Acceptance: `huddle intent lint && huddle spec lint && huddle plan lint &&
huddle review-policy lint && huddle bands lint && huddle eval lint` all report ok, and
the repo's event log shows an `artifact.write` event per created artifact
(`huddle log list`).

### R3: three evals encode defects that actually happened in this repo

Each eval names the real defect in an `origin` field and carries at least one check that
is executable and would have failed on the defective tree: skill-mode parity, the tree
compiling, and npm manifests carrying a license.

Acceptance: `evals/skill-mode-parity.json`, `evals/tree-compiles.json` and
`evals/npm-manifests-licensed.json` exist, lint clean, each has an `origin` naming the
defect, and every `type: command` check exits 0 on the current tree.

### R4: control bands are measured, not invented

`bands.yaml` tracks one metric with a baseline traceable to a measurement taken in this
repo, and cites where the number came from.

Acceptance: `bands.yaml` lints clean, `huddle bands check --series <file>` runs and
tiers a series, and every number in the file is reproducible from a command named in the
file (`huddle stats --json`, `wc -c`, `go test -bench`).

### R5: nothing else regresses

Acceptance: `go test ./...` and `task lint` pass; `task budget` passes; the non-huddle
parts of `CLAUDE.md` and all of `AGENTS.md`, `docs/adr/**` and `skills/**` are unchanged
by this workstream (`git diff --stat`).

## Design

Five artifacts, each produced by its own CLI noun and then filled in place:

- `huddle intent|spec|plan new "v1 readiness"` → `intent/v1-readiness/{intent,spec,plan}.md`.
  The CLI chains the front-matter links itself (spec → intent, plan → intent + spec).
- `huddle review-policy new` → `REVIEW.md`, filled with this repo's real passes: `task ci`
  as the gate, redaction as the single choke point, separation of duties on gates.
- `huddle bands new huddle-start-p50-ms` → `bands.yaml`, baseline from huddle's own usage
  log (`internal/usage`), budget cross-checked against `usage.Budgets` and ADR 19.
- `huddle eval new <slug>` ×3 → `evals/*.json`, each with `origin`, executable `command`
  checks, and transcript assertions for the part a shell cannot see.
- `huddle guardrails new` for the CLAUDE.md block. CLAUDE.md already exists, so the write
  path returns `ErrExists`; the CLI's own message is the documented route ("huddle only
  edits its own block: add the markers by hand, then run `huddle guardrails lint`"), so the
  template body comes from `guardrails new --dry-run` and goes inside the markers. The
  rest of CLAUDE.md is untouched.

Lint is the contract in each case: the required headings in `internal/artifacts/kinds.go`
define "done" for the markdown artifacts, `bands.ParseConfig` for bands, and
prompt+checks for evals.

## Policy concerns

- Adding huddle's front matter (`id`, `kind`, `status`, `author`) to `CLAUDE.md` is
  required by the guardrails lint but makes a YAML block visible at the top of a file
  every agent reads. It is metadata, not instructions, and it sits above the content.
- `bands check` appends to the event log and, on a breach, drafts an intent. Smoke-testing
  it on a synthetic series therefore leaves a real event in the log; the run used the
  default dry-run (no `--write`) so no intent file was created.
- The evals' shell checks run `go build`, `go vet` and `task build` — safe, local,
  network-free, and they must stay that way: an eval that needs credentials belongs in
  the `unverified` class per ADR 9.
- No secret, no credential path and no `.env` reference enters any artifact.

## Non-goals

- No product code change, no new CLI verb, no fix for the findings this work surfaces
  (multi-metric bands, missing per-command budgets) — those are findings, and any fix
  needs its own intent and an ADR if it changes a decision.
- No eval runner, no CI workflow committed (`eval ci-draft` output is not adopted here).
- No gate approval: `huddle gate approve` refuses agent sessions by design, so the gates
  for these artifacts are the owner's to open and close.
- No ADR edit, no skill edit, no release, no commit.

## Links

- Intent: `intent-v1-readiness`
- ADRs: `docs/adr/9-parity-lint-and-evals.md` (evals are a requirement),
  `docs/adr/12-writers-on-playbook-layout.md` (artifact layout and writer rules),
  `docs/adr/18-cli-first-artifact-catalog.md` (one noun per artifact),
  `docs/adr/19-v1-scope-and-value-gate.md` (the value gate this closes),
  `docs/adr/7-huddle-in-the-sdlc-artifact-chain.md` (the chain itself)
- Evals: `evals/skill-mode-parity.json`, `evals/tree-compiles.json`,
  `evals/npm-manifests-licensed.json`
- Bands: `bands.yaml` · Review policy: `REVIEW.md`
