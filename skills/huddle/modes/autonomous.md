# Mode: autonomous (headless, owner-decided)

The only mode that reaches a decision with no human in the loop. It is entered
explicitly — "decide this without me", "autonomous huddle", "convene the
deciders", or a programmatic fleet call. Every other mode keeps the "user
drives, stop after every round" rule. **Inside this mode only** that rule is
suspended: you run all the rounds, resolve the vote, and write the verdict in
one go, without stopping to ask.

If a human is at the keyboard wanting to think out loud, you are in the wrong
mode — go back to `modes/discussion.md`. Never switch someone into this mode
silently.

You supply the persona reasoning. The binary owns the session, the 5-whys
rules, the decision math and the verdict. Do not do the arithmetic yourself and
do not write the files by hand.

## 1. Convene the room

Pick the personas the question deserves (there is no cap) with
`huddle persona resolve --json`, and name **one owner** — the decider. Default
to `arasan`, the owner-aligned decider, when the caller names none. The owner
must be in the room; it speaks last every round.

```
huddle autonomous new --question "<the decision>" --personas arasan,shaama,nina --rounds 3 --json
```

Rounds are 3–5 (default 3). Keep the `session` id and the `speakingOrder` from
the output. A room needs at least two personas: an owner and one other voice.

## 2. Deliberate — R rounds of 5-whys

For each round, in speaking order, have every persona speak in character
(`huddle persona style <id>` once per persona per session). A turn is not a
one-shot take: it is a chain that **deepens** — why → why → root cause or root
risk — and a later round must dig at least as deep as that persona's earlier
rounds. The CLI rejects a shallow round, a skipped round and a second turn in
the same round, so make each round actually go further than the last.

```
huddle autonomous turn <session> --round 1 --persona shaama --stance "cache it behind a flag" --why "p50 is 40ms; the index read dominates; we are paying it on every start"
```

Semicolons separate the steps; three steps is the floor. Review what the room
has built with `huddle autonomous trail <session>`.

## 3. Vote

After the last round every persona — the owner included — casts one vote: the
option it backs, a confidence 0..1, and one line of reason. Use the same option
label for the same option across personas, or the tally splits the room.

```
huddle autonomous vote <session> --persona shaama --position "cache behind a flag" --confidence 0.8 --reason "reversible and measurable"
```

## 4. Resolve through the owner's lens

```
huddle autonomous tally <session>
```

Each option's weight is the sum of its voters' confidences. The resolved option
is the owner's position unless one single option strictly dominates it — then
the room overrides the owner and the override is recorded. A tie at the top
goes to the owner. Personas against the resolved option are recorded as
dissents, never erased.

## 5. Verdict — act or stage

Name the decision in one actionable line and declare any owner-level fork it
carries: `launch`, `spend`, `irreversible`, `legal`, `security`, `scope`. Check
a draft wording first if you are unsure:

```
huddle autonomous fork-check --decision "<text>" --flags launch,spend
huddle autonomous verdict <session> --decision "<the actionable decision>" --flags spend --summary "<one line>"
```

The verdict is written once (`verdict.json` + `verdict.md`) and logged. Then:

- `STATUS: decided` / `ACTIONABLE: true` → the verdict is the spec. Act on it
  headlessly, no human stop. Cite the session id in what you do next.
- `STATUS: escalated` / `ACTIONABLE: false` → an owner-level fork. **Do not
  act.** Hand the human owner the decision, the tally, the dissents and the
  escalation reasons, and stop there.

The CLI also escalates on fork language it finds in the decision itself, so a
forgotten `--flags` cannot ship a launch. When you are unsure whether something
is a fork, flag it: a false escalation costs one message, a wrong headless
launch costs the product.

## Rules specific to this mode

- No stopping between rounds; this mode runs to a verdict.
- Stay in character. The whys must deepen, not restate the stance in new words.
- The owner decides *between* perspectives; it does not generate them, and it
  does not out-argue the domain experts.
- You never approve a gate, here or anywhere: `huddle gate approve` is the
  human's. An escalated verdict is staged, not self-approved.
- Everything below the fork line is yours to close. Everything on or above it
  is the human owner's.
