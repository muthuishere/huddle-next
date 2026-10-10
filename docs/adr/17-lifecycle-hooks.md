# ADR 17: Lifecycle Hooks for the Entire Process — Bring Your Own gh, Jira, Slack, Anything

- Status: proposed
- Date: 2026-09-14
- v1: built (hooks in `config.json`, no trust) (ADR 19)
- Builds on: ADR 10 (ingest route), 14 (event log), 16 (orchestrator)

## Decision

1. **Every lifecycle event Huddle emits can run user hooks.** Huddle ships no GitHub, Jira, Linear, Slack, or mail integration. Teams that use them wire their own commands into the process; Huddle calls them at the right moment and passes the event.
2. **Events** (all emitted by `huddle` CLI commands, so they fire deterministically — never "if the model remembers"):

   | Group | Events |
   |---|---|
   | Session | `session.start`, `session.wrap` |
   | Stage | `stage.enter`, `stage.complete` (payload: stage, change id) |
   | Artifacts | `artifact.draft`, `artifact.written` (intent, spec, plan, adr, spike, guardrails, review-policy, eval, bands, skill, hooks) |
   | Gates | `gate.request`, `gate.approved`, `gate.rejected`; `door.one-way` |
   | Log | `decision.logged`, `decision.superseded`, `action.created`, `learning.logged` |
   | Build | `build.task.dispatched`, `build.task.done`, `build.task.failed` |
   | Verify | `verdict.issued` (PASS/CONCERNS/FAIL + fingerprint), `verdict.stale` |
   | Personas / teams | `persona.ingested`, `persona.written`, `team.started`, `team.saved` |
   | Spikes | `spike.started`, `spike.timebox.expired`, `spike.closed` (ADR 18) |

3. **Where hooks live: `hooks[]` in `config.json`, two levels only** (ADR 19): `~/.config/.huddle/config.json` (global) and `<repo>/.huddle/config.json` (repo, committed). Both run, global then repo. Scripts live anywhere (e.g. `<repo>/.huddle/hooks/*`).

   ```json
   {"hooks": [
     {"on": "artifact.written", "match": {"kind": "intent"}, "run": ".huddle/hooks/jira-create-epic.sh", "timeout": "20s"},
     {"on": "gate.request", "match": {"stage": "deploy"}, "run": ".huddle/hooks/pr-approved.sh", "blocking": true},
     {"on": "stage.enter", "match": {"stage": "plan"}, "run": ".huddle/hooks/pull-linear-issue.sh"},
     {"on": "verdict.issued", "run": ".huddle/hooks/slack-notify.sh", "async": true}
   ]}
   ```

4. **Contract (stdin JSON in, optional stdout JSON out):**
   - In: `{event, ts, repo, branch, change, stage, artifact:{kind,path}, decision?, verdict?, huddleEvent}` plus env `HUDDLE_EVENT`, `HUDDLE_REPO_ROOT`, `HUDDLE_CHANGE`.
   - Out (all optional):
     ```json
     {"decision":"allow|deny|ask","reason":"…",
      "context":"text the agent should see",
      "ingest":[{"text":"…","source":"jira","url":"…"}],
      "links":[{"system":"jira","id":"PAY-142","url":"…","artifact":"intent"}],
      "log":[{"type":"action","text":"…"}]}
     ```
   - `links` are recorded in the event log and artifact front matter — this is how the playbook's **"one source of truth per artifact, others hold a link"** rule is implemented for Jira/Linear/GitHub Issues.
   - `ingest` goes through the ADR 10 route; `context` is shown to the agent as untrusted data (ADR 5).
5. **Execution rules:**
   - Non-blocking by default; only `blocking: true` hooks may `deny`/`ask`, and only on `gate.*`, `stage.complete`, `build.task.dispatched`, `artifact.draft` (before write).
   - Timeouts (default 10 s); a failing or timing-out hook is reported, never crashes the flow; a blocking hook that denies, fails, or times out keeps the gate **closed** until a human runs `huddle gate approve --override --reason` (logged, ADR 19 row 8).
   - `async: true` hooks run detached after the CLI answers (ADR 13 — no hot-path latency).
   - Hooks inherit the user's environment; Huddle never stores, prints, or logs secrets, and redacts hook output before it reaches the agent (ADR 8 redact package).
