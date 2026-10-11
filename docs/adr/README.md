# Huddle ADRs — Go CLI

**Huddle is the AI-native SDLC orchestrator**: the agent that takes a team through every stage — setup, intent, spec, plan, build, test, deploy review, maintain — with a room of expert personas for judgment, subagents for the work, and a human gate at each stage. Artifacts use the AI-native SDLC playbook's own files and headings; no spec-framework dependency. **v1 scope: ADR 19**; playbook coverage: `stress-test.md`.

These ADRs define the Go CLI + skills that make that possible (ADR 16 is the product spine).

| # | Decision | Status |
|---|---|---|
| [1](1-go-cli-single-binary.md) | One CGO-free Go binary, npm-wrapped, `huddle install --skills` | **approved** |
| [2](2-one-preflight-lazy-network.md) | One startup call (now `huddle start`, ADR 14); GitHub context lazy + cached | **approved** |
| [3](3-persona-layers-deterministic-merge.md) | Persona layers: builtin < `~/.config/.huddle/personas` < `<repo>/.huddle/personas`, merged in code | **approved** |
| [4](4-persona-authoring-flow.md) | `huddle persona new` — first-class persona creation | proposed (partial) |
| [5](5-repo-persona-trust.md) | Hash-approved trust for repo-level personas (deferred, ADR 19) | proposed (deferred) |
| [6](6-state-storage-v2.md) | Versioned, atomic, permissive state storage | proposed (partial) |
| [7](7-huddle-in-the-sdlc-artifact-chain.md) | Huddle outputs join the intent → spec → plan → ADR chain | proposed (partial) |
| [8](8-cross-agent-context-via-crossmem.md) | Cross-agent session context via crossmem as a library (crossmem side implemented) | proposed (partial) |
| [9](9-parity-lint-and-evals.md) | Parity fixtures, asset lint, behaviour evals | proposed (partial) |
| [10](10-persona-sources-and-style-api.md) | Personas from any data via one ingest route; import, propose, style API | **approved** |
| [11](11-slim-the-skill.md) | Cut or lazy-load what costs time | proposed (partial) |
| [12](12-writers-on-playbook-layout.md) | Writers on the playbook layout (intent, spec, plan, ADR, spike, guardrails, REVIEW.md, evals, bands, skills, hooks); no spec framework | **approved** |
| [13](13-performance-and-context-budget.md) | Performance as enforced budgets: wall-clock, tool calls, tokens | proposed (partial) |
| [14](14-adopt-from-bmad-and-gstack.md) | Adopt from BMAD-METHOD and gstack: event log, room modes, verdicts, lenses, hosts | proposed (partial) |
| [15](15-subagent-first-and-skill-practices.md) | Subagent-first execution, Huddle agent definitions, skill best practices, skill family | **approved** |
| [16](16-ai-native-sdlc-orchestrator.md) | **Product spine:** Huddle orchestrates the entire AI-native SDLC (setup → maintain) | **approved** |
| [17](17-lifecycle-hooks.md) | Lifecycle hooks at every stage — `hooks[]` in global/repo `config.json`, no trust hashing; links as source-of-truth | proposed (partial) |
| [19](19-v1-scope-and-value-gate.md) | **v1 scope + value gate** — what is built, what stays deferred; resolves contradictions | **approved** |
| [20](20-huddle-serve-dashboard-and-local-analytics.md) | `huddle serve` dashboard on :16211 + local-only CLI/skill usage analytics | **approved** |
| [18](18-cli-first-artifact-catalog.md) | CLI-first and human-visible; every artifact a CLI noun — intent, spec, plan, ADR, spike, guardrails, review policy, evals, bands, skills, hooks | proposed (partial) |
| [21](21-autonomous-decider-mode-headless-owner-resolved-huddles.md) | Autonomous decider mode — headless, owner-resolved huddles | proposed (being written) |
| [22](22-ask-back-endpoint-a-huddle-asks-a-question-and-resumes.md) | Ask-back endpoint — a huddle asks a question and resumes | proposed (being written) |

## Status, as of 2026-10-11

ADRs 21 and 22 were created on 2026-10-11 and are still being written — their bodies are template
placeholders, so read them as drafts, not as decisions.

Nine ADRs were flipped `proposed → approved` after the owner directed the v1-readiness build:
1, 2, 3, 10, 12, 15, 16, 19, 20. Each cites huddle event `20261010T045742.452Z-01d288`
(`huddle log list --type decision`), which cites the originating raw decision below.

The rest stay `proposed` on purpose:

- **`proposed (deferred)`** — ADR 5: trust hashing is deferred by ADR 19.
- **`proposed (partial)`** — ADRs 4, 6, 7, 8, 9, 11, 13, 14, 17, 18: some numbered clauses shipped
  and some did not. Each one's own **Open questions** names exactly which, with `file:line`. Read
  that before building on the ADR; an approved-looking ADR with a hole in it is worse than a
  `proposed` one. None of them were flipped.

### Two things the code and these ADRs used to disagree about — both resolved 2026-10-11

- **`accepted` vs `approved`: `approved` won, and these ADRs now say `approved`.** The word is
  load-bearing in code, not a style choice. `internal/gates/gates.go:347` writes
  `status: approved` into an artifact's front matter when a human approves its gate;
  `internal/sdlc/sdlc.go:254` refuses to advance a stage until it reads `approved`; the exported
  deploy-gate hook matches on it (`internal/hooks/export.go:327`); `huddle serve` publishes the
  vocabulary (`internal/serve/api.go:120`). `accepted` appears nowhere in `internal/`. An ADR
  folder spelling it `accepted` would be silently overwritten by the next gate approval and would
  never register as done in `huddle sdlc status` — so one vocabulary across all eleven artifact
  kinds beats matching the ADR literature's habit.
- **This folder now passes `huddle adr lint`.** Every ADR carries YAML front matter
  (`id`, `kind`, `status`, `author`, `links`) and the `## Status`, `## binds`, `## prevents`
  sections the `adr` kind requires (`internal/artifacts/kinds.go:48-50`). The 80 findings were
  80 real ones: huddle cannot sell a linted artifact catalog while its own catalog fails the lint.

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

`huddle adr new "<title>"` writes the shape; `huddle adr lint` enforces it. Required:

- **YAML front matter** — `id: adr-<n>`, `kind: adr`, `status:`, `author:`, `links:`. The front
  matter `status:` is the status **of record**; the `## Status` section is prose about it.
- **`status:` is one of `draft | proposed | approved | superseded`** — the CLI's vocabulary for
  every artifact kind (`internal/artifacts/store.go:42`). Not `accepted`: see the resolved
  disagreement above.
- **Sections** — `## Status`, `## Context`, `## Decision`, `## Consequences`, `## binds`,
  `## prevents`. `## Details` and `## Open questions` are optional and widely used here. A
  numbered, normative Decision is this folder's convention, inherited from crossmemcli.
- **`binds` / `prevents`** (ADR 14 §7) — what the decision commits future work to, and what it
  rules out. An approved ADR's `binds` are inherited invariants for later huddles, so write them
  as rules someone can be held to, not as a summary.

Status moves `proposed → approved` only after a huddle decision by the owner: flip it in the ADR's
front matter and in the table above, and cite the `huddle log append --type decision` event id in
the `## Status` section (which in turn points back at the raw event). See any approved ADR above
for the exact shape. An ADR whose decision is only partly implemented is **not** flipped; the
unbuilt clauses are named in its Open questions instead, with `file:line`.
