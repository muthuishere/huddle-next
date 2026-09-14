# ADR 16: Huddle Is the AI-Native SDLC Orchestrator

- Status: proposed
- Date: 2026-09-14
- v1: built; deferred: §3 dispatch automation (v1: plan + task brief), §10 `huddle team` (ADR 19)
- Product spine — every other ADR serves this one.
- Builds on: ADR 12 (writers, playbook layout), 14 (event log, verdicts, room modes), 15 (subagents)

## Decision

1. **What Huddle is.** Huddle is the agent that takes a user or team through the **entire AI-native SDLC** — Setup → Plan → Design → Build → Test → Deploy review → Maintain — and back into Plan. It:
   - **orchestrates** each stage (what's next, who does it, what "done" means),
   - **convenes the room** (expert personas) wherever judgment is needed,
   - **delegates the work** to subagents / parallel harness sessions (ADR 15),
   - **writes and links the artifacts** in the playbook layout (ADR 12),
   - **holds the gates** — a human approves every stage transition and every one-way door.

   Huddle is not a spec format, not a CI system, not a deploy tool. It sits above them and uses whatever the repo already has.

2. **Stage map** (one lazy-loaded `stages/<stage>.md` each; playbook file locations; `custom` mapping may relocate):

   | Stage | Huddle helps the user… | Orchestrates (subagents) | Artifact | Gate |
   |---|---|---|---|---|
   | **0. Setup** | make the repo agent-ready | scout reads repo → guardrails writer | `CLAUDE.md` (Commands · Conventions · Architecture · Things Claude gets wrong) + `AGENTS.md`; policy skills in `.claude/skills/`; hooks (ADR 17); starter `.huddle/personas/` room | user approves guardrails diff |
   | **1. Plan** | turn an idea, complaint, or finding into an intent | scouts (code + cross-agent history, ADR 8) → intent writer | `intent/<slug>/intent.md` | owner accepts intent |
   | **2. Design** | requirements + design in one session, policy applied while writing | spec writer; lenses (security, compliance, UX) in parallel; ADR writer for decisions | `intent/<slug>/spec.md` (+ ADRs) | PASS/CONCERNS/FAIL spec review; one-way doors → `owners` approver (ADR 19) |
   | **3. Build** | plan before code, then run the build | plan writer → task brief per step; the harness builds (dispatch automation deferred, ADR 19) | `intent/<slug>/plan.md`; diffs on a branch | user approves plan before any code; diff-vs-plan drift check |
   | **4. Test** | define measurable targets; verify before any human review | each build task iterates to its proof target; verification lenses; incident → eval case | `evals/*.json` + `.github/workflows/agent-evals.yml` draft; verdict stamped with tree fingerprint | PASS verdict on current fingerprint |
   | **5. Deploy review** | review against intent/spec/plan, prepare the release | reviewer lenses vs spec + plan | `REVIEW.md` policy (repo root); findings vs `REVIEW.md` + spec + plan; hook gate drafts (`.claude/settings.json`, `.claude/hooks/`) | human code owner approves merge; prod is always a human one-way door |
   | **6. Maintain** | turn incidents, metrics, scans, repeated review findings back into work | scout diagnoses from what the user/tools ingest (ADR 10 route) → intent writer; guardrails refresh ledger; wrap-up action follow-through | `bands.yaml`; new `intent/<slug>/intent.md` from a breach or finding; learnings; `CLAUDE.md` refresh | owner triages |

3. **Build orchestration, precisely** (v1: task brief only; dispatch automation deferred, ADR 19). Huddle dispatches implementation to the **harness's own** execution primitives (subagents, agent teams, parallel worktree sessions) — not a Huddle-private builder flow (the old Sreyash flow stays removed, ADR 11). For each plan step: task brief (`huddle write task-brief <step>` = plan step + spec requirements + guardrails + persona style cards) → worker → proof target → result recorded in the log. Independent steps run in parallel; dependent steps wait. Workers never merge, push, or deploy. **Fix tasks:** the failing test is committed first; test paths are protected in the brief and by the exported hook (ADR 17 §8).

4. **Every stage runs the same six beats:**
   1. **Detect** — `huddle sdlc status --json`: current change, stage per artifact, stale verdicts, open one-way doors, guardrail freshness.
   2. **Interview** — ≤ 7 stage questions served by the CLI, skipping what repo/log already answer; one at a time, each with a `RECOMMENDATION:`.
   3. **Delegate** — subagents gather, draft, build, or check (ADR 15); results land as files + log events.
   4. **Room** — 2–3 relevant personas + lenses critique; `session` room in v1 (ADR 14 §2).
   5. **Gate** — a human owner who is not the writer sees diff + verdict and decides (ADR 18 §4); one-way doors never auto-decided.
   6. **Record** — artifact written, `stage-complete` event with path + fingerprint; Huddle suggests the commit, never runs it.

5. **`huddle sdlc next`** — the orchestrator's heartbeat: lowest incomplete stage for the active change and the single next action. Works on an empty repo (starts at Setup) and on a repo mid-flight (reads existing artifacts, including `custom`-mapped ones).
6. **Entry from anywhere.** Users can start at any stage ("review this PR", "we had an incident", "write our AGENTS.md"); Huddle backfills only what that stage needs and shows skipped gates as warnings, never blocks.
7. **Teaching built in.** Each stage carries a ≤ 10-line "what good looks like" with one tiny example (good intent, requirement + acceptance check, `AGENTS.md` line like "Money is always BigDecimal, never double"), shown the first time a user does that stage.
8. **Metrics from log + git, local only** (`huddle sdlc status --metrics`): intent→spec→plan lead times, spec rework after build start, first-pass verification PASS rate, diff-vs-plan drift, time from finding to intent, open actions carried across huddles.
9. **Hard limits:** Huddle never pushes, merges, deploys, rotates credentials, or changes CI/branch protection. It may **draft** hooks, review configs, or CI snippets for the user to adopt.
10. *(Deferred, ADR 19.)* **A team for anything, not only SDLC stages.** `huddle team "<goal>"` assembles a room for any task — a migration, a pricing decision, a launch post, an incident, a vendor evaluation: it picks personas (resolved roster, saved groups, or proposed ad-hoc roles), chooses a room mode (ADR 14), splits the work into subagent jobs (ADR 15), and runs the same beats — detect, interview, delegate, room, gate, record. SDLC stages are simply the best-supported, pre-wired teams. `huddle team save <name>` turns a good ad-hoc team into a reusable group.
11. **Skill packaging:** the `huddle-sdlc` skill (ADR 15 §D) is the orchestrator's entry; it hands off to the `huddle` room and `huddle-personas` as needed — one binary, one log, one set of artifacts.

## Context

- Owner: "we are the agent who can help them do the entire AI-native SDLC — orchestrator"; users need help building intent, `CLAUDE.md` / `AGENTS.md`, and specs.
- The AI-native SDLC playbook defines the stages, artifacts, gates, and metrics, and assigns execution to agents with humans above it — but names no orchestrator. Huddle already has the room (personas), the writers (ADR 12), the log (ADR 14), and subagent-first execution (ADR 15); this ADR ties them into one loop.
- BMAD (planning path, readiness gates, retros), gstack (think → plan → build → review → test → ship → reflect) each cover parts; Huddle orchestrates the whole loop on the playbook's own artifacts.

## Consequences

- Huddle's product surface becomes "run your SDLC with agents" rather than "hold a discussion"; the room is one capability inside it.
- Build dispatch makes Huddle responsible for coordinating parallel workers — ADR 13 budgets and ADR 9 evals must cover it (e2e: empty repo → intent → spec → plan → one built task → PASS verdict).
- Every stage needs a stage file, interview catalog, and eval case; the ADR 13 byte budgets apply to each.

## Open questions

- Resolved (ADR 19): v1 ships plan + task brief; dispatch automation deferred.
