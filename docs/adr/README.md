# Huddle ADRs — Go CLI

**Huddle is the AI-native SDLC orchestrator**: the agent that takes a team through every stage — setup, intent, spec, plan, build, test, deploy review, maintain — with a room of expert personas for judgment, subagents for the work, and a human gate at each stage. Artifacts use the AI-native SDLC playbook's own files and headings; no spec-framework dependency. **v1 scope: ADR 19**; playbook coverage: `stress-test.md`.

These ADRs define the Go CLI + skills that make that possible (ADR 16 is the product spine).

| # | Decision | Status |
|---|---|---|
| [1](1-go-cli-single-binary.md) | One CGO-free Go binary, npm-wrapped, `huddle install --skills` | proposed |
| [2](2-one-preflight-lazy-network.md) | One startup call (now `huddle start`, ADR 14); GitHub context lazy + cached | proposed |
| [3](3-persona-layers-deterministic-merge.md) | Persona layers: builtin < `~/.config/.huddle/personas` < `<repo>/.huddle/personas`, merged in code | proposed |
| [4](4-persona-authoring-flow.md) | `huddle persona new` — first-class persona creation | proposed |
| [5](5-repo-persona-trust.md) | Hash-approved trust for repo-level personas (deferred, ADR 19) | proposed |
| [6](6-state-storage-v2.md) | Versioned, atomic, permissive state storage | proposed |
| [7](7-huddle-in-the-sdlc-artifact-chain.md) | Huddle outputs join the intent → spec → plan → ADR chain | proposed |
| [8](8-cross-agent-context-via-crossmem.md) | Cross-agent session context via crossmem as a library (crossmem side implemented) | proposed |
| [9](9-parity-lint-and-evals.md) | Parity fixtures, asset lint, behaviour evals | proposed |
| [10](10-persona-sources-and-style-api.md) | Personas from any data via one ingest route; import, propose, style API | proposed |
| [11](11-slim-the-skill.md) | Cut or lazy-load what costs time | proposed |
| [12](12-writers-on-playbook-layout.md) | Writers on the playbook layout (intent, spec, plan, ADR, spike, guardrails, REVIEW.md, evals, bands, skills, hooks); no spec framework | proposed |
| [13](13-performance-and-context-budget.md) | Performance as enforced budgets: wall-clock, tool calls, tokens | proposed |
| [14](14-adopt-from-bmad-and-gstack.md) | Adopt from BMAD-METHOD and gstack: event log, room modes, verdicts, lenses, hosts | proposed |
| [15](15-subagent-first-and-skill-practices.md) | Subagent-first execution, Huddle agent definitions, skill best practices, skill family | proposed |
| [16](16-ai-native-sdlc-orchestrator.md) | **Product spine:** Huddle orchestrates the entire AI-native SDLC (setup → maintain) | proposed |
| [17](17-lifecycle-hooks.md) | Lifecycle hooks at every stage — `hooks[]` in global/repo `config.json`, no trust hashing; links as source-of-truth | proposed |
| [19](19-v1-scope-and-value-gate.md) | **v1 scope + value gate** — what is built, what stays deferred; resolves contradictions | proposed |
| [20](20-huddle-serve-dashboard-and-local-analytics.md) | `huddle serve` dashboard on :16211 + local-only CLI/skill usage analytics | proposed |
| [18](18-cli-first-artifact-catalog.md) | CLI-first and human-visible; every artifact a CLI noun — intent, spec, plan, ADR, spike, guardrails, review policy, evals, bands, skills, hooks | proposed |

## Inputs

- **Benchmark + decision** — OpenCode huddle 2026-09-13 (`ses_f667d8151ffe…`); raw event
  `~/.config/muthuishere-agent-skills/huddle/licensing-add-license/huddle/raw/1789282587000_decision.json`.
- **Build-readiness inventory** of this repo (same session, explore subagent).
- **AI-native SDLC playbook** — https://claude.com/blog/the-ai-native-sdlc-playbook
  (committed artifact chain, skills/hooks as policy, plan-before-build, continuous evals, human gates).
- **AI Engineer Coach** (Microsoft) — layered markdown assets with id override, hash-based trust,
  front-matter validation, per-harness skill targets, single redaction choke point.
- **BMAD-METHOD** 6.13.0-next and **gstack** 1.84.1.0 — see ADR 14 for adopt/adapt.
- **crossmemcli** (`github.com/muthuishere/crossmemcli`) — goreleaser + npm platform-package wrapper,
  embedded skill install, multi-agent session providers.

## Format

Same shape as crossmemcli's `docs/adr/`: **Decision** (numbered, normative) → **Context** →
**Details** → **Consequences** → **Open questions**. Status moves `proposed → accepted` only after
a huddle decision by the owner; flip it here and cite the raw event.
