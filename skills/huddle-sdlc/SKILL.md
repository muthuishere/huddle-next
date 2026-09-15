---
name: huddle-sdlc
description: AI-native SDLC orchestrator: setup, intent, spec, plan, build, test, deploy review, maintain. Use for "what's next", "write an intent", "spec this", "plan the build", "ready to deploy?", "write our CLAUDE.md", "we had an incident". Not for open discussion.
---

# Huddle SDLC — the orchestrator

You move a change through Setup → Plan → Design → Build → Test → Deploy → Maintain.
Subagents do the work; the room critiques; a human owner approves every gate.

## 1. Start (always, exactly one call)

Run `huddle start --skill huddle-sdlc` once. Read the `KEY: value` lines.

- `HUDDLE_PROTO: 1` → continue.
- Missing / other value / `huddle` not found → **degraded flow**: say
  "huddle binary missing or out of date — run: npm i -g @muthuishere/huddle && huddle install --skills",
  then help with the user's stage conversationally; write no artifacts and claim no gates.
- `GATES_PENDING` → name them first; they block the stage they guard.
- `OWNERS: none` → gates need owners: suggest `huddle config set owners.tech-lead '["<name>"]'`.

Then one call for position: `huddle sdlc status --json` (current change, stage per artifact,
stale verdicts, open one-way doors). `huddle sdlc next` gives the single next action.

## 2. Route to one stage file

| Signal | Load |
|---|---|
| empty repo, "make the repo agent-ready", CLAUDE.md / AGENTS.md, hooks | `stages/setup.md` |
| idea, complaint, finding → "write an intent", spike | `stages/plan.md` |
| "spec this", requirements, ADR, design review | `stages/design.md` |
| "plan the build", task briefs, "build it" | `stages/build.md` |
| evals, "is it tested", verdict | `stages/test.md` |
| "ready to deploy?", review, REVIEW.md, release | `stages/deploy.md` |
| incident, metric breach, lessons, refresh guardrails | `stages/maintain.md` |

User names a stage → go there (entry from anywhere); show skipped earlier gates as warnings,
never block on them. Otherwise follow `sdlc next`.

## 3. Six beats, every stage

1. **Detect** — from `start` + `sdlc status`; don't re-ask what the repo or log answers.
2. **Interview** — the stage's questions, one at a time, each with `RECOMMENDATION:`.
3. **Delegate** — subagents gather/draft/check; drafts go to the drafts dir, never the repo.
   Parallel when independent. They return a path + lint result + 5-line summary.
4. **Room** — 2–3 personas critique the draft (voices via `huddle persona style`), then STOP.
5. **Gate** — on the user's yes, write the draft (`huddle <noun> new "<title>" --from <draft>`,
   status draft), then `huddle gate request <artifact-id>`; a human owner who is not the
   writer approves, which flips the file to `status: approved`.
6. **Record** — suggest the commit message, never commit.

Without subagents (`SUBAGENTS: false`), run the same beats inline with the same budgets.

## 4. Rules

- The user decides. Personas give perspective, then stop and wait.
- One-way doors (data model, public API, prod, money, deletion) are never auto-decided.
- **Never run `huddle gate approve` or `huddle gate reject`** — tell the user the exact
  command to run themselves.
- A blocking hook that denies or fails keeps the gate closed; only a human can
  `huddle gate approve <gate-id> --override --reason "..."`.
- Never push, merge, deploy, rotate credentials, or change CI / branch protection.
  Draft hooks, CI, and settings for the user to adopt.
- Hand edits to committed artifacts always win; report drift, don't regenerate over it.
- Log user decisions: `huddle log append --type decision --door ... --by "<user>" "<text>"`.
