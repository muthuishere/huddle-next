---
id: adr-12
kind: adr
status: approved
author: muthuishere
links: {}
---
# ADR 12: Writer Roles on the AI-Native SDLC Playbook Layout — No Spec Framework Dependency

- Date: 2026-09-14
- v1: built; deferred: `custom` mapping, release-notes writer (ADR 19)
- Amends: ADR 7 (artifact formats and locations)

## Status

**approved** — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw
decision: `README.md` → Inputs). The front matter `status:` field is the status of record.

## Decision

1. **One format: the AI-native SDLC playbook (`native`).** Huddle ships no OpenSpec, spec-kit, or BMAD adapter. Writers produce stage artifacts in the playbook's files (decision 5).
2. **`custom` path mapping is the only escape hatch.** A repo can remap where each artifact lives and which template it uses *(deferred, ADR 19)* in `config.json` under `artifacts.*` — e.g. a team already on OpenSpec maps intent → `openspec/changes/<id>/proposal.md`. Huddle does not call, validate against, or track any framework's CLI.
3. **No detection magic, no framework init.** Default is `native`; `custom` mapping is deferred (ADR 19). Huddle never initialises third-party tooling.
4. **Stage artifacts are path-neutral in the log.** The event log (ADR 14 §1) records `artifact: {stage, kind, path}`; writers, `sdlc status` (ADR 16), and evals reason about stages, not about one tool's file names. Changing the mapping mid-project links existing artifacts; nothing is converted.
5. **`native` = the playbook, verbatim.** File names, locations, and template headings are the ones the AI-native SDLC playbook uses; Huddle adds nothing the playbook doesn't name except these **Huddle conventions**: front-matter links to log events, `docs/adr/`, `spikes/`, `.huddle/config.json`, `AGENTS.md`, `docs/lessons/`.

   | Artifact | Location (playbook) | Template headings (playbook) | Stage |
   |---|---|---|---|
   | `intent.md` | "an `intent/` folder in the product repo" → `intent/<slug>/intent.md` | Author · Status · Problem · Proposed outcome · Affected users and systems · Constraints · Open questions | Plan |
   | `spec.md` | "alongside `intent.md`" → `intent/<slug>/spec.md` | requirements + design with flagged policy concerns | Design |
   | `plan.md` | committed as `plan.md`; updated "in the same commit" when implementation departs → `intent/<slug>/plan.md` | Files that change · Order of work · Risks · Proof | Build |
   | `CLAUDE.md` | repo root | Commands · Conventions · Architecture · Things Claude gets wrong | Setup / Build |
   | `REVIEW.md` | repo root | Passes · What Important means here · Cap the nits · Do not report | Deploy |
   | evals | `evals/*.json` + `.github/workflows/agent-evals.yml` | prompt + checks that define acceptable | Test |
   | `bands.yaml` | version-controlled config (repo root by default) | metric · baseline · rules · tiers | Maintain |
   | skills | `.claude/skills/<name>/SKILL.md` | front matter says when it triggers | Design / Build (policy) |
   | hooks / settings | `.claude/settings.json`, `.claude/hooks/*.sh` | hook config | Build / Deploy gates |

   Where the playbook is silent (placing `plan.md` in the same `intent/<slug>/` folder; `AGENTS.md` mirroring `CLAUDE.md` for non-Claude harnesses), the choice is marked in the template as a Huddle convention. **Legacy systems** (Jira, Linear, …): per the playbook, one system is the source of truth per artifact and the other holds a link — wired through hooks (ADR 17).
6. **Writer roles (the layout decides the path; the role decides the content):**

   | Writer | Stage | Persona voice |
   |---|---|---|
   | **Intent writer** | Plan | PM / strategist |
   | **Spec writer** | Design | Elango |
   | **Plan writer** | Build | Engineering (Shaama / Suren) |
   | **ADR writer** | cross-cutting | Elango — `docs/adr/<n>-<slug>.md` |
   | **Guardrails writer** | Setup / Maintain | Deepak — `CLAUDE.md` (playbook sections), `AGENTS.md` (+ `.github/copilot-instructions.md` when present) |
   | **Review-policy writer** | Deploy | Staff reviewer voice — `REVIEW.md` |
   | **Eval writer** | Test | Nina — `evals/*.json` from real tasks and incidents; CI workflow draft |
   | **Bands writer** | Maintain | SRE voice — `bands.yaml` |
   | **Spike writer** | Plan / Design | Vel (diagnostician) — `spikes/<n>-<slug>/spike.md` (ADR 18 §C) |
   | **Task-brief writer** | Build | plan step + spec + guardrails + style cards (ADR 16 §3) |
   | **Hooks writer** | Setup / Deploy | drafts `hooks[]` in `.huddle/config.json` + harness hook pack (ADR 17) |
   | **Skill writer** | Design / Build | encodes team policy as `.claude/skills/<name>/SKILL.md` (host-rewritten for other harnesses) |

7. **Guardrails writer rules** :
   - Shared content in `AGENTS.md`; `CLAUDE.md` holds Claude-specific notes and references it (or mirrors, if the repo already does).
   - **Never clobbers managed blocks** of other tools (crossmem, ctx-optimize, spec tools, any `<!-- … -->` managed section); edits only its own `<!-- huddle:context -->` block or user-approved sections (ledger rules in ADR 14 §7).
   - Evidence-based: commands verified to exist, conventions observed in code, common mistakes from huddle decisions/reviews; one page; flags stale lines.
8. **Validation:** Huddle's structural lint (required playbook headings, acceptance on every requirement, spec-law checks from ADR 14 §7); `custom` templates declare their own required headings.
9. **CLI surface (LLM-free):**
   - `huddle <noun> status [--change <id>] --json` (ADR 18)
   - `huddle <noun> new [--change <id>] [--from-event <id>] --dry-run` → target path, template, context bundle (decisions, persona style cards).
   - `huddle <noun> new --from <draft>` → lint, show diff, write atomically on confirm.

## Context

- Owner: Huddle is not OpenSpec; the native format is the AI-native SDLC playbook; "if OpenSpec is unnecessary, remove it."
- The playbook already names every artifact, location, and heading Huddle needs; a framework adapter would add a second lifecycle, CLI coupling, and fixtures without adding a stage the playbook lacks.

## Consequences

- One layout to template, lint, and eval; zero third-party CLI dependencies.
- Teams on another spec framework still work via `custom` mapping, without Huddle understanding that framework's lifecycle.

## binds

- Artifacts use the AI-native SDLC playbook's own files and headings; `native` is the only format
  huddle implements.
- A repo that lives elsewhere remaps paths through `artifacts.*` in `config.json`, nothing more.

## prevents

- No adapter for, dependency on, or validation against any spec framework (OpenSpec, spec-kit,
  BMAD).
- No framework detection and no third-party `init`: huddle never bootstraps another tool's layout.
