---
id: adr-19
kind: adr
status: approved
author: muthuishere
links: {}
---
# ADR 19: v1 Scope — Build Only What Scores; Everything Else Stays in the ADRs

- Date: 2026-09-14
- Overrides: scope of ADRs 1–18 for the first release. Deferred items remain valid design, not v1 work.

## Status

**approved** — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw
decision: `README.md` → Inputs). The front matter `status:` field is the status of record.

## Context

ADRs 1-18 accumulated more design than one release can carry, and several of them contradict each
other (three layers or four, `export` or `<noun> new`, a room mode per situation). Shipping all of
it would mean shipping none of it well, so scope is decided once, in one place, by a stated measure
rather than by enthusiasm: `stress-test.md` scores how much of the AI-native SDLC playbook v1-built
features actually cover, and that score is the gate.

## Decision

### Value gate (must hold before release)

- **Playbook value score ≥ 80 %** = playbook items Covered + Partial by *v1-built* features ÷ playbook items not deliberately out (method: `stress-test.md`).
- **0 high-severity edges** open from `stress-test.md`.
- **Performance:** `huddle start` p50 ≤ 60 ms / p95 ≤ 150 ms warm; first persona response ≤ 3 tool calls; `SKILL.md` ≤ 6 KB; `start` output ≤ 2 KB. Enforced by Go tests/benchmarks.

### Build in v1

| # | Capability | Commands | From ADR |
|---|---|---|---|
| 1 | Single binary, npm + goreleaser, skill install via crossmem `pkg/skillinstall` | `install/uninstall --skills`, `--version`, `doctor` | 1 |
| 2 | One startup call, no network, index cache, `HUDDLE_PROTO` | `start --json` (replaces `preflight`) | 2, 13 |
| 3 | Config at two levels only: `~/.config/.huddle/config.json` (global) and `<repo>/.huddle/config.json` (repo). Repo overrides global, key by key. | `config get|set|show` | 6, 17 |
| 4 | **Event log = audit trail; committed artifacts = source of truth.** Log stores events + artifact path/hash; `huddle-state.json` is a derived cache, rebuildable, never truth. Hand edits to artifacts always win. | `log append|list`, `timeline` | 6, 14 |
| 5 | Personas: builtin < global < repo, deterministic merge, style cards | `persona list|resolve|style|brief|new|import|ingest` | 3, 4, 10 |
| 6 | Playbook artifacts as CLI nouns + lint + templates | `intent|spec|plan|adr|spike|guardrails|review-policy|eval|bands new|list|show|status|lint` | 12, 18 |
| 7 | **Owners + gates with separation of duties:** `owners` in config (product, tech-lead, code, service, policy). A gate needs a human approver who is not the artifact's writer; approvals made by an agent session are rejected (`HUDDLE_AGENT=1` / non-TTY without `--as <owner>`). | `gate request|approve|reject|status`, `sdlc status|next` | 16, 18 |
| 8 | **Hooks in config.json** (repo or global — the user owns them, no trust hashing): `hooks: [{on, match, run, blocking, timeout, async}]`, JSON in/out. A blocking hook that denies or fails ⇒ the gate stays closed until a human overrides with `gate approve --override --reason` (logged). | `hooks list|test` | 17 |
| 9 | `hooks export --host claude` pack (drafts for the user): protected paths, no test-file edits during fixes, plan↔diff sync check, production gate, post-edit format/lint, inner-loop `permissions.allow`, managed-settings snippet (deny `.env*`/`secrets/**`, `disableBypassPermissionsMode`) | `hooks export` | 17 |
| 10 | Verdicts stamped with tree fingerprint; stale on change | `verify record|status` | 14 |
| 11 | `bands check`: deterministic mean/σ (1σ log, 2σ diagnose, 3σ draft intent) on a user-supplied metric series; dismissals need `--reason` | `bands check` | 12, 16 |
| 12 | Eval CI draft: workflow runs evals when `CLAUDE.md` / `.claude/**` / `evals/**` change, pass-rate threshold; no authenticated runner ⇒ result `unverified`, never PASS | `eval ci-draft` | 9, 12 |
| 13 | Redaction on every ingest and hook output before storage or agent | (internal) | 8, 10 |
| 14 | Skill family: `huddle`, `huddle-sdlc`, `huddle-personas` routers + lazy mode/stage files | — | 15 |
| 15 | Cross-agent context via crossmem library (lazy) | `context --agents` | 8 |
| 16 | Migration from `~/.config/muthuishere-agent-skills` (copy, never overwrite) | `migrate` | 6 |
| 17 | Skill noun: template + lint; changes gated by the `policy` owner (drafting by subagent stays deferred) | `skill new\|list\|show\|lint` | 12, 18 |
| 18 | Lessons noun (post-mortem → eval + intent links), `docs/lessons/<date>-<slug>.md` | `lessons new\|list\|show` | 16, 18 |
| 19 | CI drafts, templates only: read-only `claude -p` triage job; intent-merge → spec-draft job | `ci draft triage\|spec` | 12, 16 |

