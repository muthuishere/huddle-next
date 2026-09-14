# huddle

The AI-native SDLC orchestrator. `huddle` takes you and your agents through every stage — setup, intent, spec, plan, build, test, deploy review, maintain — with a room of expert personas for judgment, subagents for the work, and a human gate at each step.

- **Playbook-native artifacts:** `intent/<slug>/intent.md` → `spec.md` → `plan.md`, `CLAUDE.md`, `REVIEW.md`, `evals/`, `bands.yaml`, plus ADRs and spikes.
- **Humans decide:** gates need an owner who didn't write the artifact; agent sessions can't approve.
- **Bring your own tools:** hooks in `config.json` run your `gh`, Jira, Slack, anything, at any lifecycle event.
- **Fast and visible:** `huddle start` runs in a few milliseconds with no network; every command is human-readable, `--json` for agents.
- **LLM-free CLI:** the binary never calls a model. Your agent does the judgment.

## Install

```bash
npm install -g @muthuishere/huddle
huddle install --skills          # huddle, huddle-sdlc, huddle-personas
```

Or build from source: `go install github.com/muthuishere/huddle/cmd/huddle@latest`.

## Usage

```bash
huddle                           # where each change is + the next action
huddle start --json              # the agent's one startup call
huddle intent new pay-retries    # intent/pay-retries/intent.md
huddle spec new pay-retries
huddle plan new pay-retries
huddle gate request intent-pay-retries --role product
huddle gate approve <gate-id>    # a human, in a terminal
huddle adr new "queue retries in the worker"
huddle spike new "can pgvector handle 50M rows?" --timebox 2d
huddle verify record plan-pay-retries --verdict PASS
huddle persona style shaama --for review
huddle persona brief shaama,suren --task "review this diff"
huddle log list --type decision
huddle hooks export --host claude
```

In an agent session, just say "start a huddle" or "take this through the SDLC".

## Config

Two levels, repo wins key by key:

- global: `~/.config/.huddle/config.json`
- repo: `<repo>/.huddle/config.json`

```json
{
  "owners": { "product": ["asha"], "tech-lead": ["muthu"] },
  "hooks": [
    { "on": "artifact.written", "match": { "kind": "intent" }, "run": ".huddle/hooks/jira-epic.sh" },
    { "on": "gate.request", "match": { "name": "deploy" }, "run": "gh pr view --json reviewDecision", "blocking": true }
  ]
}
```

Personas: built-in < `~/.config/.huddle/personas/<id>/` < `<repo>/.huddle/personas/<id>/`.

Upgrading from the Python skill: `huddle migrate --dry-run`, then `huddle migrate`.

## Cross-agent context

`huddle context --agents` lists earlier Claude Code, Codex, Devin, Copilot, and OpenCode sessions for this repo via [crossmem](https://github.com/muthuishere/crossmemcli).

## Safety

- No telemetry. Nothing leaves the machine unless a hook you configured sends it.
- Every ingested record and hook output passes one redaction step before it is stored or shown to an agent.
- Huddle never pushes, merges, deploys, or commits.

## Design

Architecture decisions live in [`docs/adr/`](docs/adr/README.md). v1 scope: [`19-v1-scope-and-value-gate.md`](docs/adr/19-v1-scope-and-value-gate.md).
