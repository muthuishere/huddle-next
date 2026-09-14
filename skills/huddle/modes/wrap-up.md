# Mode: wrap-up

Close or pause the huddle with a record the team can read without the chat.

Triggers: "wrap up", "that's enough", "good for now", "save and pause", "end meeting",
"summarise", "give me the notes", "what are the action items".

## Steps

1. **Collect what was said but not yet logged.** Scan this session for decisions the user
   made, open questions, and actions. Check against `huddle log list --limit 50 --json`
   so nothing is logged twice.
2. **Show the draft record** before writing anything:

   ```
   Decisions (user's)      · one line each, [one-way]/[two-way]
   Open questions          · with door type
   Actions                 · owner: action (due if stated)
   Learnings               · only durable, reusable lessons
   ```
3. **STOP and ask:** "Log these?" Edit as the user says.
4. **On confirmation**, append each item:
   `huddle log append --type decision|question|action|learning --door ... --by "<user>" "<text>"`
5. **Suggest next artifacts, don't create them:**
   - a decision with lasting consequences → "want an ADR?" (`huddle adr new` via `huddle-sdlc`)
   - scoped new work → "want an intent?" (`huddle intent new`)
   - a lesson for a persona → "want to remember this for <persona>?" (`huddle-personas`)
6. **Pause vs close.** Pause: say what's open and that `huddle start` will resume it.
   Close: show `huddle timeline --limit 30` as the final view.

## Summaries on demand ("where are we?")

Text only, from `huddle log list --json` + this conversation. Max 12 lines:
covered · decided · open · next. No diagrams. Then wait.

## Rules

- Nothing written without the user's confirmation.
- Decisions are attributed to the user, never to a persona.
- Don't invent actions or owners the user didn't state.
- Never commit, push, or approve gates as part of wrap-up; suggest the commit message only.

## What good looks like

> **Decisions** — [two-way] Advisory locks for the job queue; revisit at 2k jobs/min.
> **Open** — [one-way] Provider-side idempotency keys for refunds?
> **Actions** — Shaama: lock contention spike by Fri.
> Log these 3 items? Then I'd suggest an ADR for the queue decision.