6. **No trust hashing** (ADR 19): hooks are the user's own config, like any build script in the repo. `huddle start` notes when repo `hooks[]` changed since last run (notice only, no approval).
7. *(Deferred.)* **Agent hooks (optional):** a hook may be `prompt:` instead of `run:` — a short instruction the agent executes with its own tools (e.g. an MCP Jira server) at that event. Huddle surfaces it at the event; it never executes it itself.
8. **Harness hooks interop:** `huddle hooks export --host claude` drafts a pack for the user to adopt (ADR 19 row 9): protected paths, no test-file edits during fix tasks, plan↔diff sync, production gate (`huddle gate status`), post-edit format/lint, inner-loop `permissions.allow`, managed-settings snippet (deny `.env*`/`secrets/**`, `disableBypassPermissionsMode`); Huddle never edits harness settings without review.
9. **CLI:** `huddle hooks list|test <event> [--payload file]|export --host claude`.

## Context

- Owner: "support hooks for the entire process — if they have gh, Jira and all, they can put it somewhere and we should allow it."
- ADR 10 already refuses to own data sources; hooks are the matching outbound + inbound extension point at every stage.
- The AI-native SDLC playbook uses hooks as deterministic gates (production gate, protected paths) and asks that legacy tools hold links to repo artifacts rather than duplicate them.
- Mirrors Claude Code's hook model (JSON in/out, block/allow) so users already know the shape, but harness-neutral.

## Consequences

- Any team's toolchain plugs in without a Huddle release; integrations live in the repo, reviewed like code.
- Gates can depend on external state (PR approvals, ticket status) without Huddle knowing those systems.
- Hook failures and latency are contained by timeouts, async, and ask-on-failure.

## Open questions

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §1; §3 `hooks[]` at two config levels, global then repo
(`internal/hooks/hooks.go:65` `Fire`); §4 the stdin/stdout contract including `ingest`, `links` and
`log` (`internal/hooks/hooks.go:40-64`); §5 non-blocking by default, timeouts, escalation
(`internal/hooks/hooks.go:328`), detached `async` (`:347`) and the deny-keeps-the-gate-closed rule
with a logged `--override --reason` (`internal/gates/gates.go:300`, `:333-338`); §6 no trust hashing,
with a change notice instead (`internal/app/cmd_start.go:256`); §8 `hooks export --host claude`
(`internal/hooks/export.go`); §9 the CLI, plus a `fire` verb (`internal/app/cmd_hooks.go:17`).

**Not landed — §2's premise is the gap.** §2 says every listed event is "emitted by `huddle` CLI
commands, so they fire deterministically — never 'if the model remembers'". Only five actually fire:
`gate.request|approved|rejected|override` (`internal/gates/gates.go:324`, `:378`) and
`artifact.written` (`internal/artifacts/record.go:47`). `session.start|wrap`, `stage.enter|complete`,
`artifact.draft`, `decision.logged`, `decision.superseded`, `action.created`, `learning.logged`,
`build.task.*`, `verdict.issued`, `verdict.stale`, `spike.*`, `persona.*` and `team.*` are never
fired by any command — `persona.written` / `persona.ingested` go to the event log only
(`internal/app/cmd_persona.go:403`, `:465`). They can only be triggered by hand with
`huddle hooks fire <event>`.

Two consequences:

- §5's blocking allow-list names `stage.complete`, `build.task.dispatched` and `artifact.draft`, none
  of which exist as emitted events, so blocking is in practice only available on `gate.*`.
- `stress-test.md:35` records the "deny after `artifact.written`" edge as **Closed** because blocking
  is restricted to `artifact.draft`. Since `artifact.draft` never fires, there is no pre-write deny
  path at all; that edge should go back to Open until it does.

`stress-test.md:45` already proposes the fix: ship `stage.*`, `artifact.*`, `gate.*` and
`verdict.issued` first.
