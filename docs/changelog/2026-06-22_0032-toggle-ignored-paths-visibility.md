# 0032 changes

Task 0031 let a user **Ignore** an unknown folder on the Plan Project screen and
persisted it to the repo's `.agentfiles/state.json` `ignored_paths`. Once
persisted, `sync.Plan` suppressed the folder's whole subtree, so it vanished from
the Changes table with no way to un-ignore short of hand-editing state. This task
adds the deferred surface: **view and un-ignore already-persisted ignored
folders**, and switches persistence from union to **replace** semantics now that
the TUI can see and resend the full set.

A screen-level **Show Ignored** (`g`) / **Hide Ignored** (`h`) toggle reveals the
persisted-ignored folders as collapsed `! ignored` leaves, injected at their
natural nested/sorted position in the one unified tree. On a revealed row the
cursor offers **Show** (`w`) to un-ignore (the row pins visible and its button
flips to **Ignore** (`i`) to toggle back). On Apply the screen sends the complete
desired set `(persisted − unignored) ∪ newly-ignored`, which `sync.Apply` writes
verbatim — so un-ignoring drops the key and the folder's files reappear as
`? unknown` on the next plan.

## Decisions

- Screen toggle mnemonic `g`/`h`, row mnemonic `w`/`i` — **Why:** 0031 already
  binds `i` to the row-level **Ignore** and the `mnemonic.Set` enforces
  uniqueness across cursor-row + screen-level buttons together; a screen-level
  `i` would collide whenever the cursor sat on a registerable folder. `w` reuses
  0031's Show binding (`s` is taken by the global Settings action).
- Replace, not union, in `sync.Apply` — **Why:** the TUI now owns the full
  desired set and must be able to remove a key; union could never drop one.
- Un-ignored rows stay collapsed leaves — **Why:** Plan suppressed the subtree,
  so no children data exists until the un-ignore is Applied and re-Planned.
- An Apply whose only change is the ignore set is valid — **Why:** un-ignoring is
  a real state change even with no file writes.

Not done: rendering a persisted-ignored folder's children, a bulk "un-ignore
all" action, and any change to 0031's live-unknown Ignore behaviour (out of
scope).

## Assumptions

- A persisted-ignored folder key and a live newly-ignored folder key are disjoint
  in practice — **Why:** persisted keys are folders Plan already suppressed; live
  keys are unknown folders still visible this session. The desired-set builder
  deduplicates anyway, so an accidental overlap is harmless.

## Other Notes

- ADR `0010-sync-resolutions-and-first-apply.md` gained a "replace semantics"
  addendum (union → replace, TUI owns the full set).
- `docs/manual/plan_project.md` documents the Show/Hide Ignored toggle and the
  persisted-folder Show ⇄ Ignore row toggle.
- No new ADR or guideline file — this extends 0031's mechanism.

## Data plumbing — `internal/app/service.go`

```go
// before
type Preview struct {
	ProfileID string
	ProjectID string
	Changes   []FileChange
}
// previewFromSync did not read ManagedState.IgnoredPaths
```

```go
// after — Preview carries the persisted ignored set so the TUI can render and
// un-ignore those folders without importing internal/sync
type Preview struct {
	ProfileID    string
	ProjectID    string
	Changes      []FileChange
	IgnoredPaths []string
}
// previewFromSync now copies p.ManagedState.IgnoredPaths (guarded for nil state)
```

## Replace semantics — `internal/sync/sync.go`

```go
// before — union prior + selected
IgnoredPaths: mergeIgnoredPaths(preview.ManagedState, r.IgnoredPaths),
```

```go
// after — write the incoming desired set verbatim
IgnoredPaths: normalizeIgnoredPaths(r.IgnoredPaths),
```

## Plan Project screen — `internal/tui/shell/plan_project.go`

The screen seeds `persistedIgnored` (sorted) from `preview.IgnoredPaths` each
load and tracks `unignored` / `pinned` maps plus the `showIgnored` toggle.
`buildPlanTree` gained an `injectedIgnored []string` parameter; changes and
injected keys are merged into one path-sorted pass over a shared `dirs` map so
the persisted leaves interleave under common parents. `onApply` builds the
desired set `(persisted − unignored) ∪ live-ignored` and a pure ignore-set
change now applies even with no file changes.
