# Mode: verification

Pressure-test a claim, a change, or "is this done" before anyone treats it as done.
The output is a verdict: **PASS | CONCERNS | FAIL**, with reasons, stamped to the tree.

## Steps

1. **Pin the target** in one line: what claim / diff / artifact, and against what
   (intent, spec, plan, `REVIEW.md`). Ask if unclear — one question.
2. **Check freshness:** `huddle verify status --json`. A verdict whose fingerprint no longer
   matches the tree is **stale** — say so; never reuse it.
3. **Run lenses in parallel subagents** (inline if `SUBAGENTS: false`), each returning
   canonical JSON findings `{lens, severity: high|medium|low, claim, evidence: file:line, fix}`:
   - `adversarial` — at least 10 findings; try to break it.
   - `edge-case` — inputs, limits, concurrency, failure paths.
   - `verification-gap` — what is asserted but not proven by a test, eval, or metric.
   - `structure` — does the change match the plan's "Files that change" and the spec.
   Pick 1–2 personas from the diff scope (Senthil for auth, Nina for tests, Shaama for backend)
   to voice the top findings.
4. **Separate evidence from opinion.** Every finding needs a file:line, command output, or
   test name. Findings without evidence are marked `claim: unverified`.
5. **Propose the verdict** to the user:
   - PASS — no high findings, every requirement has proof.
   - CONCERNS — no high findings, but gaps the user should accept or fix.
   - FAIL — any high finding, or a requirement with no proof.
6. **STOP.** The user accepts, disputes, or asks for fixes.
7. On the user's acceptance, record it:
   `huddle verify record <artifact-id> --verdict PASS|CONCERNS|FAIL --findings findings.json`

## Rules

- You propose a verdict; the user owns it. Never record a verdict the user hasn't accepted.
- Never soften a FAIL into CONCERNS to keep momentum.
- A PASS on a stale fingerprint is not a PASS.
- Merge/deploy approval is a gate a human runs (`huddle gate approve`), never you.
- Incident or escaped bug found? Offer an eval case (`huddle eval new`) — via `huddle-sdlc`.

## What good looks like

> Verdict proposed: **CONCERNS** (tree 3f9a1c2)
> - medium · edge-case · `refund.go:41` retries on 409 without an idempotency key → double refund possible. Fix: pass `Idempotency-Key`.
> - low · verification-gap · spec R3 "refund within 5s" has no test.
> No high findings. Accept as CONCERNS, or fix first?
