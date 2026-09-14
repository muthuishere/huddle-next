# Stage 5: Deploy review — review against intent, spec and plan; prepare the release

**Artifacts:** `REVIEW.md` policy (Passes · What Important means here · Cap the nits · Do not report),
findings vs `REVIEW.md` + spec + plan, hook gate drafts. **Gate:** a human code owner approves the
merge; production is always a human one-way door.

## Detect

- `huddle sdlc status --json` → PASS verdict on current fingerprint? plan drift? open one-way doors?
- `huddle gate status --json` → which gates are pending/approved.
- `huddle review-policy status --json` → does `REVIEW.md` exist?

## Interview (≤ 7)

1. What exactly ships? (branch, PR, version)
2. What does "Important" mean for review in this repo? (feeds `REVIEW.md`)
3. What should reviewers not report? (style the formatter handles, etc.)
4. Rollout: flag, canary, or all at once? Rollback plan?
5. Who is the code owner approving the merge? Who approves production?
6. Migrations or irreversible data changes in this release?
7. Which commands deploy? (confirm `deployPatterns` so the production-gate hook catches them)

## Delegate (parallel)

- **Reviewer lens subagents** against spec + plan + `REVIEW.md`: correctness, security,
  migration safety, observability. Canonical JSON findings; cap nits per `REVIEW.md`.
- **Review-policy writer** (if missing): `huddle review-policy new --dry-run` → draft.
- **Hook gate drafts:** `huddle hooks export --host claude` — the production gate blocks deploy
  commands until `huddle gate status --json` shows a gate named `deploy` (or `production`,
  `release`) that is `approved` and not `stale`.

## Room

Senthil (what could leak or break trust?) + Shaama (rollback reality). Then STOP.

## Gate

1. Show findings + verdict + rollout/rollback in one screen.
2. `huddle gate request plan-<slug> --role code --name deploy`. The code owner approves the
   merge and the deploy in their own terminal; any change to the plan afterwards makes the gate
   stale and the production-gate hook blocks again. **You never run `huddle gate approve`,
   never merge, never deploy.**
3. A blocking hook denied or failed? The gate stays closed; only a human may run
   `huddle gate approve <gate-id> --override --reason "<why>"` (logged).
4. Suggest release notes text for the user; don't publish.

## What good looks like

```markdown
## What Important means here
- Anything that can move money twice, or lose a refund state.
- PII in logs.
## Do not report
- Formatting (gofmt runs in CI). Naming nits — max 3 per PR.
```
