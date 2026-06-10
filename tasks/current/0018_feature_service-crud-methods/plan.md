# Plan — Task 0018: Service CRUD methods

Links:

- Task description: [./description.md](./description.md)
- Architecture (level 2): [`docs/architecture/05-building-block-view.md`](../../../docs/architecture/05-building-block-view.md) (`app` block)
- Error guideline: [`docs/guidelines/errors.md`](../../../docs/guidelines/errors.md)
- Domain model guideline: [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md)

## Context

`app.Service` currently exposes only the **create-path** operations
(`CreateProfile`, `RegisterProfile`, `AddProject`, `InitAsset`, `Plan`,
`Apply`). The TUI refactor (task 0015 family) needs **full CRUD** because
every screen lists editable profiles, assets, and projects. Task 0018 is the
backend half — no TUI wiring, no actions yet. Task 0019
(Actions & Notifications) and the later screen tasks consume what 0018 adds.

The change is scoped to `internal/app/service.go` plus tiny additions to
`internal/registry` (a `Remove` helper) and `internal/project` (a `Delete`
helper for the project-file path). No new domain rules — these methods
compose existing primitives.

## Branch

```
feature/service-crud-methods
```

Branch from current `master` after the working tree is verified clean.

## Surface to add (in `internal/app/service.go`)

```go
type FolderAction int
const (
    KeepFolders FolderAction = iota
    DeleteFolders
)

func (s *Service) LoadProfiles() ([]*profile.Profile, errs.DomainError)
func (s *Service) DeleteProfile(profileRef string, folderAction FolderAction) errs.DomainError
func (s *Service) LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError)
func (s *Service) UpdateAsset(profileRef string, a *asset.Asset) errs.DomainError
func (s *Service) DeleteAsset(profileRef, assetID string) errs.DomainError
func (s *Service) LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError)
func (s *Service) UpdateProject(profileRef string, p *project.Manifest) errs.DomainError
func (s *Service) DeleteProject(profileRef, projectID string) errs.DomainError
```

## Helpers added in supporting packages

### `internal/registry/registry.go`

Add `Remove(profileID string) errs.DomainError` next to `Add`/`Touch`. It
loads the registry, filters out the matching `ProfileRef`, and saves.
Missing id → `ProfileNotFoundError{Ref: profileID}` (already exists).

### `internal/project/project.go`

Add `Delete(profileRoot, projectID string) errs.DomainError`. Removes the
project manifest file at `<profileRoot>/projects/<id>.json`. Missing file is
treated as success (idempotent — consistent with `DeleteProfile`'s "always
succeed for registry side even when the folder is gone" rule).

## Implementation details per method

### `LoadProfiles`

- Load registry once.
- For each `ProfileRef`: call `profile.Load(ref.Path)`. On success, append
  to the result. On per-profile failure, accumulate into
  `[]errs.DomainError`.
- Return both: the successfully-loaded subset plus an aggregated error
  (`errs.Errors(failures)` if any) — this matches the project convention in
  `docs/guidelines/errors.md` §"Non-accumulator Functions Wrap In
  `errs.Errors`".
- Profile order: keep registry order (already sorted by name in
  `Store.Save`).

### `DeleteProfile`

- Resolve `profileRef` via `s.Registry.Resolve`. Missing →
  `ProfileNotFoundError`.
- Call new `s.Registry.Remove(ref.ID)`.
- If `folderAction == DeleteFolders`: `os.RemoveAll(ref.Path)`. A
  pre-missing folder is success; any other failure wraps as a new typed
  error `ProfileFolderRemoveError{Path, Err}` in `app/errors.go`.
- Registry side **always succeeds** even when the folder is gone.

### `LoadAsset`

- `LoadProfile(profileRef)` (already does registry touch + profile load).
- `a := loaded.Assets[assetID]`. If `nil` →
  `AssetNotFoundError{AssetID: assetID}` (reuse the existing one in
  `app/errors.go`).
- Return `a`.

### `UpdateAsset`

- `LoadProfile(profileRef)`.
- Verify `loaded.Assets[a.ID] != nil`; else `AssetNotFoundError`.
- Validate the incoming manifest via `a.Manifest.Validate()`.
- Write `<a.Dir>/asset.json` via `utils.WriteJSON(..., a.Manifest)` — this
  overwrites the manifest with the user's edits. Pass `a.Manifest` (not
  `a`) so we don't serialize `Dir`.
