# ADR Stress Test vs AI-Native SDLC Playbook — v1 (ADR 19)

Counts only features in ADR 19 "Build in v1" (rows 1–19). C covered · P partial · M missing · O deliberately out. `rN` = ADR 19 row N.

## 1. Coverage (72 items)

| Stage | C | P | M | O |
|---|---|---|---|---|
| Plan | intent.md, `intent/` (r6); PO gate (r7) | git author/ts (never commits); metrics | non-engineer path | — |
| Design | spec.md (r6); TL gate (r7) | metrics; skills as constraints (brief, r5); version logging (hash, r4); policy owners (r7); intent-merge→spec job (draft, r19) | — | Claude Design mocks |
| Build | CLAUDE.md (r6) | plan gate (r7); plan↔diff sync, protected paths, fmt/lint, allow-list (drafts, r9); 2nd-mistake rule; SHA back-link (r4); metrics | skill plugins; worktree cap; team subagents | — |
| Test | evals, incident→eval (r6) | verify target; eval gate (draft, r12); test-edit block (draft, r9); metrics | OTel/PR check | UI visual check |
| Deploy | REVIEW.md (r6); scrubbing (r13); writer≠approver (r7) | Bugs/Security/Compliance passes; Nit tuning; findings→CLAUDE.md; settings gates, release gate, managed settings (drafts, r9); agent identity (r7); CI `claude -p` (draft, r19) | sandbox/tokens; DORA | `@claude` loop, MCP tiers, rollback, branch protection |
| Maintain | bands.yaml, σ tiers, detector, 3σ→intent (r11) | dismiss→tuning (r11); breach metric; post-mortem (r18) | security scans; runbooks | agent service |
| Cross | role model (r7) | chain of custody; secret denies (r13); skill sign-off (r17); drift evals (r12); anti-patterns; min-version (r2); managed-only (r9); CI transitions (r19) | OTel | infra checklist |

## 2. Edge cases

| Scenario | Sev | Status (ADR) |
|---|---|---|
| Agent self-approves gate | H | **Closed** — 18 §4, 16 §4 (r7) |
| Hook denies; user approves in chat | H | **Closed** — gate stays closed until logged override, 17 §5 (r8) |
| Hand-edit plan.md mid-build | H | **Closed** — artifacts are truth, drift reported, 14 §1, 6 (r4) |
| Worker edits test during fix | H | **Closed** — test first + protected paths, 16 §3, 17 §8 (r9) |
| Eval runner blocked | H | **Closed** — `unverified`, never PASS, 9 (r12) |
| Secret in ingested mail | H | **Closed** — redact before disk, 10 §8 (r13) |
| Pull edits blocking hook → skipped | H | **Closed** — no trust; failing blocking hook keeps gate closed, 17 §5–6 (r8) |
| Cloned repo `config.json` hooks run without approval | H | **Accepted by owner** (r8, no trust hashing); `start` change notice, 17 §6 |
| Two huddles, one change | M | Partial — `O_APPEND` log + session id, 6 §5; branch keying remains |
| Monorepo `intent/` root | M | Open — per-package `artifacts.*` root |
| No git fingerprint | M | **Closed** — content hash, 14 §6 |
| Windows `.sh` hooks | M | Open — argv exec + `shell:` |
| Spike timebox offline | M | **Closed** — checked on next call, 18 §13 |
| No subagents | M | **Closed** — `session` room + inline degrade, 15 §5 |
| Deny after `artifact.written` | M | **Closed** — blocking only on `artifact.draft`, 17 §5 |
| AGENTS.md vs CLAUDE.md drift | M | Open — hash check in `guardrails status` |
| CLAUDE.md > 1 page | L | Open — byte lint |

## 3. Contradictions

All resolved per ADR 19 "Resolved contradictions": `start` + 60 ms (1, 2, 8, 9, 11); `log append` (1, 6); `<noun> new` replaces `export`/`write` (7, 12); `artifacts.*` config (7, 12); `verify` replaces `docs/reviews/` (7, 18); three persona layers (3, 14 §5); generated roster/routing (9, 11); Deepak replaced (11); `session` room (14, 15, 16); conventions named (12 §5); WHEN/THEN → acceptance checks (16, 18); layout updated (6); hooks → `config.json`, trust dropped (6, 13, 17, 18, README). None open.

## 4. Over-engineering still in v1

- 30+ hook event types (17 §2): ship `stage.*`, `artifact.*`, `gate.*`, `verdict.issued` first.
- Index cache + 60 ms p50 (r2): Go already cold-starts fast; measure before building the cache.
- Everything else from the first pass is deferred in ADR 19.

## 5. Scorecard (v1-built only)

- C 16 · P 38 · M 10 · O 8 → **C+P 54/64 = 84 %** (gate ≥ 80 %: pass). First pass: 61 %.
- Open H edges: **0** (1 H accepted by owner decision: repo hooks without trust).
- Added to ADR 19 to reach the gate (all templates/lint, no LLM, no network): r9 extended (fmt/lint, allow-list, managed-settings snippet); r12 extended (`unverified` fallback); r17 `skill` noun; r18 `lessons` noun; r19 CI drafts (`claude -p` triage, intent→spec job).

Next cheapest gains: OTel-shaped `log list --otel` export; `sdlc status` worktree cap warning.
