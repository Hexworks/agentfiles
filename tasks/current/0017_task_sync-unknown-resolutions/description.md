---
id: 0017
type: task
status: pending
depends_on: 0015
---

# Sync: ChangeUnknown classification + Resolutions API

This task carves the sync-engine slice out of the parent UI refactor
(see `0015_task_refactor_ui/description.md`, sections **Plan Project** and
**Sync Project**). It contains **no UI work** — purely `internal/sync` and the
matching `app.Service` plumbing.

## Background

`internal/sync/sync.go` already declares `ChangeUnknown` as a `ChangeKind`
constant, but no code path emits it. `detectDeleteCandidates` lumps every
non-desired file inside a managed surface into `ChangeDelete`. `sync.Apply`'s
signature is still `Apply(preview *Preview, deleteCandidates bool)` — a single
bool — which cannot express per-file user choices.

## Scope

### 1. Split classification in `sync.Plan`

Replace the current `detectDeleteCandidates` pass with two cases:

- File recorded in `ManagedState.ManagedFiles` and missing from desired →
  `ChangeDelete` (auto-handled on apply).
- File present in a managed surface (`surfaces.Roots()`), not in
  `ManagedState`, and not in desired → `ChangeUnknown` (requires user
  resolution).

### 2. First-apply policy

When `ManagedState` is missing (first ever apply for the project), treat the
project as a clean slate:

- Every desired file is `ChangeCreate` (overwriting any existing file at that
  path).
- No `ChangeUnknown` entries are emitted; existing files in managed surfaces
  are ignored.
- The first successful `Apply` writes the initial `ManagedState`; subsequent
  plans then distinguish drift from unknown normally.

### 3. New `Resolution` + `FileResolution` types

```go
type Resolution int

const (
    ResolveAuto      Resolution = iota // create / update / delete — always applied
    ResolveOverwrite                    // drift only
    ResolveKeep                         // drift or unknown — no-op
    ResolveDelete                       // unknown only
)

type FileResolution struct {
    Path       string
    Resolution Resolution
}
```

### 4. Change `sync.Apply` signature

```go
func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError
```

Semantics: paths absent from `resolutions` keep the default behavior for their
`ChangeKind` (drift → keep, unknown → keep, create/update/delete → apply).

### 5. Update callers

- `app.Service.Apply` signature follows through: drop `deleteCandidates bool`,
  add `resolutions []sync.FileResolution`.
- Remove the legacy `Preview.DeleteCandidates` field (TODO already in source)
  once the new classification is in place; deletes flow through `Changes`.
- Existing TUI flows (`RunProjectApply` in `internal/tui/forms.go`) will be
  retired in task 0021; for this task, pass an empty `resolutions` slice from
  the existing TUI so the build stays green.

## Tests

- New unit tests in `internal/sync/sync_test.go`:
    - First-apply: missing state → every desired = `ChangeCreate`, no
      `ChangeUnknown`, unknown files in surface are ignored.
    - Subsequent plan: file in state + missing from desired → `ChangeDelete`.
    - Subsequent plan: file in surface, not in state, not in desired →
      `ChangeUnknown`.
    - Apply with `ResolveKeep` for a drift entry leaves the local file
      untouched.
    - Apply with `ResolveOverwrite` rewrites a drift entry.
    - Apply with `ResolveDelete` removes an unknown entry.
- Existing tests in `sync_test.go` must continue to pass (update them to the
  new signature).

## Out of scope

- The Plan Project Screen and its `[Overwrite] / [Keep] / [Delete]` toggle UI
  (task 0029).
- Any change to `render`.

## Verification

```
make build && make test && make lint
```
