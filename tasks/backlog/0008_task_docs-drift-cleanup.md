---
id: 0008
type: task
status: Pending
topics: documentation
---

# Refresh stale doc references in CLAUDE.md and registry.go

Pre-existing drift accumulated as guideline files were renamed and the
registry filename changed. None of this is in the diff of any single task,
so it has been carried forward. Consolidate the cleanup in one pass.

## Scope

- `CLAUDE.md:63-67` references five guideline files under their old names
  (`go_guidelines.md`, `domain_model_guidelines.md`,
  `sync_and_safety_guidelines.md`, `asset_authoring_guidelines.md`,
  `documentation_guidelines.md`). Current files live at
  `docs/guidelines/{go,domain_model,sync_and_safety,asset_authoring,documentation}.md`.
- `CLAUDE.md:57` mentions "per go_guidelines" — same rename.
- The new guideline files (`clean_architecture.md`, `clean_code.md`,
  `solid.md`, `testing.md`, `git.md`, `security.md`) are present under
  `docs/guidelines/` but absent from the CLAUDE.md "Guidelines referenced
  from docs/" index.
- `internal/registry/registry.go:21` says
  `// ProfileRef is the lightweight, global metadata stored in ~/.llmprofiles.json.`
  — actual file is `~/.agentprofiles.json` (`config.RegistryFileName`).
  Reword to point at `config.RegistryFileName` so the comment stays linked
  to the single source of truth.

## Out of scope

- Reorganising the guidelines themselves.
- Adding new architecture views.