### Deferred — stays in the ADRs only

Room modes beyond `session` · anti-consensus roles · pressure-test mode · saved groups · elicitation/brainstorm catalogs · persona `discover`/`propose`/`analyze` · trust hashing (repo personas included; revisit if cloned-repo personas become common) · `huddle watch` · `context-bill` · touchfile eval selection + LLM judge · agent-definition install · `gh` context cache (use hooks) · build dispatch automation (v1: `plan` + task brief only; harness does the build) · release-notes writer · skill drafting subagent · `custom` spec path mapping.

### Resolved contradictions (applies across ADRs)

- Startup command is `huddle start`; budget p50 ≤ 60 ms.
- Event writer is `huddle log append`; `event add` and `export` are removed — artifacts are created with `huddle <noun> new`.
- Config keys live under `artifacts.*`, `owners.*`, `hooks[]` in `config.json`.
- Review notes are the `verify` record (no `docs/reviews/`).
- Personas have three layers only (builtin, global, repo).
- The roster and routing are generated from front matter; no hand-kept XML.
- Deepak's doc flow is replaced by the guardrails writer.
- The default room is `session`.
- Spikes, `.huddle/` config, and `AGENTS.md` are marked Huddle conventions beyond the playbook.

## Consequences

- Every other ADR is read through this one: an approved-looking clause may still be out of v1, and
  the ADR's own `v1:` line says which.
- The deferred list is a commitment, not a backlog: building one of those items needs an approved
  amendment here first, which keeps scope from drifting one convenient exception at a time.
- The value gate makes "ready" checkable by a test run rather than arguable, and it can fail a
  release that otherwise looks finished.
- Where an ADR and this one disagree about scope, this one wins — so the ADRs stay honest records of
  the decision made at the time instead of being rewritten after the fact.

## Open questions

### Accepted, 2026-10-10 (event `20261010T045742.452Z-01d288`)

The scope decision was adopted and its value gate holds: `stress-test.md:51` scores C+P 54/64 = 84 %
against the ≥ 80 % gate with 0 open high-severity edges, and the performance clause is enforced by
tests (`internal/app/core_test.go:359` p50 < 60 ms; `internal/install/budget_test.go:12-18` for
SKILL.md ≤ 6 KB, description ≤ 260 B, lazy files ≤ 8 KB; `internal/app/cmd_start.go:35`
`startBudgetBytes = 2048`). Rows 1-14 and 16-19 all ship commands; `huddle --help` is the inventory.

Three rows need an amendment rather than more code:

- **Row 2's index cache was deliberately not built.** `stress-test.md:46` calls it over-engineering
  and says measure first; the p50 test passes without it. Drop it from row 2 or move it to deferred.
- **Row 15 (crossmem library) ships as a shell-out in released binaries.** The library path is behind
  `-tags crossmem` (`internal/app/context_crossmem.go:1`) and `.goreleaser.yaml` passes no tags, so
  the default is `internal/app/context_cli.go` exec'ing the CLI. See ADR 8's note.
- **"§9 learnings" is listed as deferred but is built** — typed learnings with latest-per-key and
  injection at `huddle start` (`internal/app/cmd_log.go:35`, `internal/app/cmd_start.go:333-359`),
  plus `huddle timeline`. See ADR 14's note.

Also unaccounted for by this ADR either way: ADR 14 §11 install hygiene (`.huddle-owned` is built,
`removals` and `huddle digest` are not) and ADR 13 §C14 served catalogs (not built; belongs with the
deferred ADR 14 §8).

## binds

- v1 is exactly the "Build in v1" list above, held to the value gate: playbook value score ≥ 80 %,
  0 high-severity edges, and the stated performance budgets, each enforced by a test.
- Deferred items stay valid design and stay unbuilt; where ADRs contradict each other, the
  "Resolved contradictions" section wins.

## prevents

- No deferred item is built without an approved change to this ADR.
- The value gate is never lowered to let a release through.
