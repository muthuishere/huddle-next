# ADR 11: Cut or Lazy-Load Everything That Costs Time

- Status: proposed
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **Rule: a feature stays on the default huddle path only if it earns its latency.** Measure time-to-first-persona-response and instruction tokens read per start; anything else is on-demand or removed.
2. **Per-step analytics calls are removed.** The 2026-09-13 OpenCode huddle spent **7 separate shell calls** on `analytics_log.py` (`skill-invoked`, `read-workflow`, `preflight`, `init-and-discussion`, `benchmark`, `persona-refresh`, `decision-round`, `build-readiness-re…`) — each a tool round-trip and a Python start. The uncommitted analytics block in `SKILL.md`, `scripts/analytics_log.py`, and `assets/logrepo.json` are not ported. If usage counts are ever wanted, `huddle` records one local line per command it already runs — zero extra tool calls.
3. **Visual/diagram surfaces stay out.** Graph review was already removed (`a465bbf`); reviews and summaries remain text. No Mermaid, Excalidraw, or diagram generation in the default flow — a user who wants one asks, and a separate skill does it.
4. **Removal candidates — decided one by one in a huddle, default = remove from the embedded default skill:**

   | Feature | Size today | Cost | Proposal |
   |---|---|---|---|
   | Sreyash/Hari/Harshvardhan builder flow (`step-sreyash-*`, `step-sreyash-build.md`, `dispatch-table.md`) | ~1,110 lines | loaded whenever build/dispatch routing triggers; overlaps plan mode + subagents | **remove**; builds happen in the harness's own plan mode / subagents with an ADR 7 `plan.md` |
   | Deepak project documentation (`step-deepak-document.md`) + doc-freshness at preflight | 186 lines + preflight scan | extra probe every start; hijacked first turns before `0afeeb0` | **replaced** by the guardrails writer (below); drop freshness from startup |
   | Elanchezian progressive brainstorm (`step-elanchezian-brainstorm.md`) | 186 lines | read only when triggered | **keep, lazy** |
   | `activation-routing.xml` + `step-02-discussion.md` read on every start | ~560 lines | several read rounds per huddle | **generate**: routing/roster index from front matter (ADR 13 §18); `huddle start --with-assets` returns the detected mode file + selected style cards |
   | Full persona bodies for the whole roster | 21 files | read per round in some flows | **selected personas only**, via `huddle persona style` cards (ADR 10) |
   | Cross-branch context scan at preflight | sibling branch folders | grows with branches | **summary count only**; details on ask |
   | Huddle-private spec formats (Elango's older layout, `step-sreyash-2-spec.md`, `docs/specs/*` style) | spec + builder steps | superseded by the playbook layout | **remove**; playbook artifacts replace them (ADR 12) |
   | Deepak "document the project" flow | see above | overlaps guardrails writer | **replace** with the guardrails writer (ADR 12) |
   | `usage.md`, `docs/specs/*` historic specs | docs | not loaded at runtime | keep in repo, **not embedded** in the binary |

5. **Deleted means deleted:** removed flows go from `references/`, the roster flags, routing XML, and ADR 9 lint (which then fails if anything still references them). No dead routes left behind.

## Context

- The owner's feedback: the skill is slow, and diagrams and "so many stuff" take time — remove what isn't needed.
- ADR 2 fixes the ~700 ms network wait, but the larger wall-clock cost is agent rounds: instruction reads, per-step analytics, and flows that load on routing edge-cases.
- Playbook anti-pattern: stale or bloated agent context reduces effectiveness; keep the always-loaded surface small.

## Consequences

- Smaller embedded asset set, fewer tool rounds per huddle, fewer contradictions for ADR 9 lint to police.
- Anyone relying on the Sreyash builder flow loses it; the replacement is the harness's plan mode plus `plan.md` and a task brief (ADR 16 §3).

## Verification

- Before/after on the same prompt ("start a huddle on X"): count tool calls and seconds to the first persona response. Target: ≤ 3 tool calls, and fewer instruction tokens read than today.

## Open questions

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §1 the latency rule as enforced budgets (ADR 13, `internal/install/budget_test.go:28`);
§2 per-step analytics calls are gone — no `analytics_log.py` or `logrepo.json` in the tree, and usage
is one local line written by the binary itself (`internal/usage/usage.go`, ADR 20 §4); §3 no diagram
surface in the default flow.

**Not landed — §5 "deleted means deleted" is violated by §4's own first row:**

- The Sreyash / Hari / Harshvardhan builder personas are still embedded and still route to step files
  that no longer exist: `assets/personas/sreyash/sreyash.md:23,25`,
  `assets/personas/hari/hari.md:25-26,40`, `assets/personas/harshvardhan/harshvardhan.md:23` all cite
  `references/steps/step-sreyash-{build,1-init,2-spec,3-process,4-wrap}.md`, and
  `assets/personas/vel/vel.md:36` cites `references/dispatch-table.md`. There is no `references/`
  directory. These are precisely the "dead routes left behind" §5 forbids.
- The ADR 9 lint that §5 relies on to catch this only parses `huddle` subcommands
  (`internal/app/skills_sync_test.go:28`), so nothing fails. A lint over `references/**` paths cited
  in embedded assets is the missing piece.
