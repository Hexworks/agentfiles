---
id: 0018
type: feature
status: in-review
depends_on: 0015
---

# Service API: missing CRUD methods

This task expands `internal/app/service.go` with the CRUD operations required
by the new TUI screens. See `0015_task_refactor_ui/description.md`, section
**Service API**, for the full motivation.

## Background

`app.Service` today exposes only the create-path operations
(`CreateProfile`, `RegisterProfile`, `AddProject`, `InitAsset`) plus `Plan` /
`Apply`. The TUI rewrite needs full CRUD because every screen renders editable
lists of profiles, assets, and projects.

## New methods

All calls are stateless (callers pass `profileRef` explicitly, matching the
current pattern) and return `(T, errs.DomainError)` or just `errs.DomainError`
for void operations.

```go
// FolderAction controls whether DeleteProfile also removes the on-disk
// profile folder.
type FolderAction int

const (
    KeepFolders   FolderAction = iota // default
    DeleteFolders
)

// LoadProfiles returns every registered profile fully loaded (assets +
// projects scanned). Heavy operation; the TUI caches the result for the
// Profiles Screen lifetime.
func (s *Service) LoadProfiles() ([]*profile.Profile, errs.DomainError)

// DeleteProfile removes the profile from the registry plus all associated
// assets and projects. folderAction controls whether the on-disk profile
// folder is also removed (DeleteFolders) or kept (KeepFolders, default).
func (s *Service) DeleteProfile(profileRef string, folderAction FolderAction) errs.DomainError

// LoadAsset returns one asset by id within a profile.
func (s *Service) LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError)

// UpdateAsset overwrites the asset manifest and recomputes per-file SHA
// hashes by re-walking the asset directory. Callers do not need to reload
// or rehash beforehand.
func (s *Service) UpdateAsset(profileRef string, a *asset.Asset) errs.DomainError

// DeleteAsset removes the asset folder inside the profile and unselects
// the asset id from every project in the profile. Already-synced files in
// project repos are NOT touched (they remain orphaned).
func (s *Service) DeleteAsset(profileRef, assetID string) errs.DomainError

// LoadProject returns one project manifest by id within a profile.
func (s *Service) LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError)

// UpdateProject overwrites the project manifest.
func (s *Service) UpdateProject(profileRef string, p *project.Manifest) errs.DomainError

// DeleteProject removes project metadata only; the project's repo files are
// NOT touched.
func (s *Service) DeleteProject(profileRef, projectID string) errs.DomainError
```

## Implementation notes

- `LoadProfiles` iterates `registry.Store.Load().Profiles` and calls
  `profile.Load` on each path. Aggregates per-profile load errors with
  `errors.Join` (see `docs/guidelines/errors.md`).
- `DeleteProfile` must:
    - Remove the profile from the registry (`registry.Store.Remove`, add if
      missing).
    - If `DeleteFolders`, `os.RemoveAll` the profile root.
    - Always succeed for the registry side even when the folder is gone.
- `UpdateAsset` re-walks the asset directory and refreshes hashes; that is
  what surfaces a file edit as `ChangeUpdate` on the Plan Project Screen
  rather than `ChangeDrift`.
- `DeleteAsset` iterates every project in the profile and removes the deleted
  asset id from `SelectedAssetIDs`.
- New typed errors per package (`profile/errors.go`, `asset/errors.go`,
  `project/errors.go`) wherever a new failure mode appears (e.g.
  `AssetNotFoundError` exists in `app/errors.go` and may need to move or be
  reused; keep the package boundaries clean per `docs/guidelines/errors.md`).

## Tests

Per package, unit tests for every new method:

- `app/service_*_test.go`: happy path + at least one error path (profile
  missing, asset missing, etc.).
- `DeleteProfile`: covers both `KeepFolders` and `DeleteFolders`, including
  the case where the on-disk folder is already gone.
- `DeleteAsset`: a project that referenced the deleted asset has its
  `SelectedAssetIDs` updated.
- `UpdateAsset`: hashes refresh when file contents change without any
  metadata change.

## Out of scope

- Actions / Notifications wiring (task 0019).
- Any TUI calling these methods (later screen tasks).
- The `registry` package itself — only add a `Remove` helper if not present.

## Verification

```
make build && make test && make lint
```

## Plan

[plan.md](./plan.md)
