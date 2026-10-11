# Mode: planning

Turn a direction the user has already chosen into order of work, risks and proof.
Planning does not pick the direction — if that is still open you are in
`modes/discussion.md`. The output is a sequence the room has argued over and the user owns.

## Boundary with `huddle plan` (read this first)

Two different things share the word "plan":

- **This mode deliberates.** The room sequences the work, names what breaks first and what
  would prove it. Nothing is written unless the user asks. Use it for a change small enough
  to hold in one head, a spike, a refactor, a "how do we roll this out".
- **`intent/<slug>/plan.md` is an artifact** with a gate and a lint contract (`Files that
  change` · `Order of work` · `Risks` · `Proof`). It belongs to an intent and a spec, and it
  is written by the `huddle-sdlc` skill's build stage — not here.

So: if `huddle intent list --json` shows an intent this work belongs to, or the user says
"write the plan", stop deliberating and **hand off to `huddle-sdlc`** (`huddle sdlc next`
says exactly where they are). If there is no intent and the user wants one, that is also
`huddle-sdlc`. Never run `huddle plan new` from this mode to "capture" a discussion.

## Steps

1. **Restate the chosen direction in one line** and name what is already fixed: the decision,
   its door, any constraint. If the direction is actually still contested, say so and route
   back to discussion rather than planning a thing nobody has decided.
2. **Load the constraints, don't re-derive them.** `huddle log list --type decision,lock --limit 20`
   for what the room already settled, and any accepted ADR the work touches. A plan that
   contradicts a `lock` or an accepted ADR is a finding to raise, not a step to write.
3. **Pick the room** (2, 3 only if the third changes the sequence): `suren` for the shape and
   the order, `shaama` for what the code actually makes cheap or expensive, `nina` for what
   would prove it. Add `senthil` when a trust boundary moves, `luca` for client-side work,
   `prabagar` when the acceptance criteria are fuzzy. Ids from `huddle persona resolve --json`;
   voices from `huddle persona style <id> --for review`. Brief them together on the work with
   `huddle persona brief suren,shaama,nina --task "<the direction>"` when the framing matters
   more than the individual takes.
4. **Ground the sequence in the tree.** Send a read-only subagent to find the files that would
   change and what already sits in the way; it returns ≤ 400 tokens with file:line. A step
   with no file behind it is a wish, not a step.
5. **Produce four things, nothing more:**
   - **Order of work** — numbered steps, each one shippable or revertible on its own, each
     naming the file or package it lands in.
   - **Risks** — what breaks first, and for each the cheapest early signal that it is breaking.
   - **Proof** — per step, the test, eval, metric or band that says it worked. "We'll see" is
     not proof.
   - **The first cut** — what is explicitly not in this pass.
6. **Close** with the sequencing question and the door. One-way (a migration, a public
   interface, a data shape) → options plus a `RECOMMENDATION:` line. Then STOP.

## Before anyone builds

When the user is about to hand the sequence into implementation — "let's build this", "ship
it" — offer the pre-build pressure-test **once**: Suren, Shaama and Nina each surface one
concrete gap in their domain and one "what would break first" hypothesis
(`modes/verification.md`, against the plan rather than a diff). Route only if the user accepts.
Never block implementation on it.

## Recording

Only on the user's word:

```sh
huddle log append --type decision --door two-way --by "<user>" "Ship the queue change in 3 steps: schema, writer behind a flag, reader cutover — revert point after each"
huddle log append --type assumption "Step 2 assumes no consumer reads the old column after the flag flips"
huddle log append --type action "Nina: contention test for step 2 before the flag flips"
```

An unanswerable question blocking the sequence → `huddle spike new "<question>" --timebox 1d`;
the user starts it. A sequencing choice with lasting consequences → offer an ADR via
`huddle-sdlc`. Deliberation itself writes nothing.

## Rules

- Order of work, risks, proof. No estimates in days unless the user asks, no Gantt, no phases
  named after quarters.
- Every step has a revert, or it is called out as the one that does not.
- The user owns the sequence; personas argue about it and then stop.
- Never approve a gate, never write the plan artifact from here.

## What good looks like

> **🏗️ Suren** _(Architect)_ — Three steps, in this order: add the column
> (`internal/jobs/schema.sql`), write both behind `QUEUE_V2` (`runner.go:88`), flip readers.
> Each is revertible alone; doing 1 and 3 together is not.
>
> **🧪 Nina** _(Tester)_ — Step 2 needs a contention test before the flag flips, otherwise the
> proof for step 3 is "prod looked fine".
>
> Two-way door. Sequence it this way, or collapse 1 and 2 to save a deploy? Your call, Muthu?
