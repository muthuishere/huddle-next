---
id: adr-23
kind: adr
status: proposed
author: Muthukumaran Navaneethakrishnan
links: {}
---
# ADR 23: Optional spec-framework awareness, native by default

- Date: 2026-10-11
- Extends: ADR 12 (writers on the playbook layout) — it is not amended; this ADR stays inside it
- Related: ADR 16 (`sdlc status`), ADR 19 (v1 scope; `custom` mapping still deferred)

## Status

**proposed** — 2026-10-11. The front matter `status:` field is the status of record.

## Decision

1. **Huddle may *notice* an external spec framework. It may never depend on one.** `internal/specframework`
   detects a framework's directory (today only `openspec/` with its `config.yaml`) and, *only* if that
   framework's binary is already on `PATH`, runs its read-only `list`/`status --json` commands to **locate
   files**. That is the entire capability: where someone else's artifacts are.
2. **Native is the default and the fallback.** Binary missing, directory missing, command failing, JSON
   unparseable, framework unknown — every one of those yields "no framework", which is the normal case.
   Huddle then reports its own playbook paths (`intent/<slug>/{intent,spec,plan}.md`) and behaves exactly
   as it does in a repo that never heard of OpenSpec. Absence is not a warning and never an error.
3. **Huddle's own artifacts stay native, always.** Detection changes no path huddle writes to, no template,
   no lint rule, no gate, no event.
4. **Where it surfaces:** an informational `spec-framework` line in `huddle doctor` (status `ok`, never
   `warn` — absence must not look like a defect) and, in `huddle sdlc status`, a `SPEC FRAMEWORK:` block
   listing each external change, its directory, and which of its artifacts exist.
5. **No exec on the hot path.** `specframework.Detect` is filesystem + `exec.LookPath` only, so
   `sdlc.Status` (and therefore `huddle serve`) starts no process. The one exec lives in
   `Client.Changes`, reached only from human-invoked `huddle sdlc status`. `huddle start` never
   reaches it and keeps its p50 ≤ 60 ms, exec-free, network-free budget (ADR 13, 19).
6. **The external tool's output is untrusted data.** Arguments are passed as a slice, never through a
   shell; change ids are validated against `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$` so they cannot act as a
   flag or a path; the child gets a bounded environment, a 3 s timeout, the repo root as cwd, a capped
   read and a discarded stderr. Paths come back cleaned, control-stripped, passed through
   `internal/redact`, made repo-relative, and dropped if they point outside the repo. Nothing in that
   output is ever read as instruction.
7. **The stage labels are a reading aid, not a mapping.** `proposal→intent`, `specs→spec`,
   `design→adr`, `tasks→plan` are printed next to the external paths so a human can read the two chains
   side by side. Huddle neither reads nor writes those files, and no artifact kind resolves through them.
8. **Review is format-agnostic by nature.** `skills/huddle/modes/spec-review.md` pressure-tests a spec in
   a room of personas. A room reading a markdown file needs no adapter, so the mode works on
   `intent/<slug>/spec.md` or on any file the user points at — including an OpenSpec `proposal.md`,
   `design.md` or `specs/**/*.md`. It reviews; it never writes or validates a foreign file.

## Context

- The owner's instruction: *"if openspec is not there use your own way to do"* — optional detection,
  native default, graceful absence.
- ADR 12 already settled the format question and quotes the owner: *"Huddle is not OpenSpec; the native
  format is the AI-native SDLC playbook; if OpenSpec is unnecessary, remove it."* §1 ships no OpenSpec /
  spec-kit / BMAD adapter; §2 makes `custom` path mapping the only escape hatch (deferred by ADR 19);
  §3 rules out "detection magic" and framework `init`; its **prevents** block forbids "no adapter for,
  dependency on, or validation against any spec framework" and "no framework detection and no
  third-party `init`: huddle never bootstraps another tool's layout."
