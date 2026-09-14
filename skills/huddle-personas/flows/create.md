# Flow: create a persona

## Interview (≤ 7 questions, one at a time, skip what the user already said)

1. What role do they play, and what do they own? (one line)
2. What do they always push back on? (their primary lens — one question they always ask)
3. Their background: one win and one scar that shaped them.
4. How do they talk? (terse / Socratic / blunt / warm; sample phrase)
5. 3–5 principles they hold.
6. When should the room pull them in — and when not? (non-goals, blind spots)
7. Scope: `global` (private, all repos) or `repo` (committed, this repo only)?

Give a RECOMMENDATION with each question when you have one ("repo — this is payments-specific").

## Draft

1. Check the id is free: `huddle persona resolve <id> --json` (exists → offer fork or new id).
2. Fork a built-in instead of starting cold when it's close: start from
   `huddle persona style <builtin>` and adjust.
3. Write the draft (subagent if available) to a temp file in the drafts dir, in this shape:

```markdown
---
name: <id>
displayName: <Name>
title: <Title>
icon: "<emoji>"
role: <what they own>
domains: [a, b]
capabilities: "<comma list>"
identity: "<win and scar>"
primaryLens: "<the question they always ask>"
communicationStyle: "<how they speak>"
principles: "<short beliefs>"
source: huddle
---
## Signature Phrases
## Common Disagreements
## Expertise Areas
## Voices <Name> Has Absorbed
## Non-Goals
## Blind Spots
## When Useful
```

Common Disagreements name real roster members (`huddle persona list --json`).

## Review and write

1. `huddle persona new <id> --scope <scope> --from <draft> --dry-run` → show the full draft
   and target path. For `repo`: "this will be committed".
2. STOP. Ask: "Write it?" Apply edits the user asks for, re-run the dry run.
3. On yes: `huddle persona new <id> --scope <scope> --from <draft>`.
4. `huddle persona lint <id>`. On errors, show them, fix the draft, repeat — never dead-end:
   `--template` gives a scaffold to fill.
5. Offer: "Bring <Name> into the current huddle?" Wait.

## Gate rule

No file is written without the user's explicit yes after seeing the full draft.

## What good looks like

> **primaryLens:** "What happens to the money if this request is retried?"
> **Signature Phrase:** "Show me the idempotency key."
> **Blind Spot:** Over-indexes on consistency; slow to accept eventual designs for non-money paths.
