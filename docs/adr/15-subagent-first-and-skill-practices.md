---
id: adr-15
kind: adr
status: approved
author: muthuishere
links: {}
---
# ADR 15: Subagent-First Execution and Agent-Skill Best Practices

- Date: 2026-09-14
- v1: built: §A, §C, §D; deferred: §B agent definitions, per-persona subagent views (ADR 19)
- Builds on: ADR 13 (budgets), ADR 14 §2 (room modes)

## Status

**approved** — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw
decision: `README.md` → Inputs). The front matter `status:` field is the status of record.

## Decision

### A. The main conversation is the room, not the workshop

1. **The main context holds only what the user must see:** persona voices, the decision question, the recommendation, and the user's call. Everything that reads many files, gathers data, drafts long artifacts, or checks work runs in a **subagent** and returns a compact result.
2. **Default delegation table:**

   | Work | Runs in | Returns to main |
   |---|---|---|
   | Repo reconnaissance for a topic (code, docs, spec artifacts, ADRs) | read-only explore subagent | ≤ 400-token brief + file:line citations |
   | Cross-agent history (ADR 8) | subagent calling `huddle context --agents` + `load` | what was tried, decided, left open |
   | Data gathering for personas (ADR 10) | subagent using whatever tools the harness has → `huddle persona ingest` | counts + proposal list |
   | Persona drafting / import rewrite (ADR 4, 10) | subagent with `huddle persona brief` style context | draft file path for review |
   | Writers — intent, spec, plan, ADR, guardrails (ADR 12) | one subagent per writer | draft path + lint result + 5-line summary |
   | *(deferred)* Independent persona views on one-way doors (ADR 14 §2) | one subagent per persona | each position ≤ 150 tokens |
   | Verification lenses (ADR 14 §6) | one subagent per lens, parallel | canonical JSON findings |
   | Log distillation / resume brief | subagent | ≤ 300-token brief |

   The main loop never pastes a subagent's raw file dumps; it relays the result.
3. **Parallel by default when independent:** lenses, writers for separate artifacts, and per-persona views are launched together in one turn.
4. **Files are the hand-off, not chat.** Subagents write drafts to `~/.config/.huddle/repos/<repo>/drafts/<id>.md` (never the repo's spec folder) and return the path; the next stage reads the file. Nothing is written into the repo until the user approves in the main conversation (review-before-write stays in the room).
5. **Graceful degradation:** harnesses without subagents run the same step inline with the same budgets (ADR 13); the skill checks capability once at start (`huddle start` reports `subagents: true|false` from the host table).

### B. Huddle ships its own agent definitions (deferred, ADR 19)

6. **`huddle install --skills` also installs Huddle subagent definitions** where the harness supports them (e.g. `.claude/agents/huddle-*.md` user-level): `huddle-scout` (read-only recon), `huddle-writer` (drafts only to the drafts dir), `huddle-lens` (review lens runner), `huddle-gatherer` (data → ingest). Each declares a narrow tool set — scouts and lenses get read-only tools; writers get write access to drafts only — and a short system prompt that points at `huddle` subcommands instead of long instructions.
7. **Personas are not subagents by default.** A persona becomes a subagent only for independent views (ADR 14 room modes); otherwise it is a style card in the room — cheaper and keeps the conversation coherent.

### C. Agent-skill practices Huddle follows (and lints, ADR 9)

8. **Progressive disclosure, three levels:** (1) front matter `name` + `description` is always loaded — keep it trigger-rich and ≤ 260 bytes; (2) `SKILL.md` body loads on trigger — a router under ~150 lines; (3) `modes/`, `writers/`, `personas/`, `stages/` files load only when needed (ADR 13 manifest).
9. **Scripts are executed, not read.** All deterministic work is a `huddle` subcommand; skill text says *when* to call it and *what the output means*, never re-explains the logic.
10. **Descriptions say when to use and when not to**, with concrete trigger phrases; separate skills only where triggers genuinely differ (see D).
11. **One job per reference file**, named by what it's for; no file both routes and instructs.
12. **Instructions are imperative and testable** ("call `huddle start`; if `HUDDLE_PROTO` ≠ 1 use degraded flow") — every rule maps to an eval assertion (ADR 9).
13. **No duplicated knowledge across files.** A rule lives once; others link to it. Lint flags near-duplicate paragraphs across embedded assets.
14. **Harness-neutral text.** Tool names and paths are rewritten per host at install (ADR 14 §11); assets never hard-code `.claude` paths.

### D. Skill packaging

15. `huddle install --skills` installs a **small skill family** instead of one monolith, each with a tight description so the right one triggers without loading the others:
    - `huddle` — discussions, decisions, verification, wrap-up (the room).
    - `huddle-sdlc` — the AI-native SDLC orchestrator entry: setup, intent, spec, plan + task brief, test, deploy review, maintain (ADR 16).
    - `huddle-personas` — create, import, ingest, find, style, brief (ADR 4, 10).
    All share one binary and one asset tree; they differ only in router text. `huddle context-bill` reports each one's always-on cost.

## Context

- Owner: "try most things in sub agent and adapt best ways of using agent skills."
- The 2026-09-13 OpenCode huddle and this session both showed the cost of doing recon inline: large file reads and inventories filled the main context; the same work via explore subagents returned compact reports.
- The AI-native SDLC playbook: subagents handle repeated scoped jobs (verify, research, format) with their own context and tool limits; parallel sessions/worktrees for independent work.
- BMAD's "extract, don't ingest" (subagents write full output to files, return summaries) and room modes; gstack's parallel specialist reviewers and lazy sections.

## Consequences

- Main-context growth per huddle becomes roughly linear in decisions, not in files read.
- More tokens overall (subagents re-read context), fewer in the expensive long-lived main window; budgets in ADR 13 apply per subagent return.
- Installing agent definitions writes outside the skill dir; they carry the `.huddle-owned` marker and are removed by `uninstall`.

## binds

- The main conversation holds only what the user must see: persona voices, the question, the
  recommendation, the user's call.
- Work that reads many files, gathers data, drafts long artifacts or checks results runs in a
  subagent and returns a compact result.

## prevents

- No bulk file reading or long tool output in the room's context.
- No persona voice spent on mechanical work a subagent should have done.
