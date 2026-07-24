---
id: 0008
type: task
status: done
topics: documentation
---

# Refresh stale guideline references in CLAUDE.md

Pre-existing drift accumulated as guideline files were renamed (the
`_guidelines` suffix was dropped) and new guideline files were added under
`docs/guidelines/` without being listed in the CLAUDE.md index. None of this
is in the diff of any single task, so it has been carried forward.
Consolidate the cleanup in one pass.

> [!NOTE]
> This description was re-verified on 2026-07-24 against the current repo.
> Two items from the original draft were already resolved and have been
> dropped from scope (see "Already resolved" below).

## Scope

### Rename stale guideline references

`CLAUDE.md` "Guidelines referenced from docs/" section (lines ~68-75) points
at five files under their old `_guidelines` names. Rename to the current
files:

| Stale reference                          | Current file                          |
| ---------------------------------------- | ------------------------------------- |
| `docs/guidelines/go_guidelines.md`             | `docs/guidelines/go.md`             |
| `docs/guidelines/domain_model_guidelines.md`   | `docs/guidelines/domain_model.md`   |
| `docs/guidelines/sync_and_safety_guidelines.md`| `docs/guidelines/sync_and_safety.md`|
| `docs/guidelines/asset_authoring_guidelines.md`| `docs/guidelines/asset_authoring.md`|
| `docs/guidelines/documentation_guidelines.md`  | `docs/guidelines/documentation.md`  |

`docs/guidelines/errors.md` (line 71) is already correct — leave it.

Preserve each line's existing one-line summary; only the filename changes.

### Add missing guideline files to the index

These files exist under `docs/guidelines/` but are absent from the CLAUDE.md
index. Add a one-line entry for each, matching the existing bullet style
(`` - `docs/guidelines/<file>` — <summary>. ``). Summaries should be derived
from each file's own opening/intent, not invented:

- `clean_architecture.md`
- `clean_code.md`
- `solid.md`
- `testing.md`
- `git.md`
- `security.md`
- `charm.md`
- `tui.md`
- `external_tools.md`

## Already resolved (out of scope — do not touch)

- `internal/registry/registry.go:20-21` — the comment already reads
  `~/.agentfiles/profiles.json` and is accurate. The original draft asked to
  point it at `config.RegistryFileName`, but no such constant exists (the real
  constant is `config.ProfilesStoreFileName = "profiles.json"`). No change
  needed.
- `CLAUDE.md:57` "per go_guidelines" — this phrasing is no longer present
  anywhere in the file. Nothing to fix.
- `internal/config/paths.go:20` mentioning `.agentprofiles.json` is correct
  historical context for the v1→v2 migration (matches
  `migrate.v1RegistryFileName`). Not drift.

## Out of scope

- Reorganising the guidelines themselves.
- Editing the content of any file under `docs/guidelines/`.
- Adding new architecture views.
