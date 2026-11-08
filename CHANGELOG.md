# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

## [0.1.0] — 2026-11-01

Tagged and published: <https://github.com/muthuishere/huddle-next/releases/tag/v0.1.0>.

**What that release actually delivers**, because one tag was meant to drive every channel and
does not yet:

- **Live.** The GitHub Release carries darwin, linux and windows binaries on amd64 and arm64,
  plus `.deb` and `.rpm` for both linux arches and `checksums.txt` — 11 assets. `go get
  github.com/muthuishere/huddle-next@v0.1.0` resolves through proxy.golang.org. The repo is
  public under MIT.
- **Not live: npm.** The `npm publish` workflow for this tag failed — `npm error code E404` on
  `PUT /@muthuishere%2fhuddle-darwin-arm64`, because npm trusted publishing has not been
  bootstrapped per package (`docs/packaging.md`). No `@muthuishere/huddle` exists on the
  registry.
- **Not live: Homebrew and winget.** Both are wired in `.goreleaser.yaml` and skipped for want
  of `HOMEBREW_TAP_GITHUB_TOKEN` and `WINGET_GITHUB_TOKEN`.
- **CI is not green on every platform.** lint, ubuntu-latest, macos-latest and the goreleaser
  build pass; the windows-latest test job fails. See "Known gaps" below.

First release of the Go CLI and the skill family. Scope is
[ADR 19](docs/adr/19-v1-scope-and-value-gate.md); anything an ADR describes but ADR 19 defers is
not in this release.

### Added

- **`huddle`, one CGO-free Go binary** (stdlib only) with the embedded skill family — `huddle`,
  `huddle-sdlc`, `huddle-personas` — installed by `huddle install --skills` (ADR 1).
- **`huddle start`** — the agent's single startup call: repo, branch, open huddle, computed active
  decisions, top learnings, cache ages, suggested mode. No network, no model, p50 under 60 ms, and
  output capped at 2 KB (ADR 2, 13).
- **Playbook artifacts as CLI nouns** — `intent`, `spec`, `plan`, `adr`, `spike`, `guardrails`,
  `review-policy`, `eval`, `bands`, `skill`, `lessons`, each with `new | list | show | status |
  lint`, templates, and a structural lint. Writers never overwrite and never touch another tool's
  managed block (ADR 12, 18).
- **Human gates with separation of duties** — `huddle gate request|approve|reject|status`. An
  approver may not be the artifact's writer, and an agent session cannot approve: the command
  refuses when `HUDDLE_AGENT=1` or when stdin is not a terminal (ADR 16).
- **`huddle sdlc status|next`** — where each change sits across setup → intent → spec → plan →
  build → test → deploy review → maintain, and the next action.
- **Append-only event log** — `huddle log append|list`, `huddle timeline`, typed events with
  `door: one-way|two-way`, computed active decisions, typed learnings, and `supersede` / `redact`
  lifecycle events (ADR 14 §1).
- **Personas in three layers** — builtin < `~/.config/.huddle/personas` < `<repo>/.huddle/personas`,
  merged in Go, with `persona list|resolve|new|lint|import|ingest|style|brief` (ADR 3, 4, 10).
- **Lifecycle hooks** — `hooks[]` in global or repo `config.json`, fired by the CLI so they do not
  depend on a model remembering; `huddle hooks list|test|export`, including a Claude Code hook pack
  (protected paths, plan↔diff sync, a production gate) (ADR 17).
- **Verdicts bound to the tree** — `huddle verify record|status` stamps `PASS | CONCERNS | FAIL`
  with a tree fingerprint and marks a result stale when the tree moves (ADR 14 §6).
- **`huddle bands check`** — deterministic mean/σ tiering (1σ log, 2σ diagnose, 3σ draft an intent)
  over a user-supplied series; dismissals require `--reason` (ADR 16).
- **`huddle serve`** — a loopback-only dashboard on `127.0.0.1:16211`, UI embedded in the binary,
  reading the same packages the CLI uses; plus local-only usage analytics behind `huddle stats`
  (ADR 20).
- **`huddle eval ci-draft`** and `huddle ci draft triage|spec` — drafted CI workflows the user
  commits themselves; an unauthenticated runner yields `unverified`, never `PASS`.
- **Cross-agent context** — `huddle context --agents` lists earlier Claude Code, Codex, Devin,
  Copilot and OpenCode sessions through crossmem. Released binaries shell out to the `crossmem`
  CLI; the library path is behind `-tags crossmem` (ADR 8).
