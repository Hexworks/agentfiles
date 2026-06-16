# 0028 changes

Replaced the `selectProjectAssetsStub` placeholder with the real **Select
Project Assets** screen. Hosts two `bubbles/table` widgets stacked
vertically — Selected Assets on top, Available Assets below — both reachable
via `ctrl+1` / `ctrl+2`. Row-level `[Select]` / `[Unselect]` actions
persist immediately through new `Service.UnselectAsset` plus the
already-existing `Service.SelectAsset`, so the upcoming Plan Project
screen (task 0029) plans on whatever state is current with no batched
save step. Screen-level `[Plan]` pushes the existing `planProjectStub`;
`[Back]` (b or esc) pops. Mnemonic alphabet `{1, 2, u, l, p, b}` is
unique by construction and asserted exhaustively in tests.

## Decisions

- **`SelectAsset` / `UnselectAsset` service methods, not `UpdateProject`.** —
  **Why:** the task body says "each row-level select/unselect immediately
  calls `UpdateProject`," but `Service.UpdateProject` explicitly preserves
  `SelectedAssetIDs` and documents that the TUI must never hand it a
  half-mutated `*project.Manifest` ("the merge contract lives here").
  Single-purpose service methods match the existing `SelectAsset`
  prior art and keep the TUI passing only ids across the seam. The task
  wording was shorthand for "persists immediately"; the architectural
  invariant outranks the prose.

- **`Service.UnselectAsset` is idempotent on a never-selected asset.** —
  **Why:** `Service.SelectAsset` is already idempotent on duplicates. The
  symmetric "remove a not-present id" behavior keeps the action surface
  simple, avoids a new `AssetNotInSelectionError` that nobody would handle
  differently from success, and matches what users expect from a toggle.
  Unknown profile / asset ids still return the existing typed errors.

- **Selection is computed from the loaded profile, never from a live
  pointer the screen mutates.** — **Why:** the screen holds
  `selected []*asset.Asset` and `available []*asset.Asset` as derived
  views of `profile.AssetList()` filtered by the project's
  `SelectedAssetIDs`. Row actions persist via the service, then a
  `selectionChangedMsg` carrying the new id set rebuilds the partition.
  No screen-side mutation of a domain aggregate.

- **`profile.AssetList()` ordering owns the visible order — the screen
  does not re-sort.** — **Why:** Both `edit_profile.go` and the new
  screen consume the same already-sorted list; if the canonical order
  ever changes (e.g. by Type then Name) every consumer follows along.

- **`mnemonic.Set` rebuilds in `rebuildSet()` after every focus or
  selection change.** — **Why:** mirrors `edit_profile.go` and
  `edit_asset.go`. The empty-table branches drop the row mnemonic
  (`u` or `l`) so a future addition of another screen-level button on
  the same letter cannot silently collide; `mnemonic.Set.Add` panics on
  duplicates and the exhaustive test walks every focus / selection
  state.

- **`[Back]` is the only screen-level button advertised in the status
  bar.** — **Why:** matches the explicit Settings / Profiles / Edit
  Profile exception. The body already shows `[Plan]` and `[Back]`;
  duplicating `[Plan]` in the bar buys nothing, but `[Back]` is the
  contract for "I can always leave."

## Assumptions

- **The `Plan` button continues to push `planProjectStub` until task
  0029 lands.** — **Why:** task 0028 is explicit that Plan Project's body
  is out of scope. The stub is the existing target; replacing it is
  task 0029's job.

- **No new ADR.** — **Why:** the screen fits cleanly under ADR 0011
  (TUI screen router). `Service.UnselectAsset` is a symmetric extension
  of an existing method, not a new policy.

## Other Notes

- The existing `Service.SelectAsset` had no `Actions` wrapper — added one
  alongside `Actions.UnselectAsset` with `SelectAssetInput` /
  `UnselectAssetInput` for consistency with the other input structs.
- `equalizePanelWidth` and `applyTable` from `edit_profile.go` are reused
  to keep both tables at the same outer width; no new layout helper was
  needed.
- The stub file `select_project_assets_stub.go` plus its references in
  `stub_screens_test.go` were deleted; the Plan Project stub remains.

## Service.UnselectAsset

```go
// before — only the additive half existed
func (s *Service) SelectAsset(profileRef, projectID, assetID string) errs.DomainError { … }
```

```go
// after — symmetric removal, idempotent on a not-present id
func (s *Service) UnselectAsset(profileRef, projectID, assetID string) errs.DomainError {
    loaded, p, err := s.resolveProject(profileRef, projectID)
    if err != nil {
        return err
    }
    if loaded.Assets[assetID] == nil {
        return AssetNotFoundError{AssetID: assetID}
    }
    idx := -1
    for i, existing := range p.SelectedAssetIDs {
        if existing == assetID {
            idx = i
            break
        }
    }
    if idx == -1 {
        return nil
    }
    p.SelectedAssetIDs = append(p.SelectedAssetIDs[:idx], p.SelectedAssetIDs[idx+1:]...)
    return project.Save(loaded.Root, p)
}
```

## Actions wrappers

```go
// before — assets.go ended with file-mutation wrappers; SelectAsset was
// reachable only via app.Service directly.
func (a *Actions) RemoveAssetFile(in RemoveAssetFileInput) (struct{}, errs.DomainError) { … }
```

```go
// after — uniform single-input action wrappers; both delegate straight
// to the service which owns the idempotency contract.
func (a *Actions) SelectAsset(in SelectAssetInput) (struct{}, errs.DomainError) {
    return struct{}{}, a.svc.SelectAsset(in.ProfileRef, in.ProjectID, in.AssetID)
}

func (a *Actions) UnselectAsset(in UnselectAssetInput) (struct{}, errs.DomainError) {
    return struct{}{}, a.svc.UnselectAsset(in.ProfileRef, in.ProjectID, in.AssetID)
}
```

## Screen wire-up

```go
// before — placeholder stub
func (s *editProfileScreen) onSelectAssets() tea.Cmd {
    p, ok := s.selectedProject()
    if !ok {
        return nil
    }
    return pushCmd(newSelectProjectAssetsStub(s.profileID, p.ID))
}
```

```go
// after — real screen, real actions threaded through the existing
// editProfileActions interface (now also embeds selectProjectAssetsActions).
func (s *editProfileScreen) onSelectAssets() tea.Cmd {
    p, ok := s.selectedProject()
    if !ok {
        return nil
    }
    return pushCmd(newSelectProjectAssetsScreen(s.actions, s.profileID, p.ID))
}
```

## Mnemonic rebuild

```go
// after — rebuildSet always exposes [1]/[2] for focus jumps, drops the
// row mnemonic when the focused table is empty, and always offers the
// screen-level [Plan] and [Back]. Candidate alphabet {1, 2, u, l, p, b}
// is unique by construction.
func (s *selectProjectAssetsScreen) rebuildSet() {
    set := mnemonic.NewSet()
    set.Add(s.selectedMnemo)
    set.Add(s.availableMnemo)
    switch s.handler.Focused() {
    case s.selectedIdx:
        if len(s.selected) > 0 {
            set.Add(s.unselectBtn)
        }
    case s.availableIdx:
        if len(s.available) > 0 {
            set.Add(s.selectBtn)
        }
    }
    set.Add(s.planBtn)
    set.Add(s.backBtn)
    s.set = set
}
```
