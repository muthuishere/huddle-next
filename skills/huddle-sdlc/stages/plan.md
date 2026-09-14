# Stage 1: Plan — turn an idea, complaint, or finding into an intent

**Artifact:** `intent/<slug>/intent.md` (Author · Status · Problem · Proposed outcome ·
Affected users and systems · Constraints · Open questions). **Gate:** the product owner accepts the intent.

## Detect

- `huddle intent list --json` → is there already an intent for this? Extend, don't duplicate.
- `huddle log list --type decision,question --limit 20 --json` → prior decisions that constrain it.

## Interview (≤ 7, one at a time, each with RECOMMENDATION)

1. What problem, for whom, observed how? (evidence, not solution)
2. What outcome would tell us it's solved? (measurable if possible)
3. Who and what is affected? (users, services, data)
4. Constraints: deadline, budget, compliance, tech we must keep?
5. What's explicitly out of scope?
6. What don't we know yet? (becomes Open questions — mark each one-way/two-way)
7. Who owns it? (defaults to `owners.product`)

## Delegate (parallel)

- **Code scout:** where does this live today? ≤ 400 tokens, file:line.
- **History scout:** `huddle context --agents` → what was tried or decided before in other agent sessions.
- **Intent writer:** `huddle intent new "<title>" --dry-run` for path/template, drafts to the drafts dir,
  returns path + required-heading check (same rules as `huddle intent lint`) + 5-line summary.

## Room

Prabagar (is the outcome real?) + Babu (is the demand real?) + the domain expert if technical.
Personas critique the draft, then STOP.

## Spikes

A one-way open question that no one can answer from the repo → propose a spike:
`huddle spike new "<question>" --timebox 1d`. The user starts it. A finished spike must end in
an ADR, an intent change, or a plan amendment (`huddle spike link <spike-id> --to <adr-id|intent-id|plan-id>`).

## Gate

1. Show the draft intent in full.
2. On the user's yes: `huddle intent new "<title>" --from <draft>` (status draft).
3. `huddle gate request intent-<slug>` → the product owner approves in their terminal
   (`huddle gate approve <gate-id>`). You never run approve. Log it; suggest the commit.

## What good looks like

```markdown
## Problem
Refunds over $500 fail silently when the provider times out; 14 tickets in August.
## Proposed outcome
Every refund reaches a terminal state within 60s, visible to support.
## Open questions
- [one-way] Do we need provider-side idempotency keys?
```