- **Why this is not a contradiction.** ADR 12 §3's "detection magic" is detection that *decides*: it
  would pick a layout, switch huddle's artifact paths, or bootstrap another tool. This ADR builds none
  of that. Nothing is initialised, no path huddle writes to changes, no validation is performed, and the
  binary is never installed or required. Informing a human where another tool's files already sit is
  the same class of act as `huddle doctor` printing whether `git` is on `PATH` — and §7's guardrails
  rule already obliges huddle to *know about* other tools' managed blocks so it does not clobber them.
  If this detection ever began to decide, it would need ADR 12 amended; it does not, so ADR 12 stands
  unchanged.
- OpenSpec 1.6.0's `spec-driven` schema (verified 2026-10-11) is near-isomorphic to huddle's chain:
  four artifacts, `openspec status --change <id> --json` giving `changeRoot`, `artifactPaths`,
  `existingOutputPaths` and `isComplete`. It has no personas, no deliberation, no gates and no audit
  log; huddle has those and no delta-spec format. That asymmetry is why complementing beats adapting:
  the room is worth more on their spec than a converter would be.

## Details

- `specframework.Detect(root) *Detection` — nil when absent. `Line(*Detection)` is the one human line.
- `specframework.New(root, det) *Client` — nil when there is no framework *or* no binary, so
  `Client.Changes` on a nil client is a quiet no-op that returns nothing and no error.
- `sdlc.Report.SpecFramework` carries the (exec-free) detection; the exec-read changes ride on
  `huddle sdlc status --json` as `specFrameworkChanges`, outside `sdlc.Report` so `serve` cannot exec.
- Adding a second framework is a row in the `known` table: a directory to stat, a marker file, a binary
  name. Reading one still requires an explicit `Changes` branch, which is deliberate friction.
- Tests cover absence as seriously as presence (`internal/specframework/specframework_test.go`,
  `internal/app/cmd_specframework_test.go`) and never require `openspec` to be installed: the directory
  is faked, `PATH` is emptied, the runner is stubbed, and the real exec path is exercised against a
  three-line shell script.

## Consequences

- A team already on OpenSpec gets huddle's judgement layer — personas, gates, the audit log, spec-review
  — on the specs they already have, with nothing to migrate and nothing to install.
- Huddle keeps zero dependencies (`go.mod` stays empty) and one format to template, lint and eval.
- A framework we do not recognise, or a repo with none, is indistinguishable from the past: same paths,
  same output, no warnings.
- We owe the fakes: when OpenSpec changes its JSON, detection silently degrades to "no changes" rather
  than breaking huddle — which is the intended failure, but means a drift is quiet. `huddle sdlc status`
  showing a framework with no changes is the signal to re-verify by hand.

## Open questions

- Should `huddle sdlc status` go further and *pair* an external change with a native slug of the same
  name, or does pairing start to look like the mapping ADR 12 §2 defers? Pairing is not built.
- Is there demand for the reverse direction — the room's findings written back as a comment in the
  framework's own file? Today the answer is no: huddle never writes a foreign file.
- Does any second framework (spec-kit, BMAD) justify a row in `known`, or is OpenSpec the only one
  teams actually arrive with?

## binds

- Detection is optional, read-only, and informational. Huddle's artifacts stay native in every case.
- Every call into an external CLI is argument-vector exec with a timeout, a bounded environment, a
  capped read, a validated id and `internal/redact` on the way out — and lives off the `huddle start`
  path.
- Absence of a framework is reported as `ok`, never as a warning or a failure.

## prevents

- No writing of another framework's files, no `openspec validate` (or any framework validation), no
  conversion between formats, and no framework `init` — unchanged from ADR 12 §1 and its prevents block.
- No required dependency: huddle never installs, bundles, or demands `openspec`, and never degrades
  when it is missing.
- The `custom` artifact path mapping of ADR 12 §2 stays deferred (ADR 19); this ADR does not build it,
  and detection must not grow into it.
- No exec and no network anywhere on the `huddle start` path.
