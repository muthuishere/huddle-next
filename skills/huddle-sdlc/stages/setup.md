# Stage 0: Setup — make the repo agent-ready

**Artifacts:** `CLAUDE.md` (Commands · Conventions · Architecture · Things Claude gets wrong),
`AGENTS.md` (shared content for other harnesses), `.huddle/config.json` (owners, hooks),
harness hook pack drafts. **Gate:** the user approves the guardrails diff.

## Detect

- `huddle guardrails status --json` → missing sections, stale lines, managed blocks of other tools.
- `huddle config show --json` → owners, hooks, protectedPaths, deployPatterns, innerLoop.

## Interview (≤ 7, skip what detect answered)

1. Who owns what? (product, tech-lead, code, service, policy) → `huddle config set owners.<role> '["name"]'`
2. What are the real build / test / lint commands? (verify they run before writing them down)
3. Which conventions matter most? (e.g. money types, error handling, logging)
4. What does the agent keep getting wrong here?
5. Which paths must agents never edit? (generated code, config) → `protectedPaths`
6. What counts as a production command? → `deployPatterns`
7. Which harnesses does the team use? (Claude Code, Codex, Copilot) → decides `AGENTS.md`

## Delegate

- **Scout subagent (read-only):** manifests, Taskfile/Makefile, CI, top-level layout, existing
  `CLAUDE.md`/`AGENTS.md`. Returns ≤ 400 tokens with file:line and *verified* commands.
- **Guardrails writer subagent:** `huddle guardrails new --dry-run` for target + template,
  drafts into the drafts dir, one page, evidence-based. It edits only its own
  `<!-- huddle:context -->` block; never another tool's managed block.

## Room

Deepak (docs clarity) + Shaama (are these the real commands?). Then STOP.

## Hooks (drafts only)

- `huddle hooks export --host claude` → show the draft pack: protected paths, no test edits
  in fix mode, plan drift reminder, production gate, post-edit format/lint (needs
  `formatCommand`/`lintCommand`), inner-loop `permissions.allow` (needs `innerLoop`),
  plus the managed-settings snippet for platform teams.
- The user decides; they run `huddle hooks export --host claude --write` or you do on their
  explicit yes. Never `--force` without asking.
- Integration hooks (Jira, Slack, gh) go in `hooks[]`; test with `huddle hooks test <event> --payload f.json`.

## Gate

1. Show the guardrails diff in full.
2. On the user's yes: `huddle guardrails new --from <draft>` (no CLAUDE.md yet), or edit only
   the `<!-- huddle:context -->` block by hand.
3. `huddle gate request <guardrails-id>` (from `huddle guardrails status --json`) — the user
   (or owner) approves in their terminal. Suggest the commit.

## What good looks like

```markdown
## Commands
- `task test` — unit tests (~20s). `task ci` before pushing.
## Conventions
- Money is always `decimal.Decimal`, never float64.
## Things Claude gets wrong
- Adds mocks for the DB; we use testcontainers — see `internal/db/testutil.go`.
```
