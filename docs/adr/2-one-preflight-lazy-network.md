---
id: adr-2
kind: adr
status: approved
author: muthuishere
links: {}
---
# ADR 2: One `huddle preflight` Call; Network Context Is Lazy and Cached

- Date: 2026-09-14
- v1: built as `huddle start`; deferred: §3–4 gh cache (use hooks) (ADR 19)
- Amended by: ADR 14 §10 — the single startup command is `huddle start --json`; `preflight` is its internal step.

## Status

**approved** — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw
decision: `README.md` → Inputs). The front matter `status:` field is the status of record.

## Decision

1. **Startup is one process, one tool call:** `huddle start --json` returns `{global, project, session}` — the union of today's three script outputs — computed with goroutines inside the binary.
2. **Nothing on the startup path touches the network.** `gh pr list` (and any other `gh`/remote call) moves out of preflight.
3. *(Deferred, ADR 19 — use hooks.)* **GitHub context is fetched on demand** via `huddle context --gh`, only when a mode needs it (verification, planning against open PRs, "what's in flight"), and is cached at `~/.config/.huddle/repos/<repo>/cache/gh.json` with `fetchedAt` and a TTL (default 10 min). Preflight reports `gh: {cached: bool, ageSeconds}` so the model knows whether to fetch.
4. *(Deferred.)* **A stale cache may be returned immediately and refreshed in the background** (`--refresh-async` spawns a detached `huddle context --gh --write-cache`), so even the on-demand path rarely blocks.
5. **Local git probes stay eager** (status, log `--since=8h`, branch, HEAD — each ~25 ms), run concurrently.
6. **Budget:** `huddle start` p50 ≤ 60 ms / p95 ≤ 150 ms warm (ADR 13, 19), asserted by a benchmark test (ADR 9). Regressions fail CI, not vibes.

## Context

- 2026-09-13 hyperfine: `session_state.py` 717 ms; `gh pr list` 746 ms alone; local git probes 24–27 ms; the other two scripts 56–94 ms. The network call is the whole problem.
- Three parallel Bash tool calls also cost the agent three tool-result rounds; one call is fewer tokens and one permission prompt.
- The playbook's framing: measure the leading metric (time-to-first-persona-response), not the language.

## Details

- The next real latency after this is **instruction loading** — the model reads `workflow.md`, `activation-routing.xml`, step files and persona files in several rounds. `huddle start --with-assets` can return the resolved routing + selected persona bodies inline, collapsing those reads. Measure before enabling by default.
- `session_state`'s side effect (ensure today's note exists, report `is_resume`) is preserved, but the resume rule becomes the executable one (content ≠ empty template) and `SKILL.md`'s contradictory "always resume if it exists" wording is corrected.

## Consequences

- Expected warm preflight: ~100 ms vs ~700 ms today.
- PR context can be up to TTL old unless explicitly refreshed; personas must say "as of N minutes ago" when citing it.

## Open questions

- Should `huddle context --gh` also pull CI status for the current branch (`gh run list`)? Same cache, same laziness.

## binds

- Startup is one process and one tool call (`huddle start --json`), computed concurrently inside the
  binary.
- Anything remote or slow moves behind an explicit command, caches to disk with a `fetchedAt`, and
  reports its age so the caller can decide whether to refresh.

## prevents

- Nothing on the start path touches the network — no `gh`, no remote, no model call.
- No second startup entry point: `preflight` is an internal step, not a command users learn.
