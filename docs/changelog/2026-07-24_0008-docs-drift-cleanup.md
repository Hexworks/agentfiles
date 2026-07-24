# 0008 changes

Refreshed the "Guidelines referenced from docs/" index in `CLAUDE.md` to
match the current `docs/guidelines/` folder. Two forms of drift were fixed in
one pass: five entries still used the old `_guidelines`-suffixed filenames
(from a prior rename), and nine guideline files that now exist were absent
from the index entirely.

Renamed: `go_guidelines.md`→`go.md`, `domain_model_guidelines.md`→
`domain_model.md`, `sync_and_safety_guidelines.md`→`sync_and_safety.md`,
`asset_authoring_guidelines.md`→`asset_authoring.md`,
`documentation_guidelines.md`→`documentation.md`. Existing one-line summaries
were preserved verbatim; only the filenames changed. `errors.md` was already
correct and left untouched.

Added entries for `clean_architecture.md`, `clean_code.md`, `solid.md`,
`testing.md`, `git.md`, `security.md`, `charm.md`, `tui.md`, and
`external_tools.md`, each with a one-line summary derived from that file's own
opening intent.

Only `CLAUDE.md` was touched — no code, no `docs/guidelines/` content changes.

## Decisions

- Index all nine unlisted guideline files, not just the six named in the
  original task draft — **Why:** user chose the fullest option so the index no
  longer drifts from the folder; `charm.md`, `tui.md`, `external_tools.md` are
  real guidelines and belong in the index too.
- Kept each renamed entry's summary text unchanged — **Why:** the summaries
  were still accurate; the drift was purely in the filename.

Considered but not done: reorganising the guidelines, editing guideline file
contents, adding architecture views — all explicitly out of scope.

## Assumptions

- New-entry summaries were derived from each file's opening lines rather than
  asking the user — **Why:** documentation task; the intent is stated in each
  file's first paragraph, so no user input was needed.

## Other Notes

Two items from the original task draft were verified already-resolved and
dropped from scope during planning:

- `internal/registry/registry.go:20-21` already reads
  `~/.agentfiles/profiles.json`. The draft's suggested `config.RegistryFileName`
  constant does not exist (the real one is `config.ProfilesStoreFileName`).
- `CLAUDE.md:57` "per go_guidelines" phrasing is no longer present.
- `internal/config/paths.go:20` mentioning `.agentprofiles.json` is correct
  v1-migration history (matches `migrate.v1RegistryFileName`), not drift.

Verification (all green): `grep _guidelines CLAUDE.md` → 0; every referenced
`docs/guidelines/<file>` path resolves via `test -f`; `git diff --name-only` →
only `CLAUDE.md`; `make fmt && make lint && make build && make test` pass.

## Rename stale guideline references

```md
// before
- `docs/guidelines/go_guidelines.md` — keep packages cohesive, …
- `docs/guidelines/domain_model_guidelines.md` — registry/profile/asset/… separable
- `docs/guidelines/sync_and_safety_guidelines.md` — the source of the invariants above.
- `docs/guidelines/asset_authoring_guidelines.md` — `asset.json` must have id/name/type; …
- `docs/guidelines/documentation_guidelines.md` — arc42 for architecture views, …
```

```md
// after — filenames match docs/guidelines/, summaries unchanged
- `docs/guidelines/go.md` — keep packages cohesive, …
- `docs/guidelines/domain_model.md` — registry/profile/asset/… separable
- `docs/guidelines/sync_and_safety.md` — the source of the invariants above.
- `docs/guidelines/asset_authoring.md` — `asset.json` must have id/name/type; …
- `docs/guidelines/documentation.md` — arc42 for architecture views, …
```

## Add missing guideline files to the index

```md
// after — nine new bullets appended to the index
- `docs/guidelines/clean_architecture.md` — dependencies point inward; keep the domain free of I/O; cross boundaries through interfaces.
- `docs/guidelines/clean_code.md` — code a future maintainer can understand, change, and verify without reconstructing the whole system.
- `docs/guidelines/solid.md` — SOLID as pragmatic design checks for understandable, changeable, testable code.
- `docs/guidelines/testing.md` — small, direct tests that describe the domain rule; failures should be easy to understand.
- `docs/guidelines/git.md` — one long-lived `main`; scope each commit so its purpose is easy to review.
- `docs/guidelines/security.md` — protect the user's repositories, profile content, local configuration, and credentials.
- `docs/guidelines/charm.md` — conventions for the Charmbracelet stack (bubbletea/lipgloss/huh) used by the TUI.
- `docs/guidelines/tui.md` — TUI structure and interaction conventions; keep business logic out of the view layer.
- `docs/guidelines/external_tools.md` — safely handing control to external binaries via `os/exec` (e.g. the `git` wrapper).
```
