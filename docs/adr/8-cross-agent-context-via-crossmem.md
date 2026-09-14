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
