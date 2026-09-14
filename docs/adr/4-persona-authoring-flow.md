# ADR 4: `huddle persona new` — Persona Creation Is a First-Class Flow

- Status: proposed
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **Huddle gains a persona authoring flow**, available by default, in two halves:
   - **Skill half** (conversation): "create a persona", "add a security reviewer for this repo" → an interview (role, what they push back on, principles, voice, when to pull them in, anti-patterns) → the model drafts the persona file.
   - **CLI half** (deterministic): `huddle persona new <id> --scope global|repo [--from draft.md | --template | --source …]` validates and writes (`global` → `~/.config/.huddle/personas/<id>/`, `repo` → `<repo>/.huddle/personas/<id>/`; sources: ADR 10); `huddle persona lint [<id>]` validates existing files.
2. **Human review before any write.** The draft is shown in full; the CLI writes only after explicit confirmation (`--yes` is for scripted use, never passed by the skill on its own).
3. **One persona file format — exactly the style the 21 built-ins already use** (e.g. `references/personas/shaama-engineer.md`). No new schema; `lint` codifies what exists:

   ```markdown
   ---
   name: huddle-sre                 # stable id
   displayName: Rahul
   title: SRE
   icon: "🚨"
   role: <one line: what they own>
   domains: [reliability, oncall, observability]
   capabilities: "<comma list of what they are good at>"
   identity: "<background — their win and their scar>"
   primaryLens: "<the one question they always ask>"
   communicationStyle: "<how they speak and push back>"
   principles: "<short declarative beliefs>"
   ---

   ## Signature Phrases
   ## Common Disagreements          # with other roster personas, by name
   ## Expertise Areas
   ## Voices <Name> Has Absorbed     # books / people that shaped them
   ## Non-Goals
   ## Blind Spots
   ## When Useful
   ```

   Required: all front-matter keys above + Signature Phrases, Expertise Areas, Blind Spots, When Useful. The directory name is the roster id (`rahul`); `persona-roster.xml`-style index fields (`id`, `icon`, `name`, `title`, `domains`, `file`) are derived from the front matter, never hand-maintained for user personas.
4. **Validate, then fall back:** the CLI parses the draft; on failure it reports exact errors and offers the scaffold template pre-filled with whatever parsed, so a creation attempt never dead-ends.
5. **Memories are authored the same way:** `huddle persona remember <id> --scope ... ` writes `memories/<slug>.md` with front matter (`title`, `tags`, `source`). The skill offers it when a huddle produces a durable lesson for a persona, and never writes without confirmation.
6. **Built-ins can be forked, not edited:** `huddle persona new shaama --scope repo --fork` copies the embedded body into the repo layer for customisation; ADR 3 precedence makes it win.

## Context

- The inventory: no command creates a persona directory, definition, or memory; `synced_assets.py` is explicitly read-only and producer-agnostic. Personas exist only if some external tool drops files.
- The owner asked for "a separate flow to create personas so it's default".
- AI Engineer Coach's rule editor: LLM drafts markdown, a parser validates, heuristic template on failure — and "author over generate": nothing is written unreviewed.
- The 21 built-in persona files already share one shape (front matter + Signature Phrases / Common Disagreements / Expertise Areas / Voices Absorbed / Non-Goals / Blind Spots / When Useful). Every created or imported persona uses that same style, so they sit in a huddle indistinguishable from built-ins.

## Consequences

- Huddle becomes a writer to the persona directories it previously only read. Externally synced producers (huddle-store, etc.) must tolerate Huddle-authored files; `source: huddle` in front matter marks them.
- `lint` is written against the existing built-ins; ADR 9 asserts all 21 pass unchanged.

## Open questions

- Should `new` also select the persona into the current huddle immediately?
- Rename collisions with historical persona ids (`huddle-global-roster-rename.md`): reject or warn?
