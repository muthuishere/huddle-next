# ADR 6: Versioned, Atomic, Permissive State Storage

- Status: proposed
- Date: 2026-09-14
- v1: built (ADR 19)

## Decision

1. **New global root: `~/.config/.huddle/`.** Everything Huddle owns outside a repo lives there:

   ```
   ~/.config/.huddle/
   ├── config.json                # global config: git_user, owners, hooks[] (ADR 19)
   ├── personas/<id>/             # global personas (ADR 3)
   ├── evidence/<id>/             # private ingested evidence (ADR 10)
   ├── cache/                     # discover results, misc caches
   └── repos/<repo>/
       ├── project-state.json
       ├── drafts/<id>.md         # subagent drafts (ADR 15)
       └── branches/<sanitized-branch>/
           ├── log.jsonl          # event log = audit trail (ADR 14 §1)
           ├── huddle-state.json  # derived cache, rebuildable, never truth
           └── YYYY-MM-DD.md
   ```

   In-repo, team-visible material lives in `<repo>/.huddle/` (personas, `config.json` — repo overrides global key by key) and in playbook artifacts, ADRs, and guardrail files (ADR 12).
   **Deprecated roots, read-only:** `~/.config/muthuishere-agent-skills/` and the older `~/config/muthuishere-agent-skills/`. `huddle migrate` **copies** (never moves, never overwrites) into the new root on first run and writes `~/.config/.huddle/migrated.json`; readers fall back to the old root for anything not yet migrated, for one release. Huddle never writes to the old roots. Builder manifests (`sreyash/`, `hari/`, `harshvardhan/`) are not migrated (ADR 11).
2. **Every JSON file Huddle writes carries `schemaVersion`.** Absent = version 0 (today's files). Readers upgrade in memory; files are rewritten at the new version only when Huddle writes them for a real reason — never just to normalise.
3. **Readers are permissive:** unknown fields are preserved on round-trip (decode into typed struct + `map[string]json.RawMessage` extras); `open_questions` accepts strings and objects; decision evidence accepts both nested (Elango's old shape) and top-level shapes. Corrupt JSON is reported with path and treated as empty — consistently across all commands (today `config_helper` raises while others swallow).
4. **All writes are atomic:** write `*.tmp` in the same directory, `fsync`, `rename`. Directories 0700, files 0600.
5. **Events are collision-free:** `huddle log append` is the one writer for every agent — one JSON line per event, `O_APPEND`, with `session` id; legacy `raw/*.json` is read and folded in on migrate.
6. **Concurrency:** writers to `huddle-state.json` (cache) and `config.json` take an advisory lock file (`.lock`, `O_EXCL`, stale after 30 s). Raw events need no lock.
7. **Branch folder naming:** keep reading today's sanitisation (`/`→`-`, strip leading `.`, empty → `unknown-branch`). New branch folders additionally record the original name in `branch.json` so collisions (`feature/a` vs `feature-a`) are detectable; Windows-invalid characters are percent-encoded for **new** folders only.
8. **One identity resolver:** repo name / branch / root for non-git folders is resolved by a single function used by every command — fixing today's divergence where `project_state`, `session_state`, and `repo_context` can pick different names for the same folder.

## Context

The inventory found:
- Three disagreeing `huddle-state.json` shapes (Python default, `step-02` synthesis policy, Elango persona doc).
- Direct-overwrite writes, no locks, `huddle_writer.py` able to clobber same-ms events, detached migration racing first-run config writes.
- Inconsistent corrupt-JSON handling and non-git identity resolution.
AI Engineer Coach versions its cache (`CACHE_VERSION`) with 0700/0600 permissions; unlike that cache, the log and committed artifacts are the source of truth (hand edits to artifacts win), so we migrate, never discard; `huddle-state.json` is a rebuildable cache (ADR 19).

## Consequences

- A Go struct can't silently drop historical fields; old huddles resume unchanged.
- The canonical `huddle-state.json` v1 schema is written once, as a JSON Schema in `assets/schema/`, and `step-02` + Elango's persona doc are corrected to point at it.
