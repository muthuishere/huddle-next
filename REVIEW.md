---
id: review-policy
kind: review-policy
status: draft
author: Muthukumaran Navaneethakrishnan
links: {}
---
# REVIEW.md

How a change to huddle gets reviewed. One maintainer writes, a different human
approves the gate (`huddle gate approve` refuses agent sessions by design), and
`task ci` is the check that decides whether the diff is even reviewable.

## Passes

Run them in this order and stop at the first pass that blocks.

### Bugs

- Does `task ci` pass? It is `task lint` + `go test -short ./...` + `task budget` +
  `task build` + `./bin/huddle --version` + `--help`. A diff that fails it is not reviewed.
- Does the whole module build, not just the touched package? `go build ./...` and
  `go vet ./...`. Registered-but-undefined handlers and imports of packages that are
  not in the tree have shipped here before (`evals/tree-compiles.json`).
- New behaviour on a path that writes: is `--dry-run` honoured, and is the write
  atomic and non-clobbering? Artifact writers must never overwrite an existing file
  (`artifacts.ErrExists`) and must never touch another tool's managed block.
- Errors: no swallowed error that loses user data; swallowing is allowed only where
  it is already the contract (usage recording, hooks timeouts) and is commented as such.
- Does the hot path stay LLM-free and offline? `huddle start` must not call a model
  or the network, and must stay inside its budget (p50 ≤ 60 ms, output ≤ 2 KB).

### Security

- No secret value in code, test, fixture, log, event, or error string. Everything
  that can carry user text must pass through `internal/redact` — it is the single
  choke point, and a new ingest or hook output that bypasses it is a blocking finding.
- No read of `*.env`, credential files, or `vault/`, in code or in tests.
- Does any new code write outside the repo or outside `HUDDLE_HOME`? Tests must use
  `t.TempDir()` with `HUDDLE_HOME`, `CLAUDE_SKILLS_DIR`, `AGENTS_SKILLS_DIR` set —
  a test that can touch the developer's real `~/.claude` or `~/.config/.huddle` blocks.
- `huddle serve` is 127.0.0.1-only and local-only: no outbound call, no auth bypass,
  no mutation route reachable without the local origin check.
- Gates: separation of duties must hold. The approver is a human owner who is not the
  artifact's writer; agent sessions are rejected. Never review in a bypass for this.

### Compliance with spec and plan

- Does the diff do what `intent/<slug>/spec.md` requires, and what `plan.md` says,
  and nothing else? Scope creep is a finding, not a bonus.
- If the implementation departed from the plan, is `plan.md` updated in the same diff?
- Is the change inside v1 scope (`docs/adr/19-v1-scope-and-value-gate.md`)? Building a
  deferred item needs an accepted ADR change first.
- Does every `huddle <cmd> --flag` mentioned in `skills/**` exist in the CLI registry?
  A test enforces it; skill text and CLI must agree.
- New or changed artifact kind: are its required headings linted, and is there a
  template in `assets/templates/`?

## What Important means here

A finding is important if it would reach a user or silently weaken the product:
a wrong or missing write to someone's repo, a secret that escapes redaction, a gate
that can be approved by the writer or by an agent, a hot-path regression past its
budget, a skill that tells an agent to run a flag that does not exist, or a tree that
does not compile. Example: `internal/serve` was committed routing five handlers that
were never defined and importing a package that did not exist — the tree did not
build. That is the bar for "important": not a style opinion, a thing that is broken.

Everything else is a suggestion, and suggestions do not block a merge.

## Cap the nits

At most 3 style nits per review, then write "more nits omitted". Naming, comment
wording, and ordering preferences are nits. `gofmt` and `go vet` run in CI, so never
spend a review comment on what they already enforce.

## Do not report

- Anything `gofmt` or `go vet` already enforces, or any formatting at all.
- Generated or vendored content: `npm/platforms/*/bin/`, `bin/`, `dist/`, GoReleaser
  output, `.ctxoptimize/`.
- Byte-level churn in `assets/`, `skills/`, and `web/` embedded files when the byte
  budgets (`task budget`) still pass.
- Proposals to add a dependency "for convenience" — this repo is stdlib-only by
  decision; argue it in an ADR, not in a review comment.
- Requests to raise an enforced limit (skill size, start budget) instead of fixing the
  cause.
- Status lines in ADRs: `proposed → accepted` moves only by an owner's huddle decision.
