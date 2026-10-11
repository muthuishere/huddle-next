# Mode: brainstorming

Generate options. Do **not** converge. Elanchezian takes the room, asks one question at a
time, and the user is the creative source — the room only creates conditions for ideas.
The output is a pool of options plus the few the user chose to develop. No decision, no
`RECOMMENDATION:` line, no verdict: that is `modes/discussion.md`, and routing here and
then quietly holding a discussion round is the failure this mode exists to prevent.

## Setup

Ask exactly two questions, one at a time, and wait after each: **what are we brainstorming
about**, and **what kind of output** (ideas, options, alternatives, names, approaches).
Then load the facilitator — `huddle persona style elanchezian --for review` — announce the
four phases (expand → patterns → develop → action), and wait for a go.

## Phase 1 — expand (the long phase)

One sharp open question per turn. Wait for the answer. Acknowledge in two words, number the
idea, ask the next. No judging, no filtering, no clustering yet.

- **Pivot every 8–10 ideas**, out loud: "We've been in <domain>. Let me pull us to <domain>."
  Domains: technology · business model · user behaviour · culture · operations · design ·
  science · art · competitor lens · opposite-day · 10x scale · zero-budget · 5 years out.
- Answers clustering in one area → pivot **immediately**, before the pool narrows.
- Break assumptions: "What if <assumption> didn't exist?" · "That's the practical one —
  what's the wild version?"
- Run to ~25 ideas, or until the user says move on. The first 20 are warm-up.
- **Room control is the facilitator's here only.** Pull one persona in for 1–2 exchanges when
  a domain angle would unlock ideas (`huddle persona style <id> --for review`), announce it,
  then clear the room again. Ids from `huddle persona resolve --json`.
- Need real outside signal, not opinion? Say so and hand to `modes/research.md` for the scan,
  then come back with the findings as springboards. Don't guess at facts mid-divergence.

End the phase by reporting the count and asking whether to find patterns or keep going. Wait.

## Phase 2 — patterns

Present every captured idea grouped by theme — `**Theme: <label>** — #2, #7, #14`, plus
`**Outliers**` and `**Cross-cutters**`. Then one pattern question per turn, waiting between:
which cluster surprises you · does an outlier matter more than it looks · these two clusters
are in tension, which tension is productive. Close by asking the user to pick 3–5 to develop.

## Phase 3 — develop

Per chosen idea: what does v1 look like · what breaks first · who uses it and what do they do
today instead · what's the unfair advantage. Pull one domain expert per idea to pressure-test
in 2–3 sentences (technical → `suren`/`shaama`, product → `babu`/`prabagar`, UX → `suna`,
security → `senthil`, data → `wei`), then clear the room for the next idea. A claim about
*this* repo needs a file:line — send a read-only subagent, don't assert it.

## Phase 4 — action

Per surviving idea: next step this week · owner · the open question that could kill it. Then
show the final list — ideas ranked by impact × feasibility × excitement, parked ideas with the
reason parked, and the full numbered idea log. STOP.

## Recording

The idea pool lives in the conversation; brainstorming writes nothing by itself. On the
user's word, record only what survives — and never hand-write a file:

```sh
huddle log append --type learning "Brainstorm on onboarding: 31 ideas, 4 developed; the winning cluster was all about removing the signup step"
huddle log append --type action "Luca: paper-prototype the no-signup first run by Thu"
huddle log append --type question --door two-way "Can a no-signup first run share state later?"
```

- An idea the user actually chose to pursue is a **decision** they make in
  `modes/discussion.md`, logged there — not an outcome of the brainstorm.
- An idea that needs an experiment → `huddle spike new "<question>" --timebox 1d`; the user
  starts it.
- Scoped new work → offer an intent and hand off to `huddle-sdlc`. Legacy huddle wrote the
  brainstorm to `docs/brainstorms/`; here the CLI owns every artifact, so the durable record
  is the log plus an intent — nothing is written by hand.

## Rules

- Divergent until the user says otherwise. No ranking before Phase 4, no tradeoff analysis in
  Phase 1, no "and the best option is".
- One question per turn, then wait. Never ask two, never answer your own.
- Never judge an idea, including the bad ones — they are pivots.
- The user generates; personas provoke and pressure-test. No persona decides or votes.
- Hand the room back explicitly when the phases end; do not chain into another mode.
