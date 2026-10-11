---
id: intent-v1-readiness
kind: intent
status: draft
author: Muthukumaran Navaneethakrishnan
links:
  spec: spec-v1-readiness
  plan: plan-v1-readiness
---
# v1 readiness

## Author

Muthukumaran Navaneethakrishnan

## Status

Draft. The front matter `status:` field is the status of record.

## Problem

Huddle ships as "the AI-native SDLC orchestrator" — it exists to make a team keep a
committed artifact chain, a review policy, control bands and evals. It did not do any
of that to itself. On branch `build/v1-readiness`, `./bin/huddle sdlc status` on huddle's
own repo printed:

```
CLAUDE.md: present (no huddle block)
REVIEW.md: missing
bands.yaml: missing
evals: 0
No intents yet.
```

So the product's own repo failed the value gate it asks of its users. Three concrete
consequences, all observed in the last two weeks of history:

- Six huddle modes existed in the legacy Python skill; three were carried into
  `skills/`. `grep -ri research skills/` returned nothing, so research requests fell
  through to the generic discussion flow. Nothing checked the parity.
- `internal/serve` was committed with five handlers registered on the mux but never
  defined, plus an import of a `web` package that did not exist — the tree did not
  compile. No check ran `go build ./...` before "done".
- Every npm manifest (`npm/package.json` and all six `npm/platforms/*/package.json`)
  shipped with no `license` field, with the OIDC publish workflow already wired.

ADR 9 makes evals a requirement; ADR 19 is the v1 value gate. Neither was being met by
the repo that defines them.

## Proposed outcome

`./bin/huddle sdlc status` on this repo reports a huddle block in CLAUDE.md, REVIEW.md
present, bands.yaml present, at least three evals, and one intent with a linted
intent → spec → plan chain — all of it created by huddle's own CLI, not by hand, so the
act of creating them is also a test of the product. Every `<noun> lint` is clean,
`go test ./...` and `task lint` still pass, and each eval encodes a defect that actually
happened here, with a machine-checkable check that would have caught it.

## Affected users and systems

- This repo only: `CLAUDE.md` (huddle's own `<!-- huddle:context -->` block), `REVIEW.md`,
  `bands.yaml`, `evals/*.json`, `intent/v1-readiness/*`.
- Anyone evaluating huddle: the repo becomes the reference example of the layout.
- No product code changes, no CLI behaviour changes, no release.

## Constraints

- The artifacts are created with `huddle <noun> new`, never hand-written — the point is
  to exercise the product. Any verb that cannot do the job is recorded as a finding
  instead of being papered over.
- Huddle only edits its own fenced block in `CLAUDE.md`; the rest of the file, and other
  tools' managed blocks, are untouched (ADR 12 §7).
- v1 scope is fixed by ADR 19. No new capability, no deferred item.
- `task ci` must stay green; no enforced limit gets raised to make something fit.
- `docs/adr/**` and `skills/**` are owned by other workstreams on this branch.

## Open questions

- `bands.yaml` is a singleton holding exactly one `metric:` key, so skill byte budgets
  and test pass rate cannot be bands in the same file. They are recorded under
  `other_metrics:` as documentation. Does the bands artifact need to become a list of
  metrics, or is one metric per repo the intended design?
- `internal/usage.Budgets` defines a budget for `start` only, so `huddle stats` prints
  `-` in the BUDGET column for every other command. Which commands should get budgets?
- The evals are machine-checkable shell checks plus transcript assertions, but there is
  no runner: `eval ci-draft` emits a workflow whose `command` is supplied by config, and
  without an authenticated runner the result is `unverified`, never PASS (ADR 9). Who
  runs them, and does a deterministic subset need a runner that needs no API key?
