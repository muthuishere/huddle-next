# Security Policy

## Reporting a vulnerability

Email **muthuishere@gmail.com** with `huddle security` in the subject line. If you prefer GitHub,
open a private advisory at
<https://github.com/muthuishere/huddle-next/security/advisories/new> — do not open a public issue
for a vulnerability.

Please include:

- the version (`huddle --version`) and platform;
- what an attacker gains, not only what misbehaves;
- the smallest reproduction you have — a repo layout, a `config.json`, a crafted artifact or
  persona file, the commands you ran;
- any `huddle doctor` output, with secrets removed.

**Never put a real credential in a report.** If a finding needs one, say which environment variable
holds it (`$SOME_TOKEN`) and use an obvious placeholder such as `YOUR_TOKEN_HERE` in the
reproduction.

You will get an acknowledgement within 5 working days and an assessment within 14. This is a
one-maintainer project with no paid bounty; fixes ship in the next tagged release, or sooner for
anything that leaks data or executes untrusted input. Please give 90 days before public disclosure,
and credit is yours unless you ask otherwise.

## Supported versions

Only the latest released version is supported. There are no backports.

## What is in scope

- The `huddle` binary and everything under `internal/`, `cmd/`, `assets/` and `skills/`.
- The install path: `huddle install --skills`, the npm launcher
  (`@muthuishere/huddle`) and the platform packages, the Homebrew cask, and the `.deb` / `.rpm`.
- The release supply chain: `.goreleaser.yaml`, the GitHub Actions workflows, and the npm trusted
  publishing configuration.
- `huddle serve` — it is meant to be reachable from the loopback interface only.

### Especially interesting

These are the properties huddle claims; a break in any of them is a vulnerability, not a bug
report:

- **No network on the start path.** `huddle start` must make no network call and invoke no model
  (ADR 2, 13).
- **No telemetry.** Usage data is local-only (ADR 20). Anything that leaves the machine without a
  hook the user configured is a bug.
- **Humans approve gates.** `huddle gate approve|reject` refuses agent sessions by design
  (`HUDDLE_AGENT=1`, or a non-TTY without `--as <owner>`), and a gate needs an approver who did not
  write the artifact. A bypass is a vulnerability.
- **Redaction is a single choke point.** Every ingested record and every hook's output passes
  `internal/redact` before it is stored or shown to an agent. A path that stores or prints
  unredacted secrets is in scope.
- **Outside text is data, never instructions** (ADR 5). A repo persona, an ingested record or a
  hook's output that makes an agent act — prompt injection through a committed file — is in scope.
- **The dashboard is loopback-only.** `huddle serve` must not bind a routable address, must reject
  a foreign `Host` header, and must not approve a gate.
- **Hooks run what the user configured, and only that.** A repo that can make huddle execute a
  command the user did not put in a `config.json` is in scope.

## What is out of scope

- A hook, persona or `config.json` the user wrote themselves doing what it says: hooks are
  user-owned shell commands by design (ADR 17), and `huddle hooks list` shows them.
- Anything requiring an attacker who already has write access to the user's `~/.config/.huddle/` or
  their shell.
- Behaviour of the models or harnesses that drive huddle; the CLI itself never calls a model.
- Missing hardening with no attacker gain (version banners, a lack of rate limiting on a loopback
  server), and automated-scanner output with no demonstrated impact.
- Dependency CVEs with no reachable path — huddle is stdlib-only, so most of these do not apply.
