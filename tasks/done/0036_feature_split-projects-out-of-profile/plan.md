# Plan — Task 0036: Split Projects Out Of Profile

- Task: [./description.md](./description.md)
- ADR to create: `docs/adr/0017-split-projects-out-of-profile.md`
- Related guidelines: `docs/guidelines/{clean_architecture,clean_code,domain_model,solid,testing,go,errors}.md`
- Sync/safety invariant: `CLAUDE.md` §Critical invariants #5

## Context

Today a profile folder owns both its `assets/` and its `projects/*.json`.
Sharing a profile therefore leaks the author's target-repo paths and
per-machine selections. Split so the folder stays reusable across users:

- Profile folder = `profile.json` + `assets/`.
- Per-user config = new `~/.agentfiles/` folder holding `profiles.json`
  (renamed registry) + `projects.json` (new store).

`Profile.Projects` in memory is retained as a convenience projection populated
by the app layer from `projectstore`. Downstream render/sync and TUI screens
keep their semantics.

Path uniqueness (one repo path across all profiles) is preserved but
implemented against the new centralized store, so it becomes a single-pass
check instead of walking every profile folder.

A one-shot v1→v2 migration runs at startup. Idempotent, write-then-swap, then
delete originals.

## Architecture Before / After

Before:
```
~/.agentprofiles.json                   (registry)
<profileRoot>/profile.json
<profileRoot>/assets/<type>/<id>/…
<profileRoot>/projects/<projectID>.json (leak point)
```

After:
```
~/.agentfiles/profiles.json             (registry, renamed)
~/.agentfiles/projects.json             (new: map[profileID][]Manifest)
<profileRoot>/profile.json              (unchanged)
<profileRoot>/assets/<type>/<id>/…      (unchanged)
```

`~/.agentfiles/` shares its name with the target-repo `<repo>/.agentfiles/`
state dir. Different anchors ($HOME vs repo root); glossary disambiguates.

## Package Layout After

- `internal/config` — add `UserConfigDirName`, `ProfilesStoreFileName`,
  `ProjectsStoreFileName`. Remove public `RegistryFileName`. (v1 name lives
  privately inside `internal/migrate`.)
- `internal/projectstore` — **new**. Owns `~/.agentfiles/projects.json`.
- `internal/migrate` — **new**. One-shot v1→v2 migration.
- `internal/project` — reduced to `Manifest`, `NewDraft`, `SelectAsset`,
  `Validate`, `Normalize`. `Save`/`Delete` removed.
- `internal/profile` — `Init` no longer scaffolds `projects/`; `Load` no
  longer walks it. `Profile.Projects` field retained.
- `internal/registry` — `DefaultPath` returns
  `~/.agentfiles/profiles.json`.
- `internal/app.Service` — gains a `*projectstore.Store`; project persist
  call sites route through it; `DeleteProfile*` cascades project removal;
  `ensureProjectPathAvailable` reads from projectstore.
- `cmd/af/main.go` — invokes `migrate.Run(profileStore, projectStore)`
  before the TUI starts.

## API Outlines

### `internal/config/paths.go`

```go
const UserConfigDirName     = ".agentfiles"
const ProfilesStoreFileName = "profiles.json"
const ProjectsStoreFileName = "projects.json"
```

Remove `RegistryFileName`, `ProjectsDirName`. `StateDirName` (target-repo)
stays.

### `internal/projectstore`

```go
package projectstore

type Store struct{ Path string }

type OwnedProject struct {
    ProfileID string
    Manifest  *project.Manifest
}

func DefaultPath() string                       // ~/.agentfiles/projects.json
func NewStore(path string) *Store

func (s *Store) Load(known map[string]struct{}) (map[string][]*project.Manifest, errs.DomainError)
func (s *Store) Save(state map[string][]*project.Manifest) errs.DomainError

func (s *Store) Add(profileID string, m *project.Manifest) errs.DomainError
func (s *Store) Update(profileID string, m *project.Manifest) errs.DomainError
func (s *Store) Remove(profileID, projectID string) errs.DomainError

func (s *Store) ListByProfile(profileID string) ([]*project.Manifest, errs.DomainError)
func (s *Store) RemoveByProfile(profileID string) errs.DomainError
func (s *Store) AllProjects() ([]OwnedProject, errs.DomainError)
```

`Load` takes the set of known profile ids from the registry. Any key in
`projects.json` that is not in `known` produces `OrphanProfileIDError`.
No silent pruning; app owns the file.

