# ADR 14: What Huddle Adopts from BMAD-METHOD and gstack

- Status: proposed
- Date: 2026-09-14
- v1: built: §1, §6, §7, §10; deferred: §2 non-`session` rooms, §3, §4, §5, §8, §9 learnings, §12 touchfiles (ADR 19)
- Amends: ADR 3 (merge), ADR 6 (event log), ADR 9 (evals), ADR 12 (writers)

Sources: BMAD-METHOD `6.13.0-next` (`~/Downloads/BMAD-METHOD-main`), gstack `1.84.1.0` (`~/Downloads/gstack-main`). Only adopt/adapt items are listed; everything else is explicitly out (end of file).

## Decision

### 1. Event log becomes the single source of truth (amends ADR 6)

- `huddle log append --type <t> --by <persona|user> [--refs …] "<text>"` — append-only JSONL per huddle; blind writes, never re-read mid-session (BMAD memlog).
- Types: `decision`, `assumption`, `question`, `claim` (`verified|disputed|unverified`), `lock`, `kill`, `learning`, `action`, plus lifecycle `supersede` and `redact` (gstack decision log). "Active decisions" are **computed**, with a bounded snapshot + compaction.
- Every `question` and `decision` carries `door: one-way|two-way`. **One-way doors are never auto-decided** — always handed to the user (gstack `one-way-doors`).
- **The log is the audit trail; committed artifacts are the source of truth** (ADR 19). The log records each artifact's path + hash; hand edits always win — drift is reported, never regenerated over. `huddle-state.json` is a derived, rebuildable cache. Secret/injection scan on write.

### 2. Run modes for the room (v1: `session` only; others deferred)

`huddle start --room session|auto|subagent|team`:
- `session` (default, cheapest): one model voices the selected personas.
- `auto`: spawn independent subagents only when independent views matter (contested one-way door, verification).
- `subagent`: one standing subagent per persona, room relayed each round — breaks same-context agreement.
- `team`: harness agent teams where available.
Per-persona optional `model:` and `capabilities:` front-matter fields are honoured when spawning (BMAD party members). An **outside-voice slot** lets the agent route one persona to a different model/harness for a cross-model second opinion and report overlapping vs unique points (gstack `/codex`).

### 3. Anti-consensus and pressure-test (deferred)

- Built-in lightweight roles added to the roster: **Option Generator, Claim Checker, Consensus Challenger, Loop Stopper** — they never vote; they hand the decision back (BMAD Anti-Consensus Club).
- New mode **pressure-test**: attack/defend/switch turns on one idea; ends with an explicit outcome event `HARDENED | KILLED | CLARIFIED` and a short artifact (BMAD forge-idea).

### 4. Named rooms (groups) (deferred)

`<repo>/.huddle/groups/<id>.md` and `~/.config/.huddle/groups/<id>.md`: `name`, `scene`, `members` (or open-cast: scene only, model casts), optional per-group memory brief. "Start the review crew" loads a saved room (BMAD party groups). Same layers + trust as personas.

### 5. Field-level keyed merge (deferred; no repo-user layer — three layers only, ADR 19)

- Scalars replace; tables deep-merge; lists of keyed items (`id`/`code`) merge by key; other lists append; nothing is silently removed (BMAD customize merge).
- Extension points in front matter: `persistent_facts` (supports `file:` globs), `on_start` / `on_wrap` actions (text instructions, not code).

### 6. Verdicts, review lenses, fingerprints

- Standard verdict for verification and wrap-up: **PASS | CONCERNS | FAIL** with reasons (BMAD readiness gate).
- Verification mode uses lenses served by the CLI: `adversarial` (min 10 findings), `edge-case`, `verification-gap`, `structure`; findings are canonical JSON (BMAD review lenses). Personas for a review are chosen from diff scope (gstack smart routing).
- Every verification/spec-review result is stamped with the git tree fingerprint (`git write-tree` of the working tree; no git ⇒ content hash of the folder); a changed tree marks it **stale** (gstack `gstack-review-log`, `gstack-evidence`).

### 7. Writers get stronger shapes (amends ADR 12)

