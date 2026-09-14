# Stage 4: Test — measurable targets, verified before human review

**Artifacts:** `evals/*.json` (prompt + checks), `.github/workflows/agent-evals.yml` draft, a verdict
stamped with the tree fingerprint. **Gate:** PASS verdict on the current fingerprint.

## Detect

- `huddle verify status --json` → latest verdict and whether it is stale (tree changed).
- `huddle eval list --json` → existing eval cases.

## Interview (≤ 7)

1. For each spec requirement: which test or eval proves it? (gaps are findings)
2. What real task or incident should become an eval case?
3. What does "acceptable" mean for agent output here? (checks, not vibes)
4. Pass-rate threshold for CI? (default 90%)
5. Which paths should trigger evals? (`CLAUDE.md`, `.claude/**`, `evals/**` by default)
6. Any flaky tests to quarantine first?
7. Performance or cost targets to measure?

## Delegate (parallel)

- **Eval writer subagent:** `huddle eval new "<name>" --dry-run` → draft eval JSON in drafts dir.
- **CI draft:** `huddle eval ci-draft` → workflow draft for the user to adopt. Without an
  authenticated runner the result is `unverified`, never PASS.
- **Verification lenses** (adversarial, edge-case, verification-gap, structure), one subagent
  each, canonical JSON findings with file:line. See the `huddle` skill's verification mode.

## Room

Nina (are the checks real?) + the domain expert for the riskiest requirement. Then STOP.

## Gate

1. Propose PASS / CONCERNS / FAIL with findings. FAIL on any high finding or unproven requirement.
2. The user accepts → `huddle verify record <artifact-id> --verdict <v> --findings findings.json
   --eval-status passed|unverified|blocked|failed`.
3. `huddle gate request plan-<slug>` → owner approves. A PASS on a stale fingerprint doesn't count:
   re-run verification after any change.

## What good looks like

```json
{"name": "refund-timeout",
 "prompt": "Refund order 42 while the provider times out",
 "checks": [{"type": "state", "expect": "refund.status == failed within 60s"},
            {"type": "log-absent", "pattern": "card_number"}]}
```