On-disk shape:
```json
{
  "version": 1,
  "projects": {
    "<profileID>": [ { "id": "...", "name": "...", "path": "...",
                       "enabled_agents": [...], "selected_asset_ids": [...],
                       "created_at": "..." }, ... ]
  }
}
```

### `internal/migrate`

```go
package migrate

const v1RegistryFileName = ".agentprofiles.json" // private

type Logger func(string)

func Run(profileStore *registry.Store, projectStore *projectstore.Store, log Logger) errs.DomainError
```

Behavior:
1. If either v2 file (`profileStore.Path` or `projectStore.Path`) exists →
   no-op.
2. Read v1 `~/.agentprofiles.json`. Missing → ensure `~/.agentfiles/`
   present and empty, done.
3. For each ref, walk `<ref.Path>/projects/*.json`. Missing folder →
   `log("stale profile path: <path>")`, keep ref. Collect into
   `map[string][]*project.Manifest`.
4. Ensure `~/.agentfiles/` exists (mkdir -p). Write `projects.json`, then
   `profiles.json`. Both must succeed before any destructive op.
5. Delete `~/.agentprofiles.json`; for each ref
   `os.RemoveAll(<ref.Path>/projects)`. Errors here go to `log`, not
   rollback (v2 is already durable).
6. Idempotent by construction: step 1.

### `internal/project` — trimmed

Kept: `Manifest`, `NewDraft`, `SelectAsset`, `Validate`, `Normalize`,
`ProjectFieldsRequiredError`, `NoEnabledAgentsError`.
Removed: `Save`, `Delete`, `ProjectDeleteError` (moves to `projectstore`
as `ProjectStoreDeleteError` if kept).

### `internal/profile` — trimmed

Removed: `loadProjectsInto`, `ProjectsReadDirError`, `DuplicateProjectIDError`
(moves to `projectstore` if reused). `Init` no longer creates `projects/`.
`Load` returns `Profile` with `Projects` allocated but empty. `Root`,
`Manifest`, `Assets`, `Projects` fields all retained.

`Profile.UnselectAsset(assetID)` becomes in-memory only: it mutates
`SelectedAssetIDs` on every project in the map. Persistence moves to the
caller (`app.Service.UnselectAsset`, which loops over the projectstore).

### `internal/registry`

```go
func DefaultPath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, config.UserConfigDirName, config.ProfilesStoreFileName)
}
```

`utils.WriteJSON` already creates parent dirs, so `~/.agentfiles/` gets
created on first `Save`.

### `internal/app.Service`

```go
type Service struct {
    Registry *registry.Store
    Projects *projectstore.Store
}

func NewWithStores(reg *registry.Store, proj *projectstore.Store) *Service
func New(registryPath string) *Service // thin wrapper for callers that don't inject
```

All project persist call sites migrated:

| Site                                   | Was                                   | Now                                       |
| -------------------------------------- | ------------------------------------- | ----------------------------------------- |
| `service.go:135` (`AddProject`)        | `project.Save(loaded.Root, manifest)` | `s.Projects.Add(profileID, manifest)`     |
| `service.go:232` (`CreateAssetFromFolder`) | `project.Save(loaded.Root, p)`    | `s.Projects.Update(profileID, p)`         |
| `service.go:760` (`UpdateProject`)     | `project.Save(loaded.Root, p)`        | `s.Projects.Update(profileID, p)`         |
| `service.go:778` (`SelectAsset`)       | `project.Save(loaded.Root, p)`        | `s.Projects.Update(profileID, p)`         |
| `service.go:809` (`UnselectAsset`)     | `project.Save(loaded.Root, p)`        | `s.Projects.Update(profileID, p)`         |
| `service.go:823` (`DeleteProject`)     | `project.Delete(loaded.Root, id)`     | `s.Projects.Remove(profileID, id)`        |
| `profile.go:188` (`Profile.UnselectAsset` loop) | `project.Save(l.Root, p)`    | move loop into `app`, use `s.Projects.Update` |

`loadWithProjects` private helper:
```go
func (s *Service) loadWithProjects(ref string) (*profile.Profile, errs.DomainError) {
    loaded, err := s.LoadProfile(ref) // existing method
    if err != nil { return nil, err }
    ps, err := s.Projects.ListByProfile(loaded.Manifest.ID)
    if err != nil { return nil, err }
    for _, p := range ps { loaded.Projects[p.ID] = p }
    return loaded, nil
}
```

Every method that today does `s.LoadProfile(ref)` and then reads
`loaded.Projects` switches to `s.loadWithProjects(ref)`.

