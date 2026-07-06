# 0036 changes

Split per-user project selections out of the profile aggregate so profile
folders become fully shareable. Introduces a new centralized user-config
directory `~/.agentfiles/` that houses both `profiles.json` (the renamed
registry) and `projects.json` (a new projects store keyed by profile id).
A one-shot v1→v2 migration runs at startup to move existing data across.

## Decisions

- **Group projects by profile id in `projects.json` (map keys), not by
  a `profile_id` field on each manifest.** — **Why:** the app always
  queries "give me the projects for profile X"; keying by id reflects
  that and keeps `project.Manifest` oblivious to which profile owns it.
- **Retain `Profile.Projects` as an in-memory projection populated at
  load time.** — **Why:** avoids a churn of every TUI screen and test
  that looks projects up by id; only the persistence path moves.
- **Presence-based migration detection; write-then-swap; log-not-rollback
  after v2 is durable.** — **Why:** matches the tool's existing
  idempotent-CLI shape and keeps the migration safe when interrupted
  mid-cleanup.
- **`projectstore.Load` returns a typed `OrphanProfileIDError` rather
  than silently pruning unknown profile groups.** — **Why:** the file is
  app-owned (not user-edited); an orphan is data corruption and must
  surface, not disappear.

Considered but rejected:

- Encrypting or gitignoring the leaky state inside the profile folder.
- Adding a `profile_id` field to every project manifest and leaving the
  file layout unchanged.
- Moving only the registry file and leaving `projects/` inside each
  profile.

## Assumptions

- Two concurrent first launches racing on the same fresh v1 layout is
  acceptable (single-user CLI shape; no file locking exists elsewhere).
  Documented in ADR 0017.
- The v1 registry filename (`.agentprofiles.json`) can stay hidden as a
  private constant inside `internal/migrate` for one release cycle
  before we remove v1 support entirely.

## Other Notes

Docs updated: ADR 0017 (new), arc42 blocks 03/05/06/07, glossary
(`User Config Dir`, `Projects Store`, `Registry`, `Profile`,
`Project Manifest`, `Managed State`), CLAUDE.md (package list +
invariant #5), `docs/guidelines/sync_and_safety.md` (new
"Enforce Single Project Ownership Globally" section).

## New `internal/projectstore` package

Mirrors `registry.Store`'s shape. Holds Load (with an orphan check against
the current registry), Save, typed CRUD (Add/Update/Remove/ListByProfile/
RemoveByProfile), and `AllProjects` for the global path-uniqueness check.

```go
// before — persistence was per-file inside each profile folder
project.Save(profileRoot, manifest)
project.Delete(profileRoot, projectID)
```

```go
// after — centralized store keyed by profile id
s.Projects.Add(profileID, manifest)
s.Projects.Update(profileID, manifest)
s.Projects.Remove(profileID, projectID)
```

## Trimmed `internal/project`

Reduced to `Manifest` + `NewDraft` + `SelectAsset` + `Validate` +
`Normalize`. `Save` and `Delete` moved to `projectstore`.

## Trimmed `internal/profile`

`Load` no longer walks `<root>/projects/`; `Init` no longer scaffolds it.
`Profile.Projects` is retained (allocated empty) so downstream by-id
lookups still work; the app layer composes projects via
`projectstore.ListByProfile` at load time. `Profile.UnselectAsset` now
returns the list of mutated project ids and is in-memory only; the
per-project persistence loop moved into `app.Service.DeleteAsset`.

```go
// before — profile persistence
func (l *Profile) UnselectAsset(assetID string) errs.DomainError {
    for _, p := range l.ProjectList() {
        p.SelectedAssetIDs = deleteAssetFromSelection(...)
        project.Save(l.Root, p) // persistence bleeds into profile pkg
    }
}
```

```go
// after — profile is disk-free for projects
func (l *Profile) UnselectAsset(assetID string) []string {
    var mutated []string
    for id, p := range l.Projects {
        if idx := slices.Index(p.SelectedAssetIDs, assetID); idx >= 0 {
            p.SelectedAssetIDs = slices.Delete(p.SelectedAssetIDs, idx, idx+1)
            mutated = append(mutated, id)
        }
    }
    return mutated
}
```

## Migration runner in `internal/migrate`

`Run(profileStore, projectStore, log)` invoked once from `cmd/af/main.go`
before the TUI opens. Detects the v1 layout via presence, harvests each
ref's `projects/` subdirectory, writes v2 files, then deletes the
originals. Idempotent by construction; stale profile paths are logged
without aborting.

## Rewired `app.Service`

`Service` now holds a `Projects *projectstore.Store` field.
`app.NewWithStores` is the injection seam used by main; `app.New(path)`
remains as a convenience for tests. Every persistence site
(`AddProject`, `UpdateProject`, `SelectAsset`, `UnselectAsset`,
`DeleteProject`, `CreateAssetFromFolder`, cascade in `DeleteProfile*`)
routes through the projects store. `ensureProjectPathAvailable` is a
single-pass check against `s.Projects.AllProjects()` instead of walking
every profile folder on disk.

```go
// before — cross-profile check walked filesystem
for _, profileRef := range reg.Profiles {
    loaded, _ := profile.Load(profileRef.Path)
    for _, proj := range loaded.ProjectList() { ... }
}
```

```go
// after — single pass over centralized store
owned, err := s.Projects.AllProjects()
for _, op := range owned {
    if op.Manifest.Path == projectPath { ... }
}
```

## `cmd/af/main.go`

Adds `--projects` flag alongside `--registry`. Builds both stores, calls
`migrate.Run(profileStore, projectStore, migrate.StderrLogger)` before
the TUI opens, then wires `app.NewWithStores(profileStore, projectStore)`.

## `internal/config/paths.go`

Removed `RegistryFileName` and `ProjectsDirName` from the public API.
Added three new constants:

```go
const UserConfigDirName     = ".agentfiles"
const ProfilesStoreFileName = "profiles.json"
const ProjectsStoreFileName = "projects.json"
```

The v1 registry filename lives on as a private constant inside
`internal/migrate`.
