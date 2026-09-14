# Stage 2: Design — requirements and design in one session

**Artifacts:** `intent/<slug>/spec.md` (requirements with acceptance checks, design, flagged
policy concerns) + `docs/adr/<n>-<slug>.md` for decisions. **Gate:** spec review verdict
PASS/CONCERNS/FAIL; one-way doors go to the relevant `owners` approver.

## Detect

- `huddle sdlc status --json` → the intent must exist; if not approved, warn (don't block).
- `huddle spec status --json`, `huddle adr list --status proposed --json`.

## Interview (≤ 7)

1. For each outcome in the intent: what must be true? (becomes a requirement)
2. How will we check each one? (acceptance check — a test, eval, or metric; no check = not a requirement)
3. Which design options are on the table? (≥ 2 for any one-way door)
4. Security, privacy, compliance concerns? (Senthil lens)
5. Failure modes and what the user sees when things fail?
6. Non-goals?
7. Which decisions need an ADR? (anything hard to reverse)

## Delegate (parallel)

- **Spec writer:** `huddle spec new <slug> --dry-run` → draft in drafts dir, target
  900–1600 tokens, returns path + required-heading check (same rules as `huddle spec lint`).
- **Lens subagents** (security, compliance, UX) each read the draft and return findings as
  JSON `{lens, severity, requirement, concern, fix}`.
- **ADR writer** per one-way decision: `huddle adr new "<title>" --dry-run` → draft.

## Room

Suren (design soundness) + Senthil or Suna depending on the lens findings; Elango's lens on
spec clarity. Each gives one view, then STOP.

## Gate

1. Propose a verdict: PASS (every requirement has a check, no high findings),
   CONCERNS (gaps the user accepts), FAIL (high findings or unchecked requirements).
2. One-way doors: present options + RECOMMENDATION; the user decides; log with `--door one-way`.
3. On the user's yes: `huddle spec new <slug> --from <draft>`, `huddle adr new "<title>" --from <draft>`.
4. `huddle gate request spec-<slug>` → owner approves; ADRs get their own `huddle gate request <adr-id>`.
   Suggest the commit.

## What good looks like

```markdown
### R2 Refund reaches terminal state
Every refund is `succeeded` or `failed` within 60s of request.
**Check:** `TestRefundTimeoutMarksFailed` + eval `refund-timeout.json`.
**Policy:** provider errors logged without card data (PCI).
```
