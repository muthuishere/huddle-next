---
id: adr-13
kind: adr
status: proposed
author: muthuishere
links: {}
---
# ADR 13: Performance Is a Budget — Wall-Clock, Tool Calls, and Tokens, Enforced in CI

- Date: 2026-09-14
- v1: built; deferred: `context-bill` (ADR 19)
- Builds on: ADR 2 (preflight), ADR 11 (slim)

## Status

**proposed** — the decision stands; it is only partly implemented. The `v1:` line above and
**Open questions** name which clauses landed and which did not. The front matter `status:` field
is the status of record.

## Decision

Huddle is getting big (SDLC orchestration, personas, writers, artifact writers, ingest, cross-agent context). It stays fast by treating three costs as budgets with tests, not intentions.

### A. Wall-clock (the binary)

1. **Go binary, no daemon.** Cold start of a static Go CLI is milliseconds; a persistent server (gstack's browser daemon pattern) is unnecessary and adds lifecycle bugs.
2. **One call, labelled output.** Every multi-step thing the agent would otherwise do in several shell rounds is one subcommand (gstack `gstack-skill-start` collapsed 13–18 KB of inline bash into one call). `huddle start` (ADR 14) is the only startup call.
3. **Concurrency inside, not across tool calls:** goroutines for git probes, persona scan, state read, crossmem summary; `errgroup` with per-probe deadlines (git 300 ms, crossmem summary 200 ms) — a slow probe returns `partial: true`, never blocks the huddle.
4. **Nothing networked on the hot path** (ADR 2). Update check: cached file, once per 24 h, 5 s timeout, **skipped in spawned/subagent sessions**; a crashed check never reports "up to date".
5. **Cheap before expensive:** stat/mtime and front-matter-only reads before full parses; substring prefilter before JSON decode on raw events; memory/knowledge bodies never read by the CLI unless asked.
6. **Index cache with mtime invalidation:** `~/.config/.huddle/cache/index.gob` holds resolved persona roster, knowledge index, raw-event summaries keyed by `(path, size, mtime)`. A warm `huddle start` does ~no parsing. Corrupt cache = delete and rebuild, never an error.
7. **Background for non-critical writes:** timeline/usage lines and cache refresh are written after stdout is flushed (or by a detached child), never before the answer.
8. **Budgets (owner's Mac, warm):** `huddle start` p50 ≤ 60 ms, p95 ≤ 150 ms; `persona resolve` ≤ 20 ms for 200 personas; `persona find` ≤ 50 ms over 10k knowledge files. Go benchmarks in CI fail on > 25 % regression.

### B. Tool calls (the agent loop)

9. **Target: first persona response within 3 tool calls** — `huddle start` → (optional) one mode file read → answer. Measured by the ADR 9 e2e harness from the stream-json transcript.
10. **Stale-binary handshake:** `huddle start` prints `HUDDLE_PROTO: 1`; the skill falls back to a degraded minimal flow on mismatch instead of erroring through several rounds.
11. **Errors tell the agent the next action** (`"gate 'plan' needs an owner — run: huddle gate approve plan --as <owner>"`), so failures cost one round, not three.

### C. Tokens (the context window)

12. **Router skill, lazy sections.** `SKILL.md` = front matter + a short router. Everything else is `modes/<mode>.md`, `writers/<writer>.md`, `personas/<id>.md`, listed in an embedded `manifest.json` (title, trigger, bytes) and read only when that mode/writer/persona is active (gstack `sections/manifest.json`, BMAD just-in-time step loading).
13. **Personas enter context as style cards**, not full bodies (`huddle persona style`, ≤ 400 tokens each); full body only on explicit deep-dive.
14. **Catalogs are served, not loaded:** elicitation methods, brainstorm techniques, review lenses live in the binary and are sampled (`huddle methods pick -n 5`) — never read whole (BMAD `pick_methods.py`).
15. **Enforced byte budgets (Go tests):**
    - `SKILL.md` description ≤ 260 bytes; whole `SKILL.md` ≤ 6 KB.
    - `huddle start` output ≤ 2 KB default (`--verbose` for more).
    - Any single mode file ≤ 8 KB; persona style card ≤ 1.6 KB; writer draft context bundle ≤ 12 KB.
    - Ratchet: a budget can go down in a PR, going up needs an explicit override line in the test with a reason.
16. *(Deferred, ADR 19.)* **`huddle context-bill`** prints always-on vs per-mode token cost of the installed skill, per harness — the number reviewed in PRs that touch assets.
17. **Memory in, briefs out:** learnings/decisions injected at start are top-N (3) by recency × relevance; long logs are distilled by a subagent into a brief, not pasted.
18. **Generated, committed skill text with freshness CI:** mode/writer/persona index files are generated from front matter at build time (`go generate`), embedded, and CI fails on drift — no hand-maintained roster XML.

## Context

- Measured today: preflight ~700 ms (network), plus several instruction-read rounds and 7 analytics calls in the 2026-09-13 OpenCode huddle.
- gstack runs 50+ skills and stays usable only because of this exact discipline: one start script, preamble tiers, lazy sections, catalog token caps enforced by tests, context bill, spawned-session skips.
- BMAD's lesson in the other direction: "length and ambiguity are paid on every run"; it removed sharding/manifests and moved to JIT loading with spec token targets (900–1600).
- The owner: "we are doing something very big" — scope grows, so cost must be capped structurally.

## Consequences

- Asset authors work against visible budgets; bloat shows up as a failing test with a number.
- Two representations of each persona (full body + style card) — the card is derived at build/resolve time, not hand-written.
- The e2e harness needs an authenticated runner to measure tool calls (ADR 9 open question).

## Open questions

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §A1-A5 (no daemon; one `huddle start`; concurrent probes with per-probe deadlines and
`partial: true` — `internal/app/cmd_start.go:96`, `:128`, `:159`); §A8 the p50 budget as a test
(`internal/app/core_test.go:359`); §B9-B11 (`HUDDLE_PROTO` at `internal/app/cmd_start.go:77`, `:520`;
next-action error strings at `internal/gates/gates.go:226`, `:334`); §C13 style cards with per-field
budgets (`internal/personas/style.go:43`, `internal/personas/personas_test.go:207`); §C15 the byte
budgets as a CI gate (`internal/install/budget_test.go:12-18`, `.github/workflows/ci.yml:26`);
§C17 top-N learnings, latest-per-key (`internal/app/cmd_start.go:333-359`).

**Not landed:**

- §A6 the `cache/index.gob` mtime-keyed index cache does not exist. Deliberate, and recorded:
  `stress-test.md:46` calls it over-engineering and says measure first. Either amend §A6 or drop it.
- §C12 the embedded `manifest.json` (title, trigger, bytes) is not there, and there are no
  `writers/<writer>.md` or `personas/<id>.md` lazy files — `skills/` holds `SKILL.md` plus
  `modes/` and `stages/` only. Lazy loading works by router text, not by a manifest.
- §C14 served catalogs (`huddle methods pick`) — not implemented; follows ADR 14 §8, which ADR 19
  defers, so §C14 should be marked deferred too.
- §C18 generated-and-committed index files with `go generate` + a freshness CI check — there is no
  `go:generate` directive in the repo. The drift check that does exist
  (`internal/app/skills_sync_test.go`) validates subcommands, not generated indexes.
- §A4's 24 h update check is not implemented.

## binds

- Wall-clock, tool calls and tokens are budgets with tests behind them: `huddle start` p50 ≤ 60 ms /
  p95 ≤ 150 ms warm, `start` output ≤ 2 KB, `SKILL.md` ≤ 6 KB, each lazy file ≤ 8 KB.
- `SKILL.md` stays a router; modes, writers and personas are read only when active.

## prevents

- A limit is never raised to make a change fit — the cause is fixed instead.
- No daemon or persistent server to buy back start time.
- No always-loaded catalog: nothing large is read "just in case".
