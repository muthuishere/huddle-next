# Stage 6: Maintain — turn incidents, metrics and findings back into work

**Artifacts:** `bands.yaml` (metric · baseline · rules · tiers), new `intent/<slug>/intent.md`
from a breach or finding, `docs/lessons/<date>-<slug>.md`, learnings, refreshed `CLAUDE.md`.
**Gate:** the owner triages (accept into Plan, defer, or dismiss with a reason).

## Detect

- `huddle log list --type action,learning --limit 20 --json` → actions carried across huddles.
- `huddle guardrails status --json` → stale guardrail lines.
- `huddle lessons list --json`, `huddle timeline --limit 30` (recent gates, verdicts, drift).

## Interview (≤ 7)

1. What happened? (incident, metric move, scan result, repeated review finding)
2. What evidence do we have? (the user or their tools bring it — Huddle has no adapters)
3. Impact: who was affected, how long?
4. Root cause known, or does it need diagnosis?
5. Which metric should have caught it? Baseline?
6. What eval would have caught it before release?
7. Does `CLAUDE.md` need a "Things Claude gets wrong" line?

## Delegate

- **Diagnosis scout** (Vel's lens): reads what was ingested + code; returns cause hypotheses
  with evidence, ≤ 400 tokens.
- **Bands:** the user supplies a metric series → `huddle bands check --series series.csv`
  (1σ log, 2σ diagnose, 3σ draft an intent; `--write` on the user's yes). Dismissing a breach:
  `huddle bands dismiss <event-id> --reason "<why>"`.
- **Lessons writer:** `huddle lessons new "<title>" --dry-run` → incident, cause, eval added, intent link.
- **Intent writer** for accepted follow-up work → see `stages/plan.md`.
- **Eval writer** for the regression case → see `stages/test.md`.

## Room

Vel (diagnosis) + Wei (is the metric honest?) + the owner's domain expert. Then STOP.

## Gate

1. Show: cause (with evidence) · proposed lesson · eval · intent draft.
2. The owner triages each item; dismissals are logged with a reason.
3. Accepted items: write lessons/intent via `huddle lessons new "<title>" --from <draft>` after the user's yes; log learnings:
   `huddle log append --type learning --key <slug> "<lesson>"`.
4. Guardrails refresh goes back through `stages/setup.md`'s gate.

## What good looks like

```markdown
# 2026-09-02 double refund
Cause: retry on HTTP 409 without idempotency key (`refund.go:41`).
Eval added: `evals/refund-409-retry.json`.
Intent: `intent/refund-idempotency/intent.md`.
CLAUDE.md: "Every provider call carries an Idempotency-Key."
```
