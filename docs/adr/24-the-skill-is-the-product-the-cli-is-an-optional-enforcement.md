---
id: adr-24
kind: adr
status: proposed
author: Muthukumaran Navaneethakrishnan
links: {}
---
# ADR 24: The skill is the product; the CLI is an optional enforcement kernel

## Status

Proposed (2026-10-11). The front matter `status:` field is the status of record.
Amends ADR 16 (which made the CLI the orchestrator) and the delivery assumptions in
ADR 19. Applies ADR 23's detect-never-depend pattern one level up: to huddle itself.

## Decision

The skill family is the product. It must complete the whole loop -- discussion,
brainstorming, planning, research, spec-review, verification, wrap-up, intents, specs,
plans, ADRs, spikes, custom repo personas -- with **no binary installed at all**. The
CLI becomes optional: when it is present the loop gains an append-only audit log,
gates with real separation of duties, a single redaction choke point and structural
lint; when it is absent the same loop runs on the agent's own file tools and those
guarantees are **advisory and must say so**. A default installation installs the skill.

## Context

The owner's call, 2026-10-11: "huddle should be run in claude, no one needs a cli for
huddle." Three facts make that right, and one makes it incomplete.

Right: (1) The conversational two-thirds of huddle is prompt and markdown. The legacy
Python skill did discussion, brainstorming and personas with no binary and it worked.
(2) The binary is the entire reason for the unfinished release checklist -- a public
repo, seven npm packages needing trusted publishing bootstrapped one manual publish at
a time, a homebrew tap token, a winget fork, a `v0.1.0` tag. None of that work serves a
user who wants a room of experts. (3) Requiring an install to have a conversation is a
product tax, and the testdrive already showed install itself is where huddle's worst
bug lived (skill-name shadowing, ADR 8 / commit 1de9a03).

Incomplete: **the agent cannot be the thing that gates the agent.** A skill is
instructions, and the party being restrained is the party reading them. Two independent
adversarial drives on 2026-10-11 each found, without being told to look, that
`sed -i 's/^status: proposed/status: approved/'` makes `adr lint` report `approved` with
no gate, no owner and no `gate.approve` event -- confirmed in `internal/artifacts/store.go:42`,
where status is checked only for membership in the vocabulary and nothing in
`internal/artifacts/` consults gates at all. That is a bug today. In a world with no
separate process it is not a bug but the permanent condition: every check becomes a
suggestion. The only reason one drive could report "gate approve refused me even with
CLAUDECODE and HUDDLE_AGENT stripped from the environment" is that a non-agent process
noticed `CLAUDE_CODE_SESSION_ID`. Same for redaction: a planted `sk-ant-api03-...`
never reached disk because something other than the agent wrote the file.

So the split is not CLI-or-skill. It is: the room needs no binary, and enforcement
cannot exist without one.

## Details

The skill family currently depends on the CLI in 21 distinct verbs, the heaviest being
`huddle log append` (23 call sites), `persona style` (11), `gate approve` (10),
`persona resolve` (8) and `gate request` (8). Each needs a documented no-binary path.

Assets must move from the binary to the skill: `assets/personas/` (22 personas) and
`assets/templates/` (14 templates) are `go:embed`-ed today, so with no binary there is
nothing to resolve a persona or stamp a template from. They ship as files in the skill.

Detection mirrors `internal/specframework`: look for the binary, say what was found in
one line, never require it, never fail because it is absent. Absence is the normal case
and must never read as a defect -- the same wording discipline ADR 23 fixed.

Two changes already made under this decision:
- A bare `huddle install` installs the skill family. It previously refused and demanded
  `--skills`, which contradicted "the skill is the product". `--skills` stays accepted
  because every published postinstall hook and the brew cask pass it.
- The skill's own failure line ("huddle binary missing or out of date -- run: npm i -g
  ...") must stop presenting the binary as required. It now describes an upgrade.

Honesty requirement: in skill-only mode the product must not claim a human gate at
every stage. It claims a human gate it *records*, and names the difference. Selling
enforcement that is a convention is the one outcome this ADR exists to prevent.

## Consequences

Easier: adoption drops to zero install. `v0.1.0` stops being blocked on seven npm
packages, a tap token and a winget fork, because those now ship an optional upgrade
rather than the product. The skill becomes testable without building Go.

Harder: two paths must stay honest about the same loop, and the failure mode is silent
divergence -- a mode that works with the binary and quietly degrades without it. This
repo has been bitten by exactly that twice (three dropped modes in the Python port; a
parity eval that passed because mode names appeared under `docs/adr/`). So this decision
owes an eval that runs the loop with the binary absent from PATH, not merely a test that
the skill mentions a fallback.

Owed: the no-binary path for all 21 verbs; personas and templates as skill files; the
degraded-mode wording; and the enforcement claims corrected everywhere they overstate.

## binds

- Every skill mode must state what it does when the binary is absent, and that path must
  be exercised by an eval with `huddle` off PATH.
- Personas and templates are skill files; the binary may embed a copy but is never the
  only source.
- A default install installs the skill family.
- Any claim of enforcement (gates, audit, redaction) must name whether it is enforced or
  advisory in the configuration being described.

## prevents

- Requiring the binary for any conversational mode, or for authoring any artifact.
- Claiming enforced gates, a tamper-evident log or guaranteed redaction in skill-only
  mode.
- Adding a CLI verb as the only way to do something a human in a chat could do.
- Blocking a release on binary distribution work.

## Open questions

- Does `huddle serve` survive at all without a binary, or does the dashboard become a
  CLI-only feature? (Leaning: CLI-only, and say so.)
- In skill-only mode, where does the event log live -- in-repo and reviewable, or outside
  the repo like today's `~/.config/.huddle/repos/<name>/events.jsonl`? In-repo makes it
  diffable and gives governance teeth through code review, which is the only enforcement
  available without a separate process.
- Does the repo-basename state key (`internal/repo/repo.go:34`, `internal/paths/paths.go:47`)
  get fixed or deleted by this decision? If the log moves in-repo, the collision bug both
  drives found disappears rather than needing a fix.
