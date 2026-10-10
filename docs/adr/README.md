# Huddle ADRs — Go CLI

**Huddle is the AI-native SDLC orchestrator**: the agent that takes a team through every stage — setup, intent, spec, plan, build, test, deploy review, maintain — with a room of expert personas for judgment, subagents for the work, and a human gate at each stage. Artifacts use the AI-native SDLC playbook's own files and headings; no spec-framework dependency. **v1 scope: ADR 19**; playbook coverage: `stress-test.md`.

These ADRs define the Go CLI + skills that make that possible (ADR 16 is the product spine).

| # | Decision | Status |
|---|---|---|
| [1](1-go-cli-single-binary.md) | One CGO-free Go binary, npm-wrapped, `huddle install --skills` | **accepted** |
| [2](2-one-preflight-lazy-network.md) | One startup call (now `huddle start`, ADR 14); GitHub context lazy + cached | **accepted** |
| [3](3-persona-layers-deterministic-merge.md) | Persona layers: builtin < `~/.config/.huddle/personas` < `<repo>/.huddle/personas`, merged in code | **accepted** |
| [4](4-persona-authoring-flow.md) | `huddle persona new` — first-class persona creation | proposed (partial) |
| [5](5-repo-persona-trust.md) | Hash-approved trust for repo-level personas (deferred, ADR 19) | proposed (deferred) |
| [6](6-state-storage-v2.md) | Versioned, atomic, permissive state storage | proposed (partial) |
| [7](7-huddle-in-the-sdlc-artifact-chain.md) | Huddle outputs join the intent → spec → plan → ADR chain | proposed (partial) |
| [8](8-cross-agent-context-via-crossmem.md) | Cross-agent session context via crossmem as a library (crossmem side implemented) | proposed (partial) |
| [9](9-parity-lint-and-evals.md) | Parity fixtures, asset lint, behaviour evals | proposed (partial) |
| [10](10-persona-sources-and-style-api.md) | Personas from any data via one ingest route; import, propose, style API | **accepted** |
| [11](11-slim-the-skill.md) | Cut or lazy-load what costs time | proposed (partial) |
| [12](12-writers-on-playbook-layout.md) | Writers on the playbook layout (intent, spec, plan, ADR, spike, guardrails, REVIEW.md, evals, bands, skills, hooks); no spec framework | **accepted** |
| [13](13-performance-and-context-budget.md) | Performance as enforced budgets: wall-clock, tool calls, tokens | proposed (partial) |
| [14](14-adopt-from-bmad-and-gstack.md) | Adopt from BMAD-METHOD and gstack: event log, room modes, verdicts, lenses, hosts | proposed (partial) |
| [15](15-subagent-first-and-skill-practices.md) | Subagent-first execution, Huddle agent definitions, skill best practices, skill family | **accepted** |
| [16](16-ai-native-sdlc-orchestrator.md) | **Product spine:** Huddle orchestrates the entire AI-native SDLC (setup → maintain) | **accepted** |
| [17](17-lifecycle-hooks.md) | Lifecycle hooks at every stage — `hooks[]` in global/repo `config.json`, no trust hashing; links as source-of-truth | proposed (partial) |
| [19](19-v1-scope-and-value-gate.md) | **v1 scope + value gate** — what is built, what stays deferred; resolves contradictions | **accepted** |
| [20](20-huddle-serve-dashboard-and-local-analytics.md) | `huddle serve` dashboard on :16211 + local-only CLI/skill usage analytics | **accepted** |
| [18](18-cli-first-artifact-catalog.md) | CLI-first and human-visible; every artifact a CLI noun — intent, spec, plan, ADR, spike, guardrails, review policy, evals, bands, skills, hooks | proposed (partial) |

## Status, as of 2026-10-10

Nine ADRs were flipped `proposed → accepted` after the owner directed the v1-readiness build:
1, 2, 3, 10, 12, 15, 16, 19, 20. Each cites huddle event `20261010T045742.452Z-01d288`
(`huddle log list --type decision`), which cites the originating raw decision below.

The rest stay `proposed` on purpose:

- **`proposed (deferred)`** — ADR 5: trust hashing is deferred by ADR 19.
- **`proposed (partial)`** — ADRs 4, 6, 7, 8, 9, 11, 13, 14, 17, 18: some numbered clauses shipped
  and some did not. Each one's own **Open questions** names exactly which, with `file:line`. Read
  that before building on the ADR; an accepted-looking ADR with a hole in it is worse than a
  `proposed` one. None of them were flipped.

Two things the code and these ADRs disagree about, both open:

- **`accepted` vs `approved`.** The Format section below and ADR 7 §3 / ADR 14 §7 say `accepted`;
  the CLI's status vocabulary is `draft|proposed|approved|superseded`
  (`internal/artifacts/store.go:42`). The flips above use `accepted`, per this README.
- **This folder fails `huddle adr lint`.** The `adr` kind requires YAML front matter plus `## Status`,
  `## binds`, `## prevents` (`internal/artifacts/kinds.go:48-50`); this folder uses a `- Status:`
  bullet and the crossmemcli shape. All 20 files report findings. See ADR 18's Open questions.

## Inputs

- **ADR status flip 2026-10-10** — owner decision recorded as huddle event `20261010T045742.452Z-01d288`
  on branch `build/v1-readiness`; verified against `internal/`, `skills/`, `assets/`, `cmd/`,
  `.github/`, `npm/` and `.goreleaser.yaml`, not against the ADRs' own claims.
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
a huddle decision by the owner; flip it here and cite the raw event. A flip also rewrites the
`- Status:` line in the ADR itself, citing the `huddle log append --type decision` event id for the
flip (which in turn points back at the raw event) — see any accepted ADR above for the exact shape.
An ADR whose decision is only partly implemented is **not** flipped; the unbuilt clauses are named
in its Open questions instead, with `file:line`.
