---
id: adr-21
kind: adr
status: proposed
author: Muthukumaran Navaneethakrishnan
links: {}
---
# ADR 21: Autonomous Decider Mode — headless owner-resolved huddles

## Status

Proposed (2026-10-11). The front matter `status:` field is the status of record.
This ADR amends ADR 19's "Deferred" line (`Room modes beyond session`) for one
mode only; nothing else moves out of deferred with it.

## Decision

Huddle gains a second way to run a room: **Autonomous Decider Mode**, entered
explicitly, which reaches an actionable verdict with no human stop.

1. **A room of any size, with exactly one owner.** `huddle autonomous new
   --question "…" --personas a,b,c [--owner arasan] [--rounds 3]` opens a
   session. The owner is the deciding vote and must be a member of the room;
   when the caller names none it defaults to `arasan`, a new built-in
   owner-aligned decider persona. A room needs at least two personas.
2. **R rounds of 5-whys, enforced to deepen.** `huddle autonomous turn
   <session> --round N --persona <id> --why "a; b; c"` records one turn.
   Rounds are 3–5. A turn needs at least three deepening steps, a persona
   speaks once per round, rounds are taken in order, and a round may not be
   shallower than that persona's earlier rounds.
3. **A vote resolved through the owner's lens.** `huddle autonomous vote` then
   `tally`. Each option's weight is the sum of its voters' confidences. The
   resolved option is the owner's position unless one single option strictly
   dominates it — in which case the room overrides the owner and the override
   is recorded. A tie at the top goes to the owner. Every persona against the
   resolved option is recorded as a dissent.
4. **A verdict that either acts or stages.** `huddle autonomous verdict`
   writes `verdict.json` + `verdict.md` once, appends one
   `autonomous.decided` / `autonomous.escalated` event, and sets
   `actionable`. An **owner-level fork** — `launch`, `spend`, `irreversible`,
   `legal`, `security`, `scope` — makes the verdict `escalated`: the fleet must
   stage it for the human owner rather than act.
5. **The skill is declarative; the binary is deterministic.** The agent writes
   the reasoning (`skills/huddle/modes/autonomous.md`); `internal/autonomous`
   owns the session shape, the depth rules, the arithmetic, the fork policy and
   the verdict writer. Nothing on this path calls a model or the network.

## Context

- Ported from `muthuishere-agent-skills/huddle` PR #9 ("Autonomous Decider
  Mode"), whose substrate was `scripts/autonomous_huddle.py`. The design spec
  is that PR's `docs/huddle-autonomous-decider.md`.
- The interactive huddle is human-decides by construction: personas surface
  perspectives and the skill stops. That is right for a person at a keyboard
  and a dead end for a fleet that must convene a real room inside a spec-first
  loop and come out with something it can act on.
- ADR 19 defers "room modes beyond `session`". This ADR is the explicit scope
  amendment that clause requires, for this one mode, because the capability it
  buys — a decision an agent can act on, with a recorded trail and a hard
  escalation line — is the thing the fleet cannot get any other way.
- The guardrail it must not break: agents never approve gates (AGENTS.md, ADR
  19 row 7). Autonomous mode decides *below* the fork line and approves
  nothing. An escalated verdict is staged for a human, never self-approved, and
  `huddle gate approve` still refuses an agent session.

## Details

Where the Go port deliberately differs from the Python original:

- **Sequential rounds.** The Python monotonicity check only compared a turn
  against rounds *before* it, so a deep round 3 followed by a shallow round 2
  passed. The port requires a persona's next round, closing that hole.
- **Votes come after deliberation.** The port refuses a vote until every
  persona has spoken in every round, and refuses a verdict until every persona
  has voted. The original would happily resolve a room that never deliberated.
- **Fork detection, not just fork declaration.** `ForkCheck` escalates on the
  flags the agent declared *and* on owner-level language in the decision's own
  wording ("go live", "spend", "one-way door", "rotate the credential", …).
  Detection only ever adds escalation, and each reason says it was detected. A
  forgotten `--flags` can no longer ship a launch.
- **The audit trail is the repo's, not a side file.** Every verdict appends an
  event (`door: one-way` when escalated), per ADR 14/19 row 4. The session
  manifest under `~/.config/.huddle/repos/<repo>/autonomous/<id>/` is state,
  and the committed thing is whatever the fleet then writes from the verdict.
- **Writers never overwrite.** A second `new` with the same id and a second
  `verdict` for one session are both `ErrExists`; `--dry-run` prints the
  verdict without writing it.
- **Redaction at the boundary.** Questions, stances, whys, positions, reasons
  and decisions go through `internal/redact` before they are stored.
- **Time is injected.** The Python script never read the clock so runs were
  reproducible; the port keeps that by taking `Now` as a parameter, with the
  command layer supplying `time.Now()`.

The weights are deliberately crude: confidence sums, rounded to six decimals,
with a 1e-9 tie epsilon. How persuasive a persona was belongs in the trail the
agent writes, not in the arithmetic.

## Consequences

- Adds `internal/autonomous`, `internal/app/cmd_autonomous.go`, the `arasan`
  built-in persona (22 built-ins now) and `skills/huddle/modes/autonomous.md`.
- `huddle`'s SKILL.md now carries one sentence that says the "never decide for
  them" rule has exactly one exception, and names the file. That line is the
  only edit to the interactive contract.
- A new escalation path exists with no transport: an escalated verdict is a
  file plus a log line, and something still has to put it in front of the
  human. ADR 22 proposes the channel.
- The fork word list is a heuristic in code. It will produce false escalations,
  which is the intended direction of error, and it will need maintenance.
- Nothing about interactive mode changes; the four existing modes and the
  `huddle start` budgets are untouched.

## binds

- Autonomous mode is entered explicitly or not at all. No router may fall back
  to it, and no interactive route may borrow its suspension of "user drives".
- The decision math lives in `internal/autonomous` only. A skill, a prompt or a
  dashboard that re-derives a tally or a fork verdict is a bug.
- Owner-level forks escalate. Any future flag that lets a verdict act on a fork
  needs its own accepted ADR.
- An agent never approves a gate, in any mode.

## prevents

- No LLM call, no network call, and no clock read inside the decision path.
- No autonomous verdict may approve a gate, write to another tool's managed
  block, or overwrite an existing verdict.
- No second scoring model (per-persona weights, reputation, seniority
  multipliers) without a new ADR — the owner's lens is the tie-breaker.

## Open questions

- **Where does an escalated verdict go?** Today: a file and a log event. ADR 22
  proposes an authenticated ask-back channel; until that is accepted, the
  caller is responsible for surfacing it.
- **Should the fleet's verdict become an artifact?** A `decided` verdict is not
  yet a committed artifact kind (no `verdict new`), so the chain from verdict to
  intent/spec is hand-made. Candidate for ADR 18's catalog.
- **`huddle serve` has no autonomous view.** Sessions, trails and escalations
  are not in the dashboard's read model yet.
- **Should an escalated verdict open a gate automatically?** It would give the
  human a familiar approve/reject surface, but it also lets an agent create gate
  requests at will. Left out deliberately.
