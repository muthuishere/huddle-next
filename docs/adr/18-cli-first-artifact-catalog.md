---
id: adr-18
kind: adr
status: proposed
author: muthuishere
links: {}
---
# ADR 18: CLI-First and Human-Visible — Every Artifact Is a CLI Noun (Intent, Spec, Plan, ADR, Spike, Guardrails, Hooks, …)

- Date: 2026-09-14
- v1: built; deferred: `huddle watch`, groups (ADR 19)
- Builds on: ADR 12 (writers), 16 (orchestrator), 17 (hooks)

## Status

**proposed** — the decision stands; it is only partly implemented. The `v1:` line above and
**Open questions** name which clauses landed and which did not. The front matter `status:` field
is the status of record.

## Decision

### A. CLI-first, for humans as much as agents

1. **Everything Huddle does is reachable and inspectable from the terminal by a person**, without an agent: status, artifacts, decisions, personas, teams, hooks, verdicts, metrics. The agent skill is a driver of the same commands, never a hidden second path.
2. **Output is human-readable by default, machine-readable on request:** tables/trees/colour on a TTY; `--json` (stable schema) for agents and scripts; `--md` where a pasteable report helps. Colour and spinners off when not a TTY (crossmem pattern).
3. **Nothing is only in a chat.** Every agent-driven step leaves something a human can see with one command: `huddle log` (event timeline), `huddle sdlc status` (where each change is), `huddle <artifact> show`, `huddle drafts` (pending drafts awaiting review), `huddle gates` (what's waiting for a human).
4. **Humans can drive every gate from the CLI:** `huddle gate approve|reject <id> [--reason] [--as <owner>]` — the approver must be a configured owner who is not the artifact's writer; calls from an agent session (`HUDDLE_AGENT=1`, or non-TTY without `--as`) are rejected (ADR 19 row 7). The same events (and hooks, ADR 17) fire whether the approval came from chat or terminal.
5. **Dry-run everywhere that writes:** `--dry-run` prints the exact files, diffs, hooks that would run.
6. **`huddle` with no args** prints the current repo's SDLC status + the single next action (`sdlc next`) + pending gates — the at-a-glance view.
7. *(Deferred.)* **Optional live view:** `huddle watch` (terminal, re-renders on log change). No web dashboard in core.

### B. Artifact catalog — one noun per artifact, same verbs

8. **Every artifact type is a CLI noun with the same verbs:** `new | list | show | status | lint` (approval via `huddle gate`, ADR 19).

   | Noun | What | Location (playbook, ADR 12) | Typical source |
   |---|---|---|---|
   | `intent` | problem, outcome, affected users/systems, constraints, open questions | `intent/<slug>/intent.md` | idea, complaint, finding, spike result |
   | `spec` | requirements + acceptance checks + design + policy concerns | `intent/<slug>/spec.md` | approved intent |
   | `plan` | files that change, order, risks, proof | `intent/<slug>/plan.md` | approved spec |
   | `adr` | decision record: decision, context, consequences, `binds`/`prevents`, status | `docs/adr/<n>-<slug>.md` | huddle decision events, spikes |
   | **`spike`** | time-boxed investigation (see C) | `spikes/<n>-<slug>/spike.md` (+ `spikes/<n>-<slug>/code/` if any) | open question, one-way door lacking evidence |
   | `guardrails` | `CLAUDE.md` (Commands · Conventions · Architecture · Things Claude gets wrong) + `AGENTS.md` | repo root | repo scan, decisions, reviews |
   | `review-policy` | `REVIEW.md` (Passes · What Important means here · Cap the nits · Do not report) | repo root | team conventions, repeated findings |
   | `eval` | prompt + checks | `evals/*.json` (+ workflow draft) | real tasks, incidents |
   | `bands` | metric · baseline · rules · tiers | `bands.yaml` | maintain stage |
   | `skill` | team policy as a skill | `.claude/skills/<name>/SKILL.md` (host-rewritten) | design/build policy |
   | **`hooks`** | Huddle lifecycle hooks + harness hook drafts | `hooks[]` in `.huddle/config.json`; drafts for `.claude/settings.json`, `.claude/hooks/*` | integrations, gates (ADR 17) |
   | `verify` | verdict + fingerprint (review notes) | log | ADR 14 §6 |
   | `lessons` | post-mortem: incident, cause, eval added, intent | `docs/lessons/<date>-<slug>.md` (Huddle convention) | incidents, bands breach |
   | `persona` | room members (groups deferred) | `.huddle/personas/` | ADR 3, 4, 10, 16 |

   Examples: `huddle spike new "can pgvector handle 50M rows under 50ms p95?" --timebox 2d`, `huddle adr list --status proposed`, `huddle intent show pay-retries`, `huddle hooks new --on gate.request --stage deploy`, `huddle guardrails status` (stale lines, missing sections).

9. **Every noun has a writer** (ADR 12) and is **draftable by a subagent, reviewable by the room, approvable by a human** — identical six beats (ADR 16). Paths follow the playbook layout or a `custom` mapping.

10. **Artifacts link to each other and to the log.** Front matter `links: {intent, spec, plan, adrs[], spikes[], external[]}`; `huddle <noun> show --graph` prints the chain as a text tree (intent → spikes → ADRs → spec → plan → verdicts → external links from hooks).

### C. Spikes are first-class

11. **A spike answers one question with evidence, inside a timebox, and ends in a recommendation.**
    - Template: Question · Why it matters (which decision/door it unblocks) · Hypothesis · Timebox · Approach · Findings (with evidence: benchmarks, links, code refs) · Recommendation · Follow-ups (→ ADR / intent / plan change) · Status (`open|running|done|abandoned`).
    - Throwaway code lives in `spikes/<n>-<slug>/code/` or a branch named in front matter; never merged into product code by Huddle.
12. **When Huddle proposes a spike:** a one-way door with `claim: unverified` evidence (ADR 14), a design choice personas can't settle from the repo, or a build task that failed its proof twice. The room suggests; the user starts it.
13. **Spikes are orchestrated like build tasks:** a subagent / worktree session runs the approach inside the timebox (ADR 15/16); `huddle spike status` shows time used; expiry is checked on the next `huddle` call (no daemon) and fires `spike.timebox.expired` (hook-able) and forces a recommendation or abandon.
14. **A finished spike must produce at least one of:** an ADR (decision made), an intent (new work), or a plan amendment — `huddle spike close <id> --adr|--intent|--plan` drafts it with the spike linked as evidence.

## Context

- Owner: "all stuff should be mostly CLI based so they can see it"; "I need ADR as well — ADR, spikes, spec, intent, guardrails and hooks and all."
- Agent-only flows hide state in transcripts; a CLI that humans use directly makes the process auditable and lets teams adopt Huddle incrementally (some people via agent, some via terminal).
- The playbook's audit trail is commit history plus artifacts; a uniform noun/verb CLI makes that trail browsable. Spikes are the missing bridge between "we don't know" and an ADR — without them, one-way doors get decided on opinion.

## Consequences

- Command surface grows, but uniformly: ~12 nouns × the same 8 verbs; help and completions are generated from one registry.
- Adds event types `spike.*` to ADR 17 and noun lint rules to ADR 9.
- Human and agent paths are identical, so evals can drive the CLI directly for most assertions (cheaper than LLM e2e).

## Open questions

- Spike location: `spikes/<n>-<slug>/` at repo root vs `docs/spikes/`?
- Resolved (ADR 19): `huddle watch` deferred.

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §A1-A3 (every noun reachable without an agent — `huddle log`, `huddle timeline`,
`huddle sdlc status`, `huddle <noun> show`); §A2's `--json` everywhere; §A4 gates from the CLI with
separation of duties and agent-session rejection (`internal/gates/gates.go:145` the agent env list,
`:264-300` approver-is-an-owner-and-not-the-writer); §A5 `--dry-run` on every writer
(`internal/app/cmd_artifacts.go:56`, `:171`); §A6 bare `huddle` prints `sdlc status`
(`internal/app/app.go:90-92`); §B8 the full noun catalog with `new|list|show|status|lint`
(`internal/artifacts/kinds.go:39-68`, `internal/app/cmd_artifacts.go:23`); §B9 writers per noun
(`skills/huddle-sdlc/stages/*.md`); §C11-C14 spikes as a first-class noun with the full template
(`internal/artifacts/kinds.go:51-53`, `assets/templates/spike.md`).

**Not landed:**

- §B10 `huddle <noun> show --graph` — no `--graph` flag anywhere in `internal/`. Front-matter `links`
  are parsed but the chain is never printed as a tree.
- §A7 `huddle watch` — deferred by ADR 19 (expected).

**Resolved 2026-10-11 — the lint is the authority, the folder was wrong.** `huddle adr lint`
requires YAML front matter (`id`, `kind`, `status`, `author`) and `## Status`, `## binds`,
`## prevents` headings (`internal/artifacts/kinds.go:48-50`, `internal/artifacts/lint.go:131-164`);
all 20 ADRs in this folder reported 4 findings each, and the fix was to the ADRs. They now carry
front matter plus those sections; `huddle adr lint` is clean. The status vocabulary is the CLI's
`draft|proposed|approved|superseded` (`internal/artifacts/store.go:42`) and not `accepted`, because
`approved` is load-bearing in code — `internal/gates/gates.go:347` writes `status: approved` into an
artifact's front matter when a human approves its gate, `internal/sdlc/sdlc.go:254` will not advance
a stage until it reads `approved`, the deploy-gate hook matches on it
(`internal/hooks/export.go:327`) and `huddle serve` publishes the list
(`internal/serve/api.go:120`). `accepted` appears nowhere in `internal/`. An ADR folder spelling it
`accepted` would be overwritten by the next gate approval and would never register as done in
`huddle sdlc status`; one vocabulary across eleven artifact kinds beats matching ADR literature.

## binds`, `## prevents` headings
(`internal/artifacts/kinds.go:48-50`, `internal/artifacts/lint.go:131-164`), while
`docs/adr/README.md` prescribes a `- Status:` bullet and Decision → Context → Details → Consequences
→ Open questions. All 20 ADRs in this folder therefore fail `huddle adr lint` — 80 findings, every
one of them about the format the folder's own README mandates. Separately, the lint's status
vocabulary is `draft|proposed|approved|superseded` (`internal/artifacts/store.go:42`) while the
README and ADR 7 §3 / ADR 14 §7 all say `accepted`. Pick one vocabulary and one ADR shape.

## binds

- Every artifact is a CLI noun with the same verbs (`new|list|show|status|lint`), usable by a human
  with no agent in the room.
- The skill is a driver of those same commands, and every flag it names exists in the registry.

## prevents

- No hidden second path that only agents can take.
- No artifact that only the skill knows how to write, and no output a person cannot inspect from a
  terminal.
