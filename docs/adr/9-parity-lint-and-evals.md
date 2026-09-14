# ADR 9: Parity Fixtures, Asset Lint, and Behaviour Evals

- Status: proposed
- Date: 2026-09-14
- v1: built; deferred: touchfile selection, LLM judge (ADR 19)

## Decision

Three test layers, cheapest first:

1. **Parity (deterministic, `go test`).** Before a Python script is deleted, capture its JSON stdout for a fixture matrix into `testdata/parity/<command>/<case>.json` and assert the Go subcommand matches (ignoring timestamps/abs paths via normalisers). Matrix at minimum: git repo, non-git folder, custom bootstrap identity, legacy root mid-migration, corrupt JSON, three historical state shapes, sibling branches, synced personas global/repo/memory-only, absent dirs. Fixtures are built in `t.TempDir()` with a temp `$HOME`, as crossmem's tests do.
2. **Asset lint (deterministic, `go test` + `huddle persona lint`).** Every embedded file is validated: all 21+ personas against the ADR 4 schema; the generated roster/routing index (ADR 13 §18) ⇔ persona and mode front matter, no hand-kept XML; step files reference only real `huddle` subcommands and flags (parsed from the command registry — no stale `python3 scripts/...` survives); `huddle-state` examples validate against the ADR 6 JSON Schema. Doc contradictions found by the inventory (writer both required and forbidden, per-round writes vs. none, resume rule) become failing lint cases until fixed.
3. **Behaviour evals (LLM, scheduled + on change).** A suite of 20–50 real huddle tasks in `evals/` — each a starting prompt, fixture repo/state, and assertions over the transcript + written files (e.g. "personas stop and wait after one round", "no state write on a normal round", "decision event written on decision", "Deepak does not hijack the first turn", "repo persona pending approval is not used"). Runs headless in CI on changes to `assets/SKILL.md`, `references/**`, personas, and weekly. **Every huddle misbehaviour found in real use becomes a new eval case.**

Plus:

4. **Benchmarks:** `BenchmarkStart` enforces ADR 13's budget (p50 ≤ 60 ms); CI fails on >25% regression.
5. **Release gate:** `task ci` = test + vet + lint + build + `--version` + goreleaser snapshot, mirroring crossmem.

## Context

- Existing coverage: `e2e/run.py` smoke-tests three preflight scripts, synced personas, and migration. Untested: `config_helper`, `repo_context`, `huddle_writer`, `project_state check/read/write`, installers, gh parsing, branch collisions, concurrency, persona merge, routing consistency. No CI.
- Playbook: configuration that steers the agent (CLAUDE.md, skills, hooks) is regression-tested like code with a 20–50 task eval suite that grows with every incident.
- AI Engineer Coach: inline `# Tests` in each markdown rule, executed by the test runner. Persona files keep the built-in style (ADR 4), so persona behaviour tests live as eval cases in `evals/`, not inside persona files.

## Consequences

- Python can be deleted script-by-script with evidence, not hope.
- Eval runs cost tokens; keep the suite small, run on relevant path changes plus weekly.

## Open questions

- Eval harness: Claude Agent SDK / `claude -p` with an API key in CI. **No authenticated runner ⇒ the eval result is `unverified`, never PASS; the gate asks the owner (ADR 19 row 12).**
