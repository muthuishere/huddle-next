---
id: {{id}}
kind: review-policy
status: draft
author: {{author}}
links: {}
---
# REVIEW.md

## Passes

### Bugs

_Logic errors, broken edge cases, missing error handling, races._

### Security

_Injection, secrets in code or logs, authorization gaps, unsafe dependencies._

### Compliance with spec and plan

_Does the diff do what `spec.md` requires and what `plan.md` says, and nothing else?_

## What Important means here

_The findings that block a merge in this repo, with one example._

## Cap the nits

_At most 3 style nits per review; say "more nits omitted" instead of listing them._

## Do not report

- _Generated files, vendored code, formatting a linter already enforces_