- **Spec writer:** format is the playbook layout (ADR 12); add a deterministic "spec law" lint over `spec.md` (why present, capabilities have success signals, non-goals listed, no orphan scenarios) and a token target (900–1600 per change) (BMAD spec kernel).
- **ADR writer:** front matter adds `binds:` and `prevents:`; accepted parent ADRs are inherited invariants shown read-only to later huddles (BMAD architecture spine).
- **Guardrails writer:** SHA/date-stamped managed block between `<!-- huddle:context -->` markers; refresh produces a ledger per existing line — `retain | rewrite | relocate | automate | delete` — and never silently deletes human lines; prohibitions must name the alternative (BMAD project-context).

### 8. Divergence and elicitation catalogs (deferred)

- Discussion mode gets an explicit **diverge → converge** split.
- Elicitation methods and brainstorm techniques ship inside the binary as data; `huddle methods pick -n 5 --spread` / `huddle methods show <id>` serve them (BMAD 71 methods / 108 techniques catalogs). Never loaded whole (ADR 13).

### 9. Learnings, timeline, retro follow-through

- Typed learnings (`pattern|pitfall|preference`) with confidence, latest-per-key wins; top 3 injected by `huddle start` (gstack learnings).
- `huddle timeline` — cheap per-repo event timeline for resume and wrap-up.
- Wrap-up checks **open action items from earlier huddles** against git evidence (commits/PRs touching referenced paths) and routes each finding twice: what now, what prevents recurrence (BMAD retrospective).

### 10. Startup, next-step, questions

- `huddle start --json` is the single entry (ADR 13): `HUDDLE_PROTO`, repo, branch, open huddle, active decisions (snapshot), top learnings, pending trust, gh/agents cache ages, suggested mode.
- `huddle next` reads repo + log state and suggests the next mode/writer (BMAD `bmad-help` routing).
- Standard question shape: 2–3 options + `RECOMMENDATION: X because …`; confusion protocol = stop and ask, never guess (gstack).
- Planning-mode tie-break principles (completeness over shortcut when cheap, reversible first, match existing patterns, …) as a short embedded list (gstack autoplan).

### 11. Install and distribution hygiene (amends ADR 1)

- Host table grows: `.claude/skills`, `.agents/skills` (Codex, Cursor, Windsurf, Amp…), `.cline/skills`, `.agent/skills`, `.bob/skills`, OpenCode — each row with path + text rewrites (`CLAUDE.md`↔`AGENTS.md`) (gstack `hosts/*.ts`, BMAD tool list).
- `.huddle-owned` marker: install/uninstall never touch a skill directory Huddle did not create.
- `removals` list embedded: renamed/removed skill dirs from earlier Huddle versions are pruned on install.
- `huddle digest` — ~2 KB instructions-only fallback for harnesses without skill support.

### 12. Evals (amends ADR 9)

- Three tiers like gstack: free static (asset lint, budgets, freshness) on every PR; gated e2e (stream-json transcript assertions) on touched areas; periodic LLM-judge (clarity, one-way-door respected, no premature consensus).
- **Touchfile selection:** each eval declares globs (`assets/modes/verification.md`, `assets/personas/**`); CI runs only evals whose touchfiles changed; global files run all.

## Explicitly not adopted

Menu codes per agent · theatrical keepsake HTML / fourth-wall fiction · web bundles (Gems/GPTs) · sprint-status / epics / stories / TEA / game-dev modules (the plan artifact covers delivery) · Jinja step-file renderer (BMAD is leaving it) · persistent daemons · browser/design/deploy/safety skill families · remote telemetry · model-specific prompt overlays (revisit only if evals show model drift).

## Context

- BMAD's strengths Huddle lacks: typed append-only memlog with derived artifacts, room run-modes and anti-consensus roles, keyed customization merge, stronger spec/architecture/project-context shapes, catalogs served by scripts.
- gstack's strengths Huddle lacks: single start call with protocol handshake, lazy sections + enforced token budgets, event-sourced decisions with one-way doors, fingerprint-bound reviews, learnings, host table + ownership marker, tiered evals with touchfile selection.
- Both converge on the same lesson Huddle is adopting in ADR 13: keep the always-loaded surface tiny, push deterministic work into a CLI, let the agent do judgment.

## Consequences

- New CLI surface: `log`, `methods`, `timeline`, `next`, `digest`, `start --room`, group files — all LLM-free and fixture-testable.
- Roster grows by four lightweight anti-consensus roles; they are style cards, not full personas, to keep token cost flat.
- Huddle-state and writer outputs become reproducible from the log, which makes ADR 9 parity and eval assertions far simpler.

## Open questions

- Resolved (ADR 19): default room is `session`; anti-consensus roles deferred.