- The task text says "recomputes per-file SHA hashes by re-walking the
  asset directory". The current `asset.Asset` type has **no** hash field
  — `internal/sync` already computes hashes on the fly from disk +
  `state.json` baseline, so the drift-vs-update distinction works without
  any asset-side hash store. **Decision**: this method writes the
  manifest; the re-walk is a no-op. Flagged below in
  [Open question](#open-question-flagged-for-the-users-review-of-this-plan).

### `DeleteAsset`

- `LoadProfile(profileRef)`.
- `a := loaded.Assets[assetID]`. Missing → `AssetNotFoundError`.
- For every project in `loaded.Projects` whose `SelectedAssetIDs` contains
  `assetID`: drop the id and call `project.Save(loaded.Root, p)`.
  Iteration over `loaded.ProjectList()` (already sorted) keeps saves
  deterministic.
- `os.RemoveAll(a.Dir)`. Wrap real removal failures as a new typed error
  `AssetFolderRemoveError{Dir, Err}` in `app/errors.go`. A pre-missing
  directory is success.

### `LoadProject`

- `LoadProfile(profileRef)`.
- `p := loaded.Projects[projectID]`. Missing → `ProjectNotFoundError`
  (already exists in `app/errors.go`).
- Return `p`.

### `UpdateProject`

- `LoadProfile(profileRef)`.
- Verify `loaded.Projects[p.ID] != nil`; else `ProjectNotFoundError`.
- Call `project.Save(loaded.Root, p)` (Save already normalizes +
  validates).
- Out of scope: ownership / path collision re-checks. Editing a project
  never changes its existing path in the TUI flow (a separate "move
  project" flow would re-run the check). If we decide otherwise later,
  add the check before `Save`.

### `DeleteProject`

- `LoadProfile(profileRef)`.
- `p := loaded.Projects[projectID]`. Missing → `ProjectNotFoundError`.
- Call new `project.Delete(loaded.Root, p.ID)`.

## New typed errors

In `internal/app/errors.go`:

- `ProfileFolderRemoveError{Path string; Err error}` — `Severity()=Error`,
  `Unwrap()=Err`. Used by `DeleteProfile` when `os.RemoveAll` returns a
  non-missing error.
- `AssetFolderRemoveError{Dir string; Err error}` — same shape, used by
  `DeleteAsset`.

Existing errors reused: `AssetNotFoundError`, `ProjectNotFoundError`,
`ProfileNotFoundError` (from `registry`).

In `internal/project/errors.go`:

- `ProjectDeleteError{Path string; Err error}` — non-missing `os.Remove`
  failure inside the new `project.Delete` helper.

In `internal/registry/`: no new error; `Remove` uses existing
`ProfileNotFoundError`.

## Tests

All new tests use `t.TempDir()` and typed-error assertions
(`errors.As`) per `docs/guidelines/testing.md` and the existing files in
`internal/app/`.

### `internal/registry/registry_test.go`

- `TestRemove_DeletesProfileRef` — Add then Remove; Load returns empty.
- `TestRemove_MissingIDReturnsProfileNotFoundError`.

### `internal/project/project_test.go` (new file)

- `TestDelete_RemovesManifestFile`.
- `TestDelete_MissingFileIsIdempotent`.

### `internal/app/service_load_profiles_test.go` (new)

- `TestLoadProfiles_ReturnsRegisteredProfiles`.
- `TestLoadProfiles_AggregatesPerProfileLoadErrors` — corrupt one
  profile's `profile.json`; `LoadProfiles` returns the healthy one plus a
  wrapping `errs.DomainError` whose `errs.Collect` yields a non-empty
  list.

### `internal/app/service_delete_profile_test.go` (new)

- `TestDeleteProfile_KeepFoldersRemovesRegistryEntryOnly`.
- `TestDeleteProfile_DeleteFoldersRemovesBoth`.
- `TestDeleteProfile_FolderAlreadyMissingSucceeds`.
- `TestDeleteProfile_UnknownRefReturnsProfileNotFoundError`.

### `internal/app/service_asset_test.go` (new)

- `TestLoadAsset_ReturnsExisting`.
- `TestLoadAsset_MissingReturnsAssetNotFoundError`.
- `TestUpdateAsset_OverwritesManifestOnDisk` — scaffold asset, edit
  description, call `UpdateAsset`, reload via `LoadAsset`, assert new
  field.
- `TestUpdateAsset_MissingAssetReturnsError`.
- `TestDeleteAsset_RemovesAssetDirectory`.
- `TestDeleteAsset_UnselectsAssetFromProjects`.

### `internal/app/service_project_test.go` (new)

- `TestLoadProject_ReturnsManifest`.
- `TestLoadProject_MissingReturnsProjectNotFoundError`.
- `TestUpdateProject_PersistsChanges`.
- `TestDeleteProject_RemovesManifestFile`.

## Files touched

- `internal/app/service.go` (add `FolderAction` + 8 methods).
- `internal/app/errors.go` (2 new typed errors).
- `internal/registry/registry.go` (add `Remove`).
- `internal/registry/registry_test.go` (2 tests).
- `internal/project/project.go` (add `Delete`).
- `internal/project/errors.go` (1 new typed error).
- `internal/project/project_test.go` (new).
- `internal/app/service_load_profiles_test.go` (new).
- `internal/app/service_delete_profile_test.go` (new).
- `internal/app/service_asset_test.go` (new).
- `internal/app/service_project_test.go` (new).
- `tasks/current/0018_feature_service-crud-methods/description.md` —
  status `pending` → `in-progress` before implementation, then `in-review`
  after; add a `## Plan` section linking `./plan.md`.
- `docs/changelog/2026-06-10_0018-service-crud-methods.md` (new).

No ADR. No guideline updates. The methods compose existing primitives
without introducing a new architectural concept.

## Verification

```
make build
make test
make lint
```

Targeted suites:

```
go test ./internal/registry -run TestRemove
go test ./internal/project  -run TestDelete
go test ./internal/app      -run "TestLoadProfiles|TestDeleteProfile|TestLoadAsset|TestUpdateAsset|TestDeleteAsset|TestLoadProject|TestUpdateProject|TestDeleteProject"
```

## Open question (flagged for the user's review of this plan)

`UpdateAsset` per task description: "recomputes per-file SHA hashes by
re-walking the asset directory". The current `asset.Asset` type has **no**
hash field — `internal/sync` already computes hashes on the fly from disk
+ `state.json` baseline, so the "edit shows as `ChangeUpdate` not
`ChangeDrift`" outcome described by task 0015 falls out of sync's algorithm
without an asset-side hash store. My plan therefore makes `UpdateAsset`
rewrite `asset.json` only.

If you want me to add a `Hashes map[string]string` field on `asset.Asset`
(in-memory, not persisted) and refresh it here + during `asset.Load`, say
so before approval and I'll expand the plan.
