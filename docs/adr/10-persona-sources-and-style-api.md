---
id: adr-10
kind: adr
status: approved
author: muthuishere
links: {}
---
# ADR 10: Personas from Any Data — One Ingest Route, Import, Proposals, and a Style API

- Date: 2026-09-14
- v1: built; deferred: §5 discover/propose, §6 analyze (ADR 19)

## Status

**approved** — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw
decision: `README.md` → Inputs). The front matter `status:` field is the status of record.

## Decision

1. **Huddle does not own data sources.** No built-in git, GitHub, Jira, mail, or chat adapters. Whoever has the data — the user, an agent using `gh` / `apl` / a Jira CLI / an MCP server, a cron job, another CLI — brings it through **one documented route**. Huddle's job is shaping that data into personas and serving them back.
2. **The route: `huddle persona ingest`.**

   ```sh
   <anything> | huddle persona ingest <id> [--scope global|repo] [--kind evidence|definition|knowledge] [--label <free text>]
   huddle persona ingest <id> --file dump.ndjson|notes.md|export.json
   ```

   Auto-detected inputs (override with `--format`):
   - **Evidence NDJSON** — one record per line; only `text` is required:
     ```json
     {"text":"…","author":"alice","source":"github","kind":"review-comment","url":"…","time":"2026-08-02T10:14:00Z","path":"internal/pay/charge.go","tags":["payments"]}
     ```
   - **Markdown / plain text** — one evidence record per file, or a knowledge file with `--kind knowledge`.
   - **A persona-like definition** from anywhere (`.claude/agents/*.md`, BMAD agent files, another tool's persona) with `--kind definition` → import flow (decision 4).

   `huddle persona ingest --schema` prints the JSON Schema so any producer can target it. `source` / `--label` are free text; Huddle never branches on them.
3. **The skill says "use whatever you have".** For "make a persona from Alice's reviews", "scan our PM board", "use my mail with the client", the skill tells the agent to gather with **any tool available in that harness** and pipe into `huddle persona ingest`. Huddle names no required tool.
4. **Import any persona, rewritten into our style.** `huddle persona import <file|url|-> --scope global|repo [--id]`: the skill has the model rewrite it into the built-in persona style (ADR 4), adds Common Disagreements against the current roster, shows it in full, writes on confirm, records `importedFrom: {ref, importedAt}`; reviewed before write.
5. *(Deferred, ADR 19.)* **Discover and propose work on ingested data, not services.**
   - `huddle persona discover "<query>"` searches local material: resolved personas, knowledge, ingested evidence, cache. To discover on GitHub or anywhere else, the agent searches with its own tools and ingests hits as `--kind definition`.
   - `huddle persona propose [--evidence <ids|all>] [--repo .] --json` computes candidates from ingested evidence plus offline repo facts (languages, top-level areas, manifests, `CODEOWNERS` if present): **role personas** ("payments reviewer") and **people-modelled personas** (most active voices per area in the evidence), each with the records that suggested it. The user picks; each goes through ADR 4.
6. *(Deferred, ADR 19.)* **Deterministic analysis, model-written prose.** `huddle persona analyze <id>` computes checkable facts over evidence (message length distribution, blocking vs nit ratio, most-referenced paths/topics, recurring phrases, approve-fast vs push-back topics). The skill gives these + sampled records to the model to write the persona in built-in style plus `style.md` / `examples/`. **The CLI never calls an LLM.**
7. **The style API — find and pass a style anywhere:**
   - `huddle persona find "<query>" [--tags] [--layer] --json` — ranked search across personas and knowledge.
   - `huddle persona style <id> [--for review|spec|intent|pr|email|standup] [--json|--md] [--max-tokens N]` — prompt-ready card: `primaryLens`, `communicationStyle`, `principles`, Signature Phrases, 2–3 real examples, filtered by task.
   - `huddle persona brief <id>[,<id>…] --task "<text>" [--files …]` — full fragment for handing a task to any agent: style card(s) + relevant memories + repo guardrails. `huddle persona brief alice-reviewer --task "review this diff" | claude`, or codex/opencode, a subagent prompt, CI review.
   - All read the resolved roster (ADR 3).
8. **One global root (ADR 6).** Every ingested record is redacted (ADR 8 `redact`) before it touches disk. Raw evidence `~/.config/.huddle/evidence/<id>/*.ndjson` (0600, never committed, never in `<repo>/.huddle/`); caches `~/.config/.huddle/cache/`. Only distilled, reviewed, redacted persona files are written to `~/.config/.huddle/personas/<id>/` or `<repo>/.huddle/personas/<id>/`; for `repo` scope the review screen says the text will be committed.
9. **Real-person personas are marked:** `basedOn: {name, handles, labels, gatheredAt}`; style output presents a style modelled on that person, never the person speaking. Third-party quotes are stripped at distillation.

## Context

- Owner: data sources are not Huddle's point — "git or jira or apl or whatever, let them use whatever", with one specific route in. Every harness already has its own access layer, and those change faster than Huddle should.
- One schema-documented route keeps Huddle small, offline, fixture-testable, and producer-agnostic — the stance the synced-persona directories already had.
- Mirrors crossmem: deterministic plumbing in the CLI, judgment in the agent, user review before write.

## Consequences

- Nothing network-bound in `huddle persona *`.
- Grounded persona quality depends on what gets ingested; `propose` / `analyze` show their evidence so thin data is visible.
- `persona brief` makes personas usable outside a huddle — reviews, specs, intents, email tone.

## Open questions

- Refresh: `ingest --replace` vs append-only evidence with `--since` windows in `analyze`?

## binds

- One documented ingest route (`huddle persona ingest`) for every possible persona source, and a
  CLI-served style/brief API on the way out.
- Whoever holds the data brings it; huddle's job is shaping it into personas.

## prevents

- No built-in git, GitHub, Jira, mail or chat adapter inside huddle.
- No second, undocumented way for data to become a persona.