`ensureProjectPathAvailable`:
```go
func (s *Service) ensureProjectPathAvailable(projectPath string, active *profile.Profile) []errs.DomainError {
    owned, err := s.Projects.AllProjects()
    if err != nil { return []errs.DomainError{err} }
    var conflicts []errs.DomainError
    for _, op := range owned {
        if op.Manifest.Path != projectPath { continue }
        conflicts = append(conflicts, ProjectPathOwnedError{
            Path:        projectPath,
            ProfileName: s.resolveProfileName(op.ProfileID),
            ProjectName: op.Manifest.Name,
        })
    }
    return conflicts
}
```

`DeleteProfile` + `DeleteProfileWithFolder` cascade:
1. `s.Projects.RemoveByProfile(id)`.
2. `s.Registry.Remove(id)`.
3. (folder variant) `os.RemoveAll(profilePath)`.

Two-step confirm in TUI (`shell/profiles.go`) unchanged.

### `cmd/af/main.go`

```go
registryPath := flag.String("registry", registry.DefaultPath(), "path to profile registry")
projectsPath := flag.String("projects", projectstore.DefaultPath(), "path to project store")
themePath   := flag.String("theme", "", "...")
flag.Parse()

if err := styles.LoadConfig(*themePath); err != nil { ... }

profileStore := registry.NewStore(*registryPath)
projectStore := projectstore.NewStore(*projectsPath)

if err := migrate.Run(profileStore, projectStore, migrate.StderrLogger); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}

svc := app.NewWithStores(profileStore, projectStore)
a := actions.New(svc)
log := notifications.NewLog()

if _, err := tea.NewProgram(shell.New(a, log)).Run(); err != nil { ... }
```

## Step-By-Step Execution

Each step should leave the tree buildable. Commit boundaries suggested in
parentheses.

1. **Config paths** (`internal/config/paths.go`): add three constants;
   remove `RegistryFileName` and `ProjectsDirName`. Update
   `internal/registry/registry.go` `DefaultPath` to use the new
   constants. (`step 1: config paths + registry default`)

2. **`internal/projectstore` package**: create files:
   - `store.go` — `Store`, `OwnedProject`, `DefaultPath`, `NewStore`,
     `Load`, `Save`, `Add`, `Update`, `Remove`, `ListByProfile`,
     `RemoveByProfile`, `AllProjects`.
   - `errors.go` — `OrphanProfileIDError`, `ProjectStoreReadError`,
     `ProjectNotFoundError` (if not reused), `DuplicateProjectIDError`.
   - `store_test.go` — round-trip; CRUD; `RemoveByProfile` cascade;
     `AllProjects` deterministic ordering; orphan detection.
   (`step 2: introduce projectstore`)

3. **`internal/project` trimming**: delete `Save`, `Delete`, and
   `ProjectDeleteError` from `errors.go`. Adjust tests. (`step 3: trim project`)

4. **`internal/profile` trimming**: drop `loadProjectsInto` and the
   `Load` call to it; drop `projects/` scaffolding in `Init`; drop
   `ProjectsReadDirError`; rewrite `Profile.UnselectAsset` to be
   in-memory-only. Update tests. (`step 4: trim profile`)

5. **`internal/migrate` package**: create files:
   - `migrate.go` — `Run`, `Logger`, `StderrLogger`.
   - `errors.go` — typed errors (e.g. `MigrateWriteError`,
     `MigrateReadV1Error`).
   - `migrate_test.go` — v1-present happy path (v2 written, originals
     deleted, idempotent); v2-present no-op; v1 write failure; stale
     profile path warning captured. Use a `t.TempDir()` faux `$HOME`
     and inject a captured logger.
   (`step 5: migration runner`)

6. **`internal/app.Service` rewiring**:
   - Add `Projects *projectstore.Store` field, `NewWithStores`, wrapper
     `New`.
   - Add private `loadWithProjects`.
   - Rewrite `AddProject`, `CreateAssetFromFolder`, `UpdateProject`,
     `SelectAsset`, `UnselectAsset`, `DeleteProject`, `resolveProject`,
     `planSync` call sites to route through `s.Projects.*` and
     `loadWithProjects`.
   - Rewrite `ensureProjectPathAvailable` to use `s.Projects.AllProjects()`.
   - Cascade in `DeleteProfile` + `DeleteProfileWithFolder`.
   - Update `service_test.go` fixtures.
   (`step 6: route service through projectstore`)

