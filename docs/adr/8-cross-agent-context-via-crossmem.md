# ADR 8: Cross-Agent Session Context via crossmem, Imported as a Library

- Status: proposed
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **Huddle can see what every agent did in this folder** — Claude Code, Codex, Devin, Copilot (VS Code + CLI), OpenCode — by using crossmemcli's providers. Huddle does not grow its own session parsers.
2. **Import as a Go library, not a subprocess** — available now: crossmemcli ADR 3 (`docs/adr/3-public-library-api.md`) is **accepted and implemented (2026-09-14)**. Huddle pins `github.com/muthuishere/crossmemcli v0.2.x` and uses:
   - `crossmem.New(crossmem.Options{Config, CurrentSessionIDs, Debug})` — no process-global state.
   - `c.List(ctx, ListOptions{CWD, Limit, IncludeSubagents})`, `c.Transcript(ctx, ref)` (typed `Event{Role, Name, Content, Time}`), `c.Load(ctx, ref, LoadFull)`, `c.Guardrails(root)`, `c.Scan()`.
   - `pkg/skillinstall` (`Install(fsys, name, targets)`, `DefaultTargets`) for ADR 1's `huddle install --skills`.
3. **No shell-out fallback needed.** The library is linked into the binary; the PATH version of crossmem is irrelevant to Huddle.
4. **Lazy, like `gh` (ADR 2):** `huddle start` returns only a cheap count/recency summary if the library is linked (`agents: {claude: 2, opencode: 1, newestAgo: "1 day"}`); full `huddle context --agents [--provider] [--limit]` is on demand — e.g. "resume", "what did Codex try here", research and verification modes.
5. **What Huddle uses:** `list` (provider, ago, title, firstQuestion, lastQuestion) to let personas cite prior attempts; `load` for a chosen session; `guardrails` for the repo's instruction files. The current session is excluded (crossmem's env-based detection).
6. **Single redaction choke point:** any transcript text Huddle passes to the model goes through one `redact` package (port of AI Engineer Coach's `redact-secrets.ts` patterns) and is labelled untrusted data (ADR 5). Credential files, `*.env`, `vault/` are never read — crossmem's existing rule.
7. **Transcripts are context, not memory.** Huddle never writes transcript content into `huddle-state.json` or persona memories automatically; a persona may *propose* a memory (ADR 4) that the user confirms.

## Context

- This session's own resume exposed the gap: the huddle that decided the Go CLI ran in **OpenCode**, and was only recovered by hand-querying `~/.local/share/opencode/opencode.db`. Root cause: the globally installed crossmem is 0.1.4, which predates the OpenCode provider (added in 0.1.5; npm is at 0.1.10). The repo build finds all three sessions.
- The owner works across Claude Code, Codex, Devin, Copilot, and OpenCode; a huddle that only knows its own harness repeats settled debates.
- crossmemcli is the owner's, CGO-free (modernc SQLite), same packaging pattern as ADR 1 — linking it costs no build complexity.

## Details — crossmem fixes (shipped in v0.2.0)

- Folder filter no longer loses sessions to `LIMIT` (OpenCode, Devin, Copilot CLI): rows stream newest-first and match in Go until `limit`.
- OpenCode subagent sessions hidden by default, exposed as `Session.Children` / `IncludeSubagents`.
- OpenCode fixtures added; relocated stores (`$CLAUDE_CONFIG_DIR=~/.claude-cys`) now `Load` correctly.

## Consequences

- Huddle's binary grows by the SQLite driver (~several MB); acceptable for a CLI.
- Version coupling: Huddle pins a crossmem module version; provider fixes arrive via `go get -u` + a Huddle release.
- Owner action: `npm i -g @muthuishere/crossmem@latest` so the standalone crossmem-loader skill also sees OpenCode.

## Open questions

### Implementation status, 2026-10-10 (event `20261010T045742.452Z-01d288`)

Status stays `proposed`: the decision is **partially implemented**. Verified against the
tree, not against this ADR's own claims.

**Landed:** §4 lazy — `huddle start` reports only a summary and `huddle context --agents` is
on demand (`internal/app/cmd_context.go:13`); §6 the single redaction choke point
(`internal/redact/redact.go:21`, `internal/redact/redact_test.go`); §7 transcripts are never written
into state or memories.

**Not landed — and §3 is contradicted by the code:**

- §2 the library import exists but is behind a build tag: `internal/app/context_crossmem.go:1` is
  `//go:build crossmem`, and it is only reachable with `go build -tags crossmem` plus a gitignored
  `go.work` (see the repo `CLAUDE.md`).
- §3 "No shell-out fallback needed" — the **default** build is the shell-out:
  `internal/app/context_cli.go:1-2` is `//go:build !crossmem` and execs the `crossmem` CLI. So every
  released binary (`.goreleaser.yaml` passes no tags) takes the path this ADR says is unnecessary,
  and cross-agent context silently degrades to "hint only" when the CLI is absent.
- §5 `guardrails` and `load` of a chosen session are not exposed by `huddle context`.

Decide one way or the other: either make `crossmem` a plain dependency and build it into the
release, or amend §2/§3 to say the CLI is the shipped path and the library is opt-in.
