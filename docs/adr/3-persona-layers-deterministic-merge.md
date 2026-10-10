# ADR 3: Persona Layers — Global Folder + In-Repo `.huddle/personas/`, Merged in Code

- Status: accepted — 2026-10-10, owner decision; event `20261010T045742.452Z-01d288` (originating raw decision: README Inputs)
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **Three layers, lowest to highest:**

   | Layer | Location | Shared how |
   |---|---|---|
   | `builtin` | embedded in the binary | ships with Huddle |
   | `global` | `~/.config/.huddle/personas/<id>/` | yours, every repo |
   | `repo` | `<repo>/.huddle/personas/<id>/` | committed with the code, the whole team |

   There is no separate "local per-repo" layer under `~/.config`. **Deprecated, read for one release, then removed:** `~/.config/muthuishere-agent-skills/personas/` (read as `global`) and `~/.config/muthuishere-agent-skills/<repo>/personas/` (read as `global`-for-this-repo). `huddle migrate --personas` copies each (never overwrites) into `~/.config/.huddle/personas/` (private) or `<repo>/.huddle/personas/` (commit it). Global root rationale: ADR 6.
2. **A persona directory is a small folder of markdown, any shape inside:**

   ```
   .huddle/personas/rahul-sre/
   ├── rahul-sre.md        # definition (ADR 4 front matter) — optional if the id is a built-in
   ├── memories/*.md       # lessons, one per file
   ├── style.md            # how they write/review/argue (ADR 10)
   ├── examples/*.md       # real snippets: review comments, PR descriptions, emails
   └── *.md                # anything else — runbooks, glossaries, checklists
   ```

   Only `<id>.md` is special. Every other `.md` is indexed as **persona knowledge** (path, title from first heading or front matter, tags) and read by the model on demand. Non-markdown files are ignored.
3. **`huddle persona resolve --json` returns one merged roster.** The model never performs precedence.
4. **Merge rules (deterministic):**
   - Id = directory name; the same id across layers merges.
   - Metadata: highest layer that **sets** a field wins; unset fields fall through.
   - Definition body: highest layer that has `<id>.md` wins; a folder with only memories/knowledge never erases a lower body.
   - Memories and knowledge files **concatenate** builtin → global → repo, **deduplicated by relative path** (higher layer wins). Each entry carries its `source` layer.
   - `huddle persona resolve <id> --explain` prints where every field and file came from.
5. **Caps:** ≤ 200 persona dirs per layer, ≤ 200 files per persona; overflow reported, not silently read.
6. **Index, not bodies.** `resolve` never inlines knowledge or memory bodies.
7. **`.huddle/` is the repo's Huddle root** (personas, `config.json` — ADR 19). Private working state still lives in `~/.config` (ADR 6) — `.huddle/` holds only what the team should see.

## Context

- Today global and repo personas come back as two lists and `step-01-meeting-init.md` asks the model to apply precedence; memory merge behaviour is undefined.
- The owner wants personal personas in the global folder and team personas inside the repo, as plain markdown folders that can hold "some md and all kind of".
- AI Engineer Coach does the same three layers (built-in → user dir → `<repo>/.ai-engineer-coach/`) with id override, in code.

## Consequences

- Team personas get reviewed in PRs like any other policy file (playbook: skills as versioned policy).
- `<repo>/.huddle/personas/` arrives via `git clone`; trust hashing (ADR 5) is deferred in v1 (ADR 19), so repo personas load like any committed file.
- Behaviour change: memories concatenate instead of whatever the model did before.
