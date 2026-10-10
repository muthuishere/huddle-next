# huddle

**The AI-native SDLC orchestrator for coding agents.** Expert-persona huddles for decisions,
playbook artifacts (intent, spec, plan, ADR, spike, evals), human-approved gates, and
lifecycle hooks — one local binary, no network on the hot path, no telemetry.

## Install

```sh
npm install -g @muthuishere/huddle
huddle install --skills     # installs huddle, huddle-sdlc, huddle-personas skills
huddle doctor
```

This package is a thin launcher that resolves a prebuilt native binary for your platform
(`darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `windows-x64`) via optional
dependencies — no Go toolchain required.

Also available via:

```sh
brew install --cask muthuishere/tap/huddle
go install github.com/muthuishere/huddle-next/cmd/huddle@latest
```

## Use

In Claude Code or Codex: "start a huddle", "what's next for this change", "create a persona".
In a terminal: `huddle`, `huddle sdlc status`, `huddle hooks list`, `huddle --help`.

