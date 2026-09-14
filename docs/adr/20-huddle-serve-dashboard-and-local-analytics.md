# ADR 20: `huddle serve` — Local Dashboard on :16211 and Local-Only Usage Analytics

- Status: proposed
- Date: 2026-09-14
- v1: built
- Amends: ADR 18 §7 ("no web dashboard in core"), ADR 11 §2 (usage lines)

## Decision

1. **`huddle serve [--port 16211] [--open] [--repo <path>]`** starts a dashboard on `127.0.0.1:16211` (loopback only; never `0.0.0.0`). Port 16211 is the default; `--port 0` picks a free one and prints it. One stdlib `net/http` server, UI embedded in the binary (no CDN, no build step, works offline).
2. **What people see** (read from the same packages the CLI uses — no second source of truth):
   - **Overview:** repo, branch, SDLC table per change (intent/spec/plan status, gates, verdict, stale), next action, open spikes, missing guardrails/REVIEW.md/bands/evals.
   - **Artifacts:** every noun with status, links, lint result; rendered markdown view.
   - **Gates:** pending/approved/rejected/stale, who approved, overrides with reasons.
   - **Timeline:** event log with filters (type, door, branch, date), decisions superseded shown struck through.
   - **Personas:** resolved roster with layer provenance, style cards, knowledge/memory index.
   - **Hooks:** configured hooks by scope, last runs, failures, `REPO_HOOKS_CHANGED`.
   - **Analytics:** decision 4.
   - **Repos:** every repo with state under `~/.config/.huddle/repos/`, switchable.
3. **What people can do in it** — safe, local, auditable actions only:
   - Create an artifact from a template (`<noun> new`), run lint, append a decision/learning/question to the log, copy a persona style card or brief, run `hooks test` with a sample payload, download a markdown report of status.
   - **Gates are not approved in the browser.** The gate view shows the exact terminal command. A localhost HTTP endpoint is reachable by any agent on the machine, so it cannot prove a human (ADR 19 row 7).
   - Mutating requests require a per-process token (printed in the terminal and embedded in the served page), `Origin` = the served origin, and `Content-Type: application/json` — blocks cross-site requests from other browser tabs. Every mutation is logged with `by: "serve"`.
4. **Local-only analytics** — no network, ever:
   - Every CLI invocation appends one line to `~/.config/.huddle/usage.jsonl`: `{ts, cmd, sub, ms, exit, agent (bool, via agent-session env detection), skill (from --skill or HUDDLE_SKILL), repo, version}` — no arguments, no text, no paths beyond repo name. Written after output is flushed; failure to write is ignored. Opt out: `config set usage false` or `HUDDLE_NO_USAGE=1`. Rotates at 10 MB.
   - Skill routers pass `--skill <name>` on `huddle start`, so skill usage is counted with zero extra tool calls.
   - **Dashboard analytics:** command counts and p50/p95 latency per command (performance budgets shown against actuals); human vs agent share; skill invocations per skill; persona style/brief usage; SDLC flow metrics from the log (intent→spec→plan→verdict lead times, spec rework after plan, first-pass PASS rate, stale verdict count, gate wait time, overrides); hook run/failure counts; errors by command.
   - `huddle stats [--json] [--since 30d]` prints the same numbers in the terminal (CLI-first, ADR 18).
5. **Performance:** `serve` is never on the agent hot path. Pages load from JSON APIs computed on request with a 2 s in-memory cache; the event log is streamed, not loaded whole; usage aggregation over 100k lines ≤ 200 ms.

## Context

- Owner: "make huddle serve show all stuff on a unique port like 16211, do something within, so people know stuff — and all analytics about our CLI and skill."
- The CLI already makes everything visible (ADR 18); a dashboard makes the same state browsable for a whole team member at a glance, especially flow metrics the playbook asks teams to watch.
- ADR 11 already allowed "one local line per command it already runs"; this defines it and keeps the no-telemetry promise.

## Consequences

- Adds `internal/usage`, `internal/serve`, `web/` (embedded), `cmd_serve.go`, `cmd_stats.go`.
- The usage file is personal data on the user's machine; `huddle stats --json` is the export, nothing is sent.
