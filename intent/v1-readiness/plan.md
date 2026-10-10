---
id: plan-v1-readiness
kind: plan
status: draft
author: Muthukumaran Navaneethakrishnan
links:
  intent: intent-v1-readiness
  spec: spec-v1-readiness
---
# v1 readiness — plan

## Files that change

- `intent/v1-readiness/intent.md` — the problem: huddle keeps no artifact chain for itself.
- `intent/v1-readiness/spec.md` — R1–R5 with acceptance commands.
- `intent/v1-readiness/plan.md` — this file.
- `REVIEW.md` — new; this repo's real review policy (`task ci` as the check, redaction
  choke point, separation of duties on gates).
- `bands.yaml` — new; one metric, `huddle-start-p50-ms`, baseline measured here.
- `evals/skill-mode-parity.json` — new; the six-modes-to-three parity gap.
- `evals/tree-compiles.json` — new; `internal/serve`'s undefined handlers and the
  non-existent `web` import.
- `evals/npm-manifests-licensed.json` — new; the missing `license` field in all six npm
  manifests.
- `CLAUDE.md` — huddle's own `<!-- huddle:context -->` block plus the front matter its
  lint requires. Nothing outside the markers is restructured.

Not touched by this workstream: `AGENTS.md`, `docs/adr/**`, `skills/**`, `internal/**`,
`npm/**`.

## Order of work

1. Read `AGENTS.md`, `CLAUDE.md`, and ADRs 7, 9, 12, 16, 18, 19; then every noun's
   `--help`. The layout and the required headings come from the ADRs, not from taste.
2. `task build` so `bin/huddle` is current, and capture `huddle sdlc status` as the
   before state.
3. Measure before writing any band: `huddle start` ×25 → `huddle stats` for p50/p95,
   a wall-clock loop for the exec-included number, `go test -bench BenchmarkStart`,
   `wc -c` on the start output and on every `SKILL.md`. Read `internal/usage.Budgets`.
4. Create the chain with the CLI: `intent new`, `spec new`, `plan new` (links chain
   themselves), then `review-policy new`, `bands new huddle-start-p50-ms`,
   `eval new` ×3.
5. Fill each file against its lint contract, preserving the front matter the CLI wrote.
6. Prove the evals: execute every `type: command` check and confirm it exits 0 on this
   tree — a check that cannot run is not a check.
7. Smoke-test `bands check --series` on a 12-point series; confirm the breach path is a
   dry run and writes no intent file.
8. CLAUDE.md last: `guardrails new --dry-run` for the exact template, insert the block,
   `guardrails lint`.
9. Verify: `huddle sdlc status`, every `<noun> lint`, `go test ./...`, `task lint`,
   `task budget`, `git diff --stat` to confirm nothing outside this workstream moved.
10. Report the findings — the broken and rough CLI edges are an output of this work, not
    a side effect.

## Risks

- **Concurrent edits.** `skills/**` and `docs/adr/**` are being changed by other agents
  on this branch; touching them would collide. Mitigation: own only the files listed
  above, and check `git status` before and after.
- **Guardrails writing over a hand-maintained CLAUDE.md.** `artifacts.Create` refuses to
  overwrite, which is the safe failure, but the block still has to be placed correctly.
  Mitigation: only the fenced block is inserted; the pre-existing sections stay as they
  are, and `guardrails lint` proves the block is well-formed.
- **Invented baselines.** A band with a made-up mean is worse than no band. Mitigation:
  every number in `bands.yaml` cites the command that produced it, and the baseline is
  computed from the series (window) rather than pinned.
- **Evals that only look rigorous.** A check nobody can execute is decoration.
  Mitigation: every `command` check was run; transcript assertions are marked as such so
  they are never mistaken for automated ones.
- **Front matter in CLAUDE.md confusing an agent.** Mitigation: metadata only, four keys,
  above the content; the body is unchanged prose.

## Proof

- Before: `huddle sdlc status` → `CLAUDE.md: present (no huddle block)`, `REVIEW.md:
  missing`, `bands.yaml: missing`, `evals: 0`, `No intents yet.`
- After: the same command shows the huddle block, REVIEW.md and bands.yaml present,
  `evals: 3`, and the `v1-readiness` row with intent, spec and plan.
- `huddle intent lint`, `spec lint`, `plan lint`, `review-policy lint`, `bands lint`,
  `eval lint` — all ok.
- Every `type: command` check in `evals/*.json` exits 0 (run individually).
- `go test ./...`, `task lint`, `task budget` pass; `task build` succeeds and
  `./bin/huddle --version` answers.
- `huddle log list` shows one artifact write event per created artifact.

Update this plan in the same commit if the implementation departs from it.
