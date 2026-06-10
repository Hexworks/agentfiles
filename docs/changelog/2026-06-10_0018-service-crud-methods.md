# 0018 changes

Expanded `app.Service` with the full set of Read/Update/Delete operations the
TUI rewrite (task 0015 family) needs. The Service previously exposed only
create-path operations (`CreateProfile`, `RegisterProfile`, `AddProject`,
`InitAsset`) plus `Plan`/`Apply`. The new methods compose existing primitives
in the `registry`, `profile`, `asset`, and `project` packages — no new domain
rules — and stay consistent with the typed-error convention in
`docs/guidelines/errors.md`.

Two tiny helpers were added in supporting packages: `registry.Store.Remove`
and `project.Delete`. They live with the domain concept they belong to so the
Service stays a thin coordinator.

## Decisions

- `UpdateAsset` writes the asset manifest only; no per-file SHA recompute. —
  **Why:** the current `asset.Asset` model has no hash field, and
  `internal/sync` already computes hashes on the fly from disk +
  `state.json`, so the drift-vs-update distinction described by task 0015
  works without an asset-side hash store. Flagged in the plan; the user
  approved the simpler implementation.
- `LoadProfiles` returns the healthy subset plus a wrapping
  `errs.Errors(failures)` rather than bailing on the first broken profile. —
  **Why:** the TUI Profiles Screen needs to list every working profile even
  when one folder is corrupted; aggregating matches the project convention
  in `docs/guidelines/errors.md` §"Non-accumulator Functions Wrap In
  `errs.Errors`".
- `DeleteProfile` and `DeleteAsset` treat a pre-missing folder as success.
  — **Why:** the task description explicitly states "always succeed for the
  registry side even when the folder is gone"; symmetrical idempotence for
  `DeleteAsset` keeps the API consistent.
- `UpdateProject` does **not** re-check path-ownership invariants. —
  **Why:** editing a project never changes its path in the existing TUI
  flow; a future "move project" flow can call `ensureProjectPathAvailable`
  before `Save`.

Considered but rejected: adding a `Hashes map[string]string` field on
`asset.Asset` populated during `Load` and `UpdateAsset`. No consumer exists
today; YAGNI per `clean_code.md`.

## Assumptions

- Path uniqueness checks already enforced by `AddProject` do not need to be
  duplicated in `UpdateProject`. — **Why:** existing project records pass
  the check at creation time, and `UpdateProject` does not change the path.

## Other Notes

- No ADR. The methods stay inside existing architectural boundaries (see
  `docs/architecture/05-building-block-view.md` §`app`).
- No guideline changes. The implementation follows the existing
  conventions in `docs/guidelines/errors.md` and `domain_model.md`.
- Added unit-test files under `internal/app/`, `internal/registry/`, and
  `internal/project/` matching the existing typed-error assertion style.

## Service surface additions (`internal/app/service.go`)

```go
// before — only create-path operations
func (s *Service) CreateProfile(...) ...
func (s *Service) RegisterProfile(...) ...
func (s *Service) AddProject(...) ...
func (s *Service) InitAsset(...) ...
func (s *Service) Plan(...) ...
func (s *Service) Apply(...) ...
```

```go
// after — full CRUD; new methods compose existing primitives
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

## Registry helper

```go
// before — no removal path
func (s *Store) Add(ref ProfileRef) errs.DomainError { ... }
func (s *Store) Touch(profileID string) errs.DomainError { ... }
```

```go
// after — Remove drops the matching ref and saves; missing id is reported
func (s *Store) Remove(profileID string) errs.DomainError {
    reg, err := s.Load()
    if err != nil { return err }
    for i, existing := range reg.Profiles {
        if existing.ID == profileID {
            reg.Profiles = append(reg.Profiles[:i], reg.Profiles[i+1:]...)
            return s.Save(reg)
        }
    }
    return ProfileNotFoundError{Ref: profileID}
}
```

## Project helper

```go
// before — only Save existed
func Save(profileRoot string, manifest *Manifest) errs.DomainError { ... }
```

```go
// after — Delete is idempotent; pre-missing file is success
func Delete(profileRoot, projectID string) errs.DomainError {
    path := filepath.Join(profileRoot, config.ProjectsDirName, projectID+".json")
    if err := os.Remove(path); err != nil {
        if errors.Is(err, fs.ErrNotExist) { return nil }
        return ProjectDeleteError{Path: path, Err: err}
    }
    return nil
}
```

## New typed errors

- `app.ProfileFolderRemoveError{Path, Err}` — `os.RemoveAll` failure during
  `DeleteProfile(DeleteFolders)`.
- `app.AssetFolderRemoveError{Dir, Err}` — `os.RemoveAll` failure during
  `DeleteAsset`.
- `project.ProjectDeleteError{Path, Err}` — non-missing `os.Remove` failure
  inside `project.Delete`.

All three carry `Severity() = SeverityError` and `Unwrap()` to preserve the
underlying `os` error per `errors.md` §"Wrapping External Errors".

## Test additions

- `internal/registry/registry_test.go`: `TestRemove_DeletesProfileRef`,
  `TestRemove_MissingIDReturnsProfileNotFoundError`.
- `internal/project/project_test.go`: `TestDelete_RemovesManifestFile`,
  `TestDelete_MissingFileIsIdempotent`.
- `internal/app/service_load_profiles_test.go`: 2 tests.
- `internal/app/service_delete_profile_test.go`: 4 tests covering
  `KeepFolders`, `DeleteFolders`, missing-folder idempotence, unknown ref.
- `internal/app/service_asset_test.go`: 6 tests (`LoadAsset`,
  `UpdateAsset`, `DeleteAsset` happy paths plus error paths and the
  unselect-from-projects side effect).
- `internal/app/service_project_test.go`: 4 tests (`LoadProject`,
  `UpdateProject`, `DeleteProject` happy paths plus a `not-found` error
  path).

## Verification

```
make build && make test && make lint
```

All green: `go build`, `go test ./...`, `go vet ./...`.
