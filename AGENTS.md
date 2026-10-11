# AGENTS.md

Authoritative guardrails for any agent working in this repo. `CLAUDE.md` points here.

- Huddle is an LLM-free Go CLI plus agent skills. The CLI never calls a model and never touches the network on the `huddle start` hot path.
- Never read or export `*.env`, credential files, auth databases, or `vault/`. Refer to secrets by env-var name only.
- Never run `huddle install --skills`, `huddle migrate`, or anything that writes to the real `~/.claude`, `~/.agents`, or `~/.config/.huddle` while developing. Tests use `t.TempDir()` with `HUDDLE_HOME`, `CLAUDE_SKILLS_DIR`, `AGENTS_SKILLS_DIR`.
- Agents never approve gates. `huddle gate approve|reject` refuses agent sessions by design — do not add a bypass.
- Committed artifacts are the source of truth; the event log is the audit trail. Never make a derived cache authoritative.
- Keep `huddle start` p50 ≤ 60 ms and its output ≤ 2 KB; keep every `SKILL.md` ≤ 6 KB. The benchmarks and budget tests enforce this — fix the cause, don't raise the limit.
- Skill text and CLI must agree: every `huddle <cmd> --flag` in `skills/**` must exist (a test checks it).
- Stdlib only. The crossmem library is linked only with `-tags crossmem`; the default build shells out to the `crossmem` CLI.
- Skills install globally only; never commit `.claude/` or `.agents/`.
- Never publish from a laptop. Releases go through the tag-triggered workflow with npm OIDC trusted publishing.
- Scope for v1 is `docs/adr/19-v1-scope-and-value-gate.md`. Deferred items stay in the ADRs; don't build them without an approved ADR change.
