# ADR 7: Huddle Outputs Join the Committed SDLC Artifact Chain

- Status: proposed
- Date: 2026-09-14
- v1: built via `<noun> new` (no `export`) (ADR 19)
- Amended by: ADR 12 — intent/spec/plan paths follow the playbook (`intent/<slug>/{intent,spec,plan}.md`) or a `custom` mapping. The `docs/intent|specs|plans` paths below are superseded. ADR export is unchanged.

## Decision

1. **Huddle maps its modes onto the AI-native SDLC stages** and, on explicit ask or wrap-up, **exports a committable artifact** for that stage into the repo:

   | Huddle mode | Playbook stage | Artifact | Default path |
   |---|---|---|---|
   | discussion / research on a problem | Plan | `intent.md` | `docs/intent/<slug>.md` |
   | spec-review / "get me a spec" | Design | `spec.md` | `docs/specs/<slug>.md` |
   | planning | Build (plan mode) | `plan.md` | `docs/plans/<slug>.md` |
   | any accepted decision | cross-cutting | ADR | `docs/adr/<n>-<slug>.md` |
   | verification | Test / Deploy review | `verify` record against spec + plan (ADR 19) | log + `huddle verify status` |

2. **Private working state stays private.** `~/.config/.../huddle-state.json`, raw events, and daily notes remain outside the repo (ADR 6). Export is a deliberate step: `huddle <noun> new --from-event <id>` (ADR 18), shown for review, written only on confirm. Huddle never commits.
3. **ADRs are the decision artifact.** A raw `decision` event already holds `topic`, `content`, `rationale`, `personas`, `by`, `rejected`, `open`; `huddle adr new --from-event` renders it into this repo's ADR format (Decision → Context → Details → Consequences → Open questions), auto-numbering from existing files, `Status: proposed`. Accepting an ADR in a later huddle records a new event referencing it and flips the status line.
4. **Traceability both ways:** the exported artifact footer cites the huddle event id and date; the event records the exported path and (once known) commit SHA.
5. **Per-repo paths are configurable** in `config.json` under `artifacts.*` (ADR 19), detected on first export from existing folders (`docs/adr`, `adr/`, `docs/decisions`).
6. **Humans own the gates.** Personas can recommend a stage transition ("this is ready to become a spec"); only the user approves it — consistent with Huddle's "personas perspective, user decides" rule and the playbook's "human judgement stays above it".

## Context

- Playbook core principle: *commit after every stage* — `intent.md → spec.md → plan.md → diff → review`, each human-readable and machine-actionable, the chain itself being the audit trail. Huddle today produces the thinking behind those stages but leaves it in `~/.config`, invisible to teammates, reviewers, CI review (`REVIEW.md`), and other agents.
- These very ADRs were produced from a huddle decision event by hand — the export command automates what just happened.
- Huddle already has spec capture (Elango) and project documentation (Deepak); this ADR gives them a stable, repo-visible destination.

## Consequences

- A reviewer (human or `claude-code-action`) can check a diff against the huddle-produced `plan.md` and ADRs.
- Elango's synthesis gains render targets; Deepak's project doc flow can link the ADR index.
- Risk of doc sprawl — mitigated by export being explicit and deduplicated by topic slug (re-export updates the same file).

## Open questions

- Resolved (ADR 19): verification is `huddle verify record|status`.

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §1 every mode's artifact is a CLI noun on the playbook path
(`internal/artifacts/kinds.go:39-68`); §3 ADR rendering with auto-numbering from existing files
(`internal/artifacts/store.go:272-303` `nextNumber`) and `Status: proposed` in the template
(`assets/templates/adr.md`); §5 per-repo paths configurable under `artifacts.*`
(`internal/artifacts/kinds.go:73-82` `Get`); §6 humans own the gates
(`internal/gates/gates.go:264-300`).

**Not landed:**

- §2 and §3 `huddle <noun> new --from-event <id>` / `huddle adr new --from-event` — no `--from-event`
  flag exists (`internal/app/cmd_artifacts.go:56` is the whole `new` surface: slug/title, `--dry-run`,
  `--author`). A raw `decision` event therefore cannot be rendered into an ADR by the CLI; the model
  has to retype it, which is exactly what §3 set out to remove.
- §4's two-way traceability is half-wired: the event records the artifact path and hash
  (`internal/artifacts/record.go:38`), but nothing writes the event id into the artifact footer, and
  no commit SHA is ever recorded.