7. **`cmd/af/main.go`**: build both stores, run `migrate.Run`, call
   `app.NewWithStores`. (`step 7: wire migration in main`)

8. **TUI shell/test fixtures**: touch only the fixtures — `fakeLoadedProfile`
   in `internal/tui/shell/edit_profile_test.go` and
   `newSelectActionsFake` in `internal/tui/shell/select_project_assets_test.go`.
   Keep the direct seeding of `prof.Projects[id]` (field survives); adjust
   any actions fake so it can accept a projectstore or a hand-written
   fake for the cascade paths. (`step 8: TUI test fixtures`)

9. **Docs**:
    - **ADR 0017** — new file `docs/adr/0017-split-projects-out-of-profile.md`
      matching ADR 0016's shape: Status (accepted), Context, Decision,
      Consequences, Notes.
    - **`docs/architecture/05-building-block-view.md`** — rewrite
      `registry`, `profile`, `project` bullets; add bullets for
      `projectstore` and `migrate`.
    - **`docs/architecture/06-runtime-view.md`** — add a top note about
      the startup migration.
    - **`docs/architecture/03-context-and-scope.md`** — Filesystem section
      mentions `~/.agentfiles/` user-config dir.
    - **`docs/architecture/07-deployment-view.md`** — replace
      `~/.agentprofiles.json` with `~/.agentfiles/profiles.json`; add
      `~/.agentfiles/projects.json`.
    - **`docs/glossary.md`** — refine "Profile" (no `projects/`), refine
      "Registry" (new path), refine "Managed State" (add note about the
      two `.agentfiles` folders having different anchors), add
      "User Config Dir", "Projects Store".
    - **`docs/guidelines/sync_and_safety.md`** — reword single-ownership
      mention if it appears (invariant lives in `CLAUDE.md`; guideline
      may not need a change).
    - **`CLAUDE.md`** — update `Package layout` (add `projectstore`,
      `migrate`; refine `profile` and `project`); rewrite invariant #5.
   (`step 9: docs`)

10. **Changelog** — write
    `docs/changelog/2026-07-06_0036-split-projects-out-of-profile.md`
    from the skill template.
   (`step 10: changelog`)

## Testing Strategy

Follow `testing.md` (unit first, behavior-focused, `t.TempDir()`). Tests
belong to the package under test.

- `projectstore`: table-driven CRUD; `RemoveByProfile` cascade; orphan
  detection; `AllProjects` ordering.
- `migrate`: fixture-driven with a faux `$HOME` via explicit paths and an
  injected logger:
  - v1 present → v2 written, originals deleted, idempotent second run.
  - v2 present → no-op (v1 untouched).
  - v1 write failure → v2 files partially absent, originals intact.
  - Stale profile path → warning captured, ref retained.
- `profile`: `Init` no longer creates `projects/`; `Load` returns
  `Profile` with empty `Projects` map (allocated, len 0).
- `project`: `Save`/`Delete` tests deleted; validation + normalization
  tests kept.
- `app.Service`:
  - `AddProject` → manifest present in projectstore under the profile id.
  - `DeleteProject` → manifest gone.
  - `DeleteProfile` → all projects for that id gone; other profiles
    untouched.
  - `ensureProjectPathAvailable` catches cross-profile conflicts with
    only projectstore populated.
- TUI shell tests keep asserting via `prof.Projects[id]` (field
  retained); update fixture wiring only.

## Verification

Baseline gate:
```
make build && make test && make lint
```

Behavior smoke:
- Rename real `~/.agentprofiles.json` aside; run `./bin/af`; empty
  registry loads; create profile + project; quit; re-run; both persist
  under `~/.agentfiles/`.
- In a scratch `HOME` (`env HOME=$(mktemp -d) ./bin/af`) seed a v1
  layout; first run migrates; second run is a no-op; `~/.agentfiles/`
  contains both files; v1 originals gone.

## Migration Risks / Callouts

- **HOME resolution.** `registry.DefaultPath` swallows `os.UserHomeDir()`
  errors today. Migrate step returns a typed error on empty home so
  startup fails loudly.
- **Concurrent runs.** No file locking; ADR notes it as accepted.
- **Orphan handling on first launch after migration.** A fresh
  migration cannot produce orphans (only reachable refs contribute); a
  hand-edited `projects.json` can. Test that explicitly.

## Out Of Scope (mirrors description)

- Remote profile distribution.
- Renaming `<repo>/.agentfiles/`.
- render/sync semantic changes.
