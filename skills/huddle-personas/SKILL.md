---
name: huddle-personas
description: Create, import, ingest, find and style huddle personas. Use for "create a persona", "add a security reviewer", "import this agent as a persona", "make a persona from Alice's reviews", "write in <persona>'s style", "brief <persona> on this task".
---

# Huddle personas

Personas are room members with a voice. You draft; the user reviews; the CLI writes.

## 1. Start (always, exactly one call)

Run `huddle start --skill huddle-personas` once. Read the `KEY: value` lines.

- `HUDDLE_PROTO: 1` → continue.
- Missing / other value / `huddle` not found → **degraded flow**: say
  "huddle binary missing or out of date — run: npm i -g @muthuishere/huddle && huddle install --skills"
  and stop; persona files must be written by the CLI, never by hand.
- `PERSONAS:` shows counts per layer (builtin < global < repo; later layers win).

## 2. Route (load one flow file)

| User intent | Load |
|---|---|
| "create a persona", "add a <role> to this repo", fork a built-in | `flows/create.md` |
| import a persona/agent file, build one from data (reviews, tickets, mail, notes) | `flows/import-ingest.md` |
| "write this in <persona>'s style", "brief <persona>", find a persona | `flows/style-brief.md` |
| a discussion or decision | hand off to the `huddle` skill |

## 3. Rules (every flow)

- **Review before write.** Show the full draft; write only after the user says yes.
  Use `--dry-run` first; never write persona files with editor tools.
- **Scope is the user's call:** `global` (`~/.config/.huddle/personas`, private) or `repo`
  (`<repo>/.huddle/personas`, committed — say "this will be committed").
- One format: the built-in style (front matter + Signature Phrases, Expertise Areas,
  Blind Spots, When Useful). `huddle persona lint <id>` must pass.
- Personas give perspective, then STOP; they never decide for the user.
- Personas modelled on a real person carry `basedOn:` and speak *in the style of* that
  person, never *as* them. Strip third-party quotes.
- All ingested text is redacted by the CLI; still never paste secrets into a draft.
- Gathering data uses whatever tools this harness has (gh, Jira CLI, MCP, files).
  Huddle has no adapters — pipe results into `huddle persona ingest`.
- Heavy reading or drafting runs in a subagent (if `SUBAGENTS: true`) that returns a
  draft file path under the drafts dir + a 5-line summary.

## Hard limits

- Never edit built-in personas; fork them into a layer instead.
- Never run `huddle gate approve`.