- **`huddle migrate`** — copies state out of the old Python skill's
  `~/.config/muthuishere-agent-skills` root, never overwriting (ADR 6).
- **`huddle doctor`** — environment and asset health.
- **Distribution** — GoReleaser builds darwin and linux on amd64 + arm64 and windows on amd64 +
  arm64; a `v*` tag publishes `@muthuishere/huddle` plus one platform package per target with npm
  OIDC provenance, a Homebrew cask, `.deb` and `.rpm` release assets, and a winget manifest PR.
- **Repository hygiene** — MIT `LICENSE`, `THIRD-PARTY-LICENSES.md`, `SECURITY.md`, this changelog,
  `docs/packaging.md`, and this repo's own `intent/v1-readiness/`, `REVIEW.md`, `bands.yaml` and
  `evals/` — huddle runs its own SDLC.

### Changed

- The project moved to `github.com/muthuishere/huddle-next`; `muthuishere/huddle` is frozen and is
  no longer the home of this code.
- Installing huddle now installs the skills on every channel. `huddle start` syncs the embedded
  skills when they are missing or when huddle's own copy is stale, which covers apt, dnf, winget,
  `go install` and a plain tarball — none of which can run a post-install hook. Opt out with
  `HUDDLE_NO_SKILL_SYNC=1` or `"skillSync": false`.
- An upgrade keeps memories: the sync replaces a skill directory huddle owns, removes an old
  `install.sh` symlink without touching the repo behind it, and never touches state under the
  config roots. A skill directory huddle does not own is reported and left alone.
- Every ADR now carries front matter and the `Status` / `binds` / `prevents` sections the `adr`
  lint requires, and the ADR status vocabulary is the CLI's `draft | proposed | approved |
  superseded` — `accepted` is gone, because `approved` is what the gate writes and what
  `huddle sdlc status` reads.

### Removed

- No Python at runtime: the eight helper scripts of the previous skill are subcommands, and
  `install.sh` / `install.cmd` symlink installs are retired (ADR 1).
- Per-step analytics shell calls; the binary records one local line per command it already runs
  (ADR 11).

### Security

- The CLI never calls a model and makes no network call on the `huddle start` path.
- No telemetry. Usage data stays on the machine; nothing leaves unless a hook the user configured
  sends it.
- Every ingested record and every hook's output passes one redaction step before it is stored or
  shown to an agent.
- `huddle serve` binds the loopback interface only and checks the `Host` header; it cannot approve
  a gate.
- Agent sessions cannot approve gates, and huddle never pushes, merges, deploys or commits.

### Known gaps in 0.1.0

- **The gate records approvals; it does not enforce them.** Nothing in the `artifacts` package
  consults `internal/gates`, so `status: approved` can be written through `Create` / `Render` /
  `UpdateFrontMatter` with no approval behind it. `gate approve` keeps separation of duties and
  still refuses an agent session, so the *gate* is sound — it is simply not on the write path.
- **The windows-latest test job fails.** Two tests in `internal/specframework`:
  `TestChangesStubbed` (a fixture that interpolates the repo root into a JSON string literal,
  where Windows backslashes are JSON escapes) and
  `TestRelAcceptsSymlinkedRootAndStillRejectsEscapes` (a rooted path with no drive letter is not
  `filepath.IsAbs` on Windows, so it took the relative branch and was echoed back). Windows
  binaries ship regardless.
- **An unfilled template lints clean.** `artifacts.Lint` has no emptiness check, so an untouched
  ADR template reports no findings. Pinned by `artifacts/example_test.go` so it cannot change
  silently.
- `huddle <noun> show --graph` (ADR 18 §B10) is not implemented.
- ADR 14 §6's review lenses are skill prose, not CLI-served data, and findings have no canonical
  JSON shape; §7's guardrails refresh ledger is not built; §11's embedded `removals` list and
  `huddle digest` are not built.
- The row-2 index cache of ADR 19 was deliberately not built — the performance budget passes
  without it.
- `apt-get install huddle` from a bare package name needs a hosted APT repository that does not
  exist yet; `apt install ./huddle_<version>_linux_amd64.deb` works today
  (`docs/packaging.md`).

[Unreleased]: https://github.com/muthuishere/huddle-next/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/muthuishere/huddle-next/releases/tag/v0.1.0
