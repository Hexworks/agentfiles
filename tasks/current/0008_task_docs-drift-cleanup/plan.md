# Plan — 0008 docs drift cleanup

See [description.md](./description.md).

Pure documentation edit of a single file: `CLAUDE.md` (repo root). No code,
no `docs/` content changes. Fixes two forms of drift in the "Guidelines
referenced from docs/" index — stale renamed filenames, and missing entries
for guideline files that now exist.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Current guideline files (no `_guidelines` suffix) | `docs/guidelines/` listing | `go.md`, `domain_model.md`, `sync_and_safety.md`, `asset_authoring.md`, `documentation.md`, `errors.md`, `clean_architecture.md`, `clean_code.md`, `solid.md`, `testing.md`, `git.md`, `security.md`, `charm.md`, `tui.md`, `external_tools.md` all present |
| CLAUDE.md index uses stale `_guidelines` names | `CLAUDE.md:70,72,73,74,75` | `- `docs/guidelines/go_guidelines.md` — …` (and 4 siblings) |
| `errors.md` ref already correct | `CLAUDE.md:71` | `- `docs/guidelines/errors.md` — typed-error structs…` |
| registry.go comment already fixed (out of scope) | `internal/registry/registry.go:20-21` | `// ProfileRef is the lightweight, global metadata stored in` / `// ~/.agentfiles/profiles.json.` |
| No `config.RegistryFileName` const exists (draft was wrong) | `internal/config/paths.go:22` | `const ProfilesStoreFileName = "profiles.json"` |
| `.agentprofiles.json` in paths.go is correct v1 history, not drift | `internal/config/paths.go:20-22` + `internal/migrate/migrate.go:36` | `const v1RegistryFileName = ".agentprofiles.json"` |
| Index bullet style | `CLAUDE.md:70` | `` - `docs/guidelines/<file>` — <summary>. `` |

## Execution plan

Single edit target: `CLAUDE.md`, section "## Guidelines referenced from docs/"
(lines ~68-77).

### Step 1 — Rename the five stale references

In place, filename only, keep each existing summary verbatim:

- L70 `go_guidelines.md` → `go.md`
- L72 `domain_model_guidelines.md` → `domain_model.md`
- L73 `sync_and_safety_guidelines.md` → `sync_and_safety.md`
- L74 `asset_authoring_guidelines.md` → `asset_authoring.md`
- L75 `documentation_guidelines.md` → `documentation.md`

Leave L71 (`errors.md`) untouched.

### Step 2 — Add the nine missing entries

Append after the existing bullets, before the "Full arc42 set…" line. One
bullet each, matching style. Draft summaries (derived from each file's own
opening — implementer may tighten wording, must stay accurate):

- `clean_architecture.md` — dependency direction inward; domain free of I/O; boundaries via interfaces.
- `clean_code.md` — code a future maintainer can understand/change/verify without reconstructing the whole system.
- `solid.md` — SOLID as pragmatic design checks for understandable, changeable, testable code.
- `testing.md` — small direct tests that describe the domain rule; failures easy to understand.
- `git.md` — one long-lived `main`; commits scoped so each change's purpose is easy to review.
- `security.md` — protect the user's repos, profile content, local config, and credentials.
- `charm.md` — conventions for the Charmbracelet stack (bubbletea/lipgloss/huh) in the TUI.
- `tui.md` — TUI structure/interaction conventions; keep business logic out of the view layer.
- `external_tools.md` — safely handing control to external binaries (per `os/exec`), e.g. the `git` wrapper.

### Step 3 — Sanity pass

Grep `CLAUDE.md` for any remaining `_guidelines` token → expect zero.
Confirm every listed `docs/guidelines/<file>` path resolves to a real file.

## ADRs / docs / guidelines

- No ADR. No new/updated files under `docs/`. No glossary change.
- Only `CLAUDE.md` changes.

## Testing note

No Go code changes → no unit tests. Verification is the grep/path-existence
gate below, promoted into Acceptance Criteria so the DoD gate enforces it.

## Acceptance Criteria

- [ ] The five stale references in `CLAUDE.md` are renamed to `go.md`, `domain_model.md`, `sync_and_safety.md`, `asset_authoring.md`, `documentation.md`; each original summary text is preserved unchanged.
- [ ] `grep -n "_guidelines" CLAUDE.md` returns no matches.
- [ ] Nine new bullets added for `clean_architecture.md`, `clean_code.md`, `solid.md`, `testing.md`, `git.md`, `security.md`, `charm.md`, `tui.md`, `external_tools.md`, each in the existing `` - `docs/guidelines/<file>` — <summary>. `` style.
- [ ] Every `docs/guidelines/<file>` path referenced in `CLAUDE.md` resolves to an existing file: for each, `test -f docs/guidelines/<file>` succeeds (end-to-end against the real filesystem, not an assumed list).
- [ ] `internal/registry/registry.go` and `internal/config/paths.go` are unchanged (confirmed out of scope; `git diff --name-only` lists only `CLAUDE.md`).
- [ ] `make build && make test && make lint` still pass (no code touched — guards against accidental edits).
