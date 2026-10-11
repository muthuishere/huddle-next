---
id: adr-22
kind: adr
status: draft
author: Muthukumaran Navaneethakrishnan
links: {}
---
# ADR 22: Ask-back endpoint — a huddle asks a question and resumes

## Status

Draft (2026-10-11) — design only, nothing in this ADR is built. It extends
ADR 20 §3 and ADR 21; it does not amend ADR 20's gate rule, and argues below
why it does not need to.

## Decision

A huddle may **ask a question over a configured endpoint, stop, and resume when
the answer arrives**. Questions and answers travel over HTTP; **gate approval
does not**.

1. **`ask` is an artifact of the event log, not a new store.** A huddle posts a
   question with `huddle ask new --question "…" [--options a,b] [--session
   <autonomous id>] [--timeout 30m]`, which appends one `ask.asked` event and
   returns an `ask` id. `huddle ask status <id>` reports `pending|answered|
   expired`; `huddle ask wait <id>` blocks a caller that wants to be woken.
2. **The endpoint is outbound, named in config, and dumb.** `config set
   ask.endpoint <url>` (plus `ask.channel`) names a webhook huddle POSTs the
   question to: `{ask, repo, branch, question, options, session, expires}`.
   Huddle opens no port for this and listens on nothing. Reaching the endpoint
   is the only network call any huddle command makes, it happens on `ask new`
   only, and it is never on the `huddle start` path (ADR 2, ADR 13).
3. **The answer comes back identified.** The answering party calls back with
   `{ask, answer, by, identity}` where `identity` is a signed assertion from
   the channel (the owner's messenger hub: a specific account on a specific
   channel, not "someone on localhost"). Huddle records `ask.answered` with
   `by` set to that identity and the assertion's fingerprint in `data`.
   Delivery is either a short-lived loopback receiver started by `huddle ask
   wait`, or a poll of the channel — both are receiver shapes, not a second
   server, and both record the same event.
4. **An answer is an input, never an approval.** An `ask.answered` event can
   unblock a waiting huddle, fill a question in an artifact, or feed an
   autonomous round. It cannot change a gate's state, set an artifact's
   `status`, or make an escalated verdict actionable. The vocabulary is
   enforced: `ask` events carry no `door` and no gate id.
5. **ADR 20 §3 stands, unweakened.** `internal/serve/mutate.go` keeps refusing
   `gate` and `verdict` types in `postLog`; `huddle gate approve|reject` keeps
   refusing agent sessions. Approving a gate through the ask-back channel is
   out of scope of this ADR and needs its own.

## Context

- The owner's ask: *"they will be giving an endpoint, they can ask questions and
  then come back."* A fleet huddle that hits a genuine unknown should be able to
  ask the human and continue, instead of guessing or dying.
- ADR 21 created the need: an escalated verdict is currently a file and a log
  line with no transport. Something has to carry it to a human.
- The collision: **ADR 20 §3 says gates are not approved in the browser**,
  because "a localhost HTTP endpoint is reachable by any agent on the machine,
  so it cannot prove a human (ADR 19 row 7)". `postLog` enforces it
  structurally by allowing only `decision`, `learning` and `question`.
- Read literally as "no approvals over HTTP", that rule would also forbid this
  feature. Read as what it argues — *the channel could not prove a human* — it
  does not, and the distinction is the whole decision here.

## Details

**Why this is not a hole in ADR 20.**

1. **No separation-of-duties claim is being made.** A gate is an assertion that
   a specific accountable human, who is not the artifact's writer, approved a
   specific artifact hash. An answer to a question asserts only "this is the
   information you asked for". ADR 20's reasoning was about the first kind of
   claim; it has nothing to defend in the second.
2. **ADR 20 ruled on the channel it had.** Its subject was a browser page on
   `127.0.0.1` with a per-process token, where every agent on the machine is
   indistinguishable from the user. That is an unauthenticated channel, and the
   conclusion ("it cannot prove a human") is correct *about that channel*.
3. **An authenticated channel to a named person is a different fact.** The
   owner's messenger hub delivers to a specific account the owner controls and
   returns an identity the event log can record and a reviewer can audit. That
   is an identity path ADR 20 never considered, not a weakening of it.
4. **So the line moves from transport to proof.** The rule that should survive
   is *"a gate needs a provable human"*, not *"HTTP is forbidden"*. Under that
   rule this endpoint is plainly fine, and — stated for the record — a future
   gate approval over an authenticated channel is **arguable but not approved
   here**: it needs a replay-resistant assertion bound to the artifact hash,
   and a trust story for the hub itself. Until that ADR exists, the conservative
   refusal is the right default, because the cost of being wrong is asymmetric:
   a question answered by the wrong party wastes a round, a gate approved by the
   wrong party ships unreviewed code.
5. **The dashboard stays as it is.** No new mutation type, no gate route, and
   the `postLog` allow-list is untouched. The ask-back path is a CLI feature
   with an outbound webhook, not a second write surface into the repo.

**Failure modes, by design.** No endpoint configured → `ask new` fails loudly
and the caller decides what to do; it never silently proceeds. Endpoint
unreachable → the question is still logged, `status: pending`; huddle retries
nothing. Timeout → `ask.expired`; a waiting autonomous huddle treats an expired
ask as an escalation, not as consent. Answer text is untrusted input: it is
redacted on arrival, is data and never instructions (AGENTS.md), and is capped
in length. Two answers to one ask → the first wins, the second is recorded and
ignored.

## Consequences

- Adds `internal/ask` (events, state machine, identity record), an `ask`
  command group, two config keys, and the first deliberate outbound network
  call in the CLI — which needs its own line in the security story and must
  stay off the hot path.
- Gives ADR 21's escalation a destination and gives interactive huddles a way
  to ask a blocked question without ending the session.
- The claim "the channel proves a human" is only as strong as the hub. If the
  hub is compromised, the blast radius is a wrong answer to a question — which
  is exactly why gates are kept out of it.
- A waiting huddle holds state across processes; `ask wait` must be
  interruptible, and a pending ask must be visible in `huddle sdlc status`.
- Outside v1 scope (ADR 19). Nothing is built until this ADR is accepted.

## binds

- Any answer path must record an identity in the event log. An anonymous answer
  is not an answer.
- `ask` events may never carry a gate id or a `door`, and may never set an
  artifact's status.
- The `postLog` refusal of `gate` and `verdict` types stays. Removing it needs a
  new ADR that supersedes ADR 20 §3 explicitly.
- Huddle opens no listening port for ask-back beyond a short-lived, loopback,
  single-ask receiver started by an explicit `ask wait`.

## prevents

- No gate approval over HTTP, authenticated or not, under this ADR.
- No inbound control channel: the endpoint cannot make huddle run commands,
  write artifacts, or change config — only answer a question it asked.
- No polling loop or daemon on the agent hot path; no network call in
  `huddle start`.

## Open questions

- **What identity format does the hub return?** A signed JWT, a channel-scoped
  user id, or a shared-secret HMAC over the ask id — unresolved, and it decides
  how strong the "provable human" claim can later become.
- **Who answers when the owner is asleep?** An ask with no answer and a short
  timeout degrades to an escalation; whether a delegate may answer on the
  owner's behalf is an owners-config question.
- **Should an answered ask be an artifact?** Decisions are; answers are
  currently only events.
- **One ask or many?** A huddle that needs three answers to proceed either asks
  three times or needs a batched ask; the batched shape is not designed.
- **Does an autonomous session resume in the same process?** Resuming means
  re-entering the room with new information mid-round, which the round model
  (ADR 21 §2) does not describe.
