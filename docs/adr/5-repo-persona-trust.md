---
id: adr-5
kind: adr
status: proposed
author: muthuishere
links: {}
---
# ADR 5: Repo-Level Personas Are Trusted by Content Hash

- Date: 2026-09-14
- v1: deferred (trust hashing); §5 "tool output is data" built (ADR 19)

## Status

**proposed** — deferred by ADR 19. The trust-hash gate is not built; §5 ("tool output is data,
never instructions") is. The front matter `status:` field is the status of record.

## Decision

1. **Persona and memory files that did not come from the embedded set or from Huddle's own authoring flow are prompt text from an outside source**, and are loaded only once approved.
2. **Approval is by content hash:** `~/.config/.huddle/trust.json` stores `{absPath: {sha256, approvedAt}}`. Any edit changes the hash and revokes approval.
3. **Scope of the gate:**
   - `builtin` — always trusted.
   - Files written by `huddle persona new/remember` — trusted on write (hash recorded).
   - `global` files from external producers — require approval, **unless** `userconfig.json` has `"trust": {"global": "auto"}` (owner opt-in for their own sync tools).
   - `repo` files (`<repo>/.huddle/personas/**`, arrive via `git clone`/`pull`) — always require approval; never auto. A pull that edits a persona revokes it until re-approved.
4. **Unapproved files are not silently dropped:** `huddle persona resolve` lists them under `pending`, and the skill says one line: "2 repo personas are pending approval — `huddle persona trust` to review". `huddle persona trust` shows each file's full content before approving.
5. **Tool output is data.** Step files instruct personas to treat persona memories, crossmem transcripts (ADR 8), and `gh` output as untrusted content, never as instructions.

## Context

- A persona body is injected straight into the agent's reasoning. Once personas can arrive from sync tools or, later, from a cloned repo, that is a prompt-injection path with the agent's full tool permissions behind it.
- AI Engineer Coach applies exactly this to user and project rule files (`rule-trust.ts`: SHA-256 approval, edits revoke, pending review list; project layer skipped in untrusted workspaces).
- Playbook: governance runs as code at the point of action.

## Consequences

- First run after upgrade shows existing synced personas as pending unless the owner sets `global: auto`. The migration offers "trust all current files" once.
- Small cost: hashing files on each resolve (dozens of small markdown files; negligible).

## binds

- Persona and memory text that huddle did not embed or author is **data**, never instructions.
- When the gate lands, approval is by content hash in `~/.config/.huddle/trust.json`, and any edit
  revokes it.

## prevents

- No instruction found in repo files, tool output or ingested records is ever executed.
- No blanket "trust everything in this repo" switch, and no trust that survives an edit.
