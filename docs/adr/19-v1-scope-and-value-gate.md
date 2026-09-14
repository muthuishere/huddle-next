# ADR 19: v1 Scope — Build Only What Scores; Everything Else Stays in the ADRs

- Status: proposed
- Date: 2026-09-14
- Overrides: scope of ADRs 1–18 for the first release. Deferred items remain valid design, not v1 work.

## Value gate (must hold before release)

- **Playbook value score ≥ 80 %** = playbook items Covered + Partial by *v1-built* features ÷ playbook items not deliberately out (method: `stress-test.md`).
- **0 high-severity edges** open from `stress-test.md`.
- **Performance:** `huddle start` p50 ≤ 60 ms / p95 ≤ 150 ms warm; first persona response ≤ 3 tool calls; `SKILL.md` ≤ 6 KB; `start` output ≤ 2 KB. Enforced by Go tests/benchmarks.

## Build in v1

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

## Deferred — stays in the ADRs only

Room modes beyond `session` · anti-consensus roles · pressure-test mode · saved groups · elicitation/brainstorm catalogs · persona `discover`/`propose`/`analyze` · trust hashing (repo personas included; revisit if cloned-repo personas become common) · `huddle watch` · `context-bill` · touchfile eval selection + LLM judge · agent-definition install · `gh` context cache (use hooks) · build dispatch automation (v1: `plan` + task brief only; harness does the build) · release-notes writer · skill drafting subagent · `custom` spec path mapping.

## Resolved contradictions (applies across ADRs)

- Startup command is `huddle start`; budget p50 ≤ 60 ms.
- Event writer is `huddle log append`; `event add` and `export` are removed — artifacts are created with `huddle <noun> new`.
- Config keys live under `artifacts.*`, `owners.*`, `hooks[]` in `config.json`.
- Review notes are the `verify` record (no `docs/reviews/`).
- Personas have three layers only (builtin, global, repo).
- The roster and routing are generated from front matter; no hand-kept XML.
- Deepak's doc flow is replaced by the guardrails writer.
- The default room is `session`.
- Spikes, `.huddle/` config, and `AGENTS.md` are marked Huddle conventions beyond the playbook.
