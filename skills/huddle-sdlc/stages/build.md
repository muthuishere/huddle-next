# Stage 3: Build — plan before code, then the harness builds

**Artifacts:** `intent/<slug>/plan.md` (Files that change · Order of work · Risks · Proof) and a
task brief per step; diffs on a branch. **Gate:** the user approves the plan before any code;
diff-vs-plan drift is checked as work lands.

## Detect

- `huddle sdlc status --json` → spec approved? stale verdicts? (warn, don't block)
- `huddle plan status plan-<slug> --json` → existing plan and drift.

## Interview (≤ 7)

1. Which files change, and why each? (becomes "Files that change")
2. What order — which steps are independent and can run in parallel?
3. What's the proof target per step? (the command that shows it's done)
4. Riskiest step, and how would we notice it went wrong?
5. Is any step a one-way door (migration, public API, data deletion)?
6. Is this a fix? → the failing test is committed first; tests are protected during the fix.
7. Branch / worktree strategy for parallel steps?

## Delegate

- **Plan writer subagent:** `huddle plan new <slug> --dry-run` → draft in drafts dir;
  returns path + required-heading check (same rules as `huddle plan lint`) + 5-line summary.
- After approval, **per step**: build a task brief (plan step + spec requirements + guardrails
  + style cards, e.g. `huddle persona brief shaama --task "<step>"`) and hand it to the
  harness's own workers (subagents, parallel worktree sessions). Independent steps in
  parallel; dependent steps wait. Workers iterate to their proof target.
- Workers never merge, push, or deploy. Results: `huddle log append --type action ...`.
- Fix tasks: set `HUDDLE_FIX_MODE=1` for the worker so the exported hook blocks test edits.

## Room

Shaama (is the order buildable?) + Nina (is each proof real?). Suren only if the plan touches
architecture. Then STOP.

## Gate

1. Show the plan in full. No code before approval.
2. On the user's yes: `huddle plan new <slug> --from <draft>`, then
   `huddle gate request plan-<slug>` → owner approves in their terminal. You never approve.
3. Suggest the commit.
4. When implementation departs from the plan, update `plan.md` in the same commit and tell
   the user (the plan-drift hook reminds you).

## What good looks like

```markdown
## Files that change
- `internal/pay/refund.go` — add timeout → failed transition
- `internal/pay/refund_test.go` — failing test first
## Order of work
1. Failing test `TestRefundTimeoutMarksFailed` (commit alone)
2. Timeout transition (parallel with 3)
3. Support dashboard column
## Proof
`go test ./internal/pay/ -run Refund` green; eval `refund-timeout.json` passes.
```
