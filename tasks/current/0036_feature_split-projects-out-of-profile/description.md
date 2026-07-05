---
id: 0036
type: feature
status: pending
topics: go
---

# Split projects out of profile

Make `Profile` shareable. Today a profile folder owns both its assets *and*
its `projects/*.json` files. Sharing a profile leaks the original author's
target-repo paths and per-machine selections. Split: profile folder keeps
assets only; per-user project selections move into a new centralized
user-config folder, leaving the profile folder content reusable across users.

## Background

- `~/.agentprofiles.json` is the v1 registry of `ProfileRef` records.
- Each profile folder under `<root>/projects/` holds per-project manifests
  (target repo path, enabled agents, selected asset ids).
- `profile.Load` populates `Profile.Projects` by walking `<root>/projects/`.
- CLAUDE.md invariant #5 enforces one repo path per profile.

After the split:
- Profile folder = `profile.json` + `assets/` only. No `projects/` dir.
- User-config = a centralized `~/.agentfiles/` folder housing both
  `profiles.json` and `projects.json`.

## Decisions

1. **User-config dir:** `~/.agentfiles/`. Collision with the target-repo
   managed-state dir (also `.agentfiles/`) is accepted — different anchors
   (home vs repo root). Glossary disambiguates.
2. **Files:**
    - `~/.agentfiles/profiles.json` (was `~/.agentprofiles.json`).
    - `~/.agentfiles/projects.json` (new).
3. **`projects.json` shape:** map keyed by `profile_id` → array of project
   manifests. No `profile_id` field on `project.Manifest`; map key is the
   grouping.
4. **Repo-path uniqueness:** stays globally unique across all
   profiles+projects. Render/sync ownership stays unambiguous.
5. **Package layout:**
    - New `internal/projectstore` — owns `projects.json` (Load, Save, Add,
      Remove, ListByProfile, RemoveByProfile). Mirrors `registry.Store`
      shape.
    - `internal/project` shrinks to `Manifest` + `Validate` + `Normalize`
      (no Save/Delete tied to a profile root).
    - New `internal/migrate` — versioned migration runner. Called once
      from `cmd/af/main.go` before TUI starts.
    - `internal/registry` continues to own profile discovery; file path
      moves to `~/.agentfiles/profiles.json`.
    - `internal/profile`: `Profile.Projects` map retained. `profile.Load`
      no longer fills it; app layer composes (profile + projects) at load
      time. `profile.Init` no longer scaffolds `projects/` dir.
6. **Cascade on profile removal:** TUI confirm prompt, then
   `profilestore.Remove(id)` + `projectstore.RemoveByProfile(id)`.
7. **Orphans on `projectstore.Load`:** if a `projects.json` entry's
   `profile_id` has no matching profile, return a typed `DomainError`.
   `projects.json` is owned by the app, not user-edited; an orphan is
   data corruption and must surface, not be silently pruned.

## Migration (v1 → v2)

Runs once at startup from `cmd/af/main.go` via `migrate.Run`.

1. **Detect.** If `~/.agentfiles/profiles.json` exists, skip migration.
2. **Harvest.**
    - Read `~/.agentprofiles.json` → in-memory profile refs.
    - For each ref, walk `<ref.Path>/projects/*.json`. Collect into a
      `map[profileID][]project.Manifest`. Stale `ref.Path` (folder
      missing) → skip with a startup-log warning, keep the ref.
3. **Write-then-swap.** Build full v2 state in memory. Create
   `~/.agentfiles/`. Write `profiles.json` + `projects.json`. Both must
   succeed before any destructive op.
4. **Delete originals.**
    - Remove `~/.agentprofiles.json`.
    - Remove each `<ref.Path>/projects/` dir.
    - Partial failure here is logged but does not roll back — v2 is
      already durable.
5. **Idempotency.** Subsequent runs see `profiles.json` present and skip.

## New constants (`internal/config/paths.go`)

```go
const UserConfigDirName     = ".agentfiles"
const ProfilesStoreFileName = "profiles.json"
const ProjectsStoreFileName = "projects.json"
```

`RegistryFileName` is dropped from the public API. The v1 path
(`~/.agentprofiles.json`) lives only as a private const inside
`internal/migrate`.

## API outline

```go
// internal/projectstore
type Store struct{ Path string }

func DefaultPath() string                                                // ~/.agentfiles/projects.json
func NewStore(path string) *Store
func (s *Store) Load() (map[string][]*project.Manifest, errs.DomainError)
func (s *Store) Save(state map[string][]*project.Manifest) errs.DomainError
func (s *Store) Add(profileID string, m *project.Manifest) errs.DomainError
func (s *Store) Update(profileID string, m *project.Manifest) errs.DomainError
func (s *Store) Remove(profileID, projectID string) errs.DomainError
func (s *Store) ListByProfile(profileID string) ([]*project.Manifest, errs.DomainError)
func (s *Store) RemoveByProfile(profileID string) errs.DomainError

// internal/migrate
func Run(profileStore *registry.Store, projectStore *projectstore.Store) errs.DomainError
```

`app.Service` constructor gains a `*projectstore.Store`. Service helper
(or a small `app.LoadProfileWithProjects`) calls `profile.Load` then
`projectstore.ListByProfile(id)` and attaches the result to
`Profile.Projects`.

## Touch points

- `internal/profile/profile.go`: drop `loadProjectsInto`, drop `projects/`
  scaffolding in `Init`. Keep `Profile.Projects` field.
- `internal/project/project.go`: drop `Save(profileRoot,...)` and
  `Delete(profileRoot,...)`; move equivalents to `projectstore`.
- `internal/app/service.go`: every `project.Save(loaded.Root, ...)` and
  `project.Delete(loaded.Root, ...)` call now routes through
  `projectstore`. Cascade behavior added to `DeleteProfile`.
- `internal/registry/registry.go`: file path constant change.
- `cmd/af/main.go`: invoke `migrate.Run` before opening TUI.
- Tests: `profile`, `project`, `app`, `actions`, `tui/shell` fixtures that
  seed `<root>/projects/*.json` switch to seeding a `projectstore`.

## Docs

- New ADR in `docs/adr/` recording the split + migration approach.
- Update arc42 block view + runtime view (`docs/architecture/`) to reflect
  the new user-config aggregate and the removal of Projects from the
  Profile aggregate.
- Update `docs/glossary.md`: add "user-config dir", "projects store",
  refine "Profile" (no longer owns Projects), refine
  "managed-state dir" with disambiguation note.
- Update top-level `CLAUDE.md`: package list (add `projectstore`,
  `migrate`; refine `profile`), invariants section (#5 single-ownership
  rewording).
- Update `docs/guidelines/sync_and_safety.md` for the rewritten
  single-ownership invariant.

## Tests

- `projectstore`: load/save round-trip; add/update/remove; cascade
  `RemoveByProfile`; orphan profile_id surfaces typed error on `Load`.
- `migrate`: v1 detected → v2 written, originals deleted; v2 present →
  no-op; v1 write failure leaves originals intact; stale profile path
  skipped with warning.
- `profile`: `Init` no longer creates `projects/`; `Load` returns Profile
  with empty `Projects` map.
- `app.Service`: project CRUD routes through projectstore; `DeleteProfile`
  cascades; repo-path uniqueness still global.
- TUI shell tests that today inject `prof.Projects[id] = p` keep working
  (field retained), but fixtures construct via service composition.

## Out of scope

- Profile distribution / discovery from remote sources (registry `Source`
  remains `"local"` only).
- Renaming the target-repo managed-state dir.
- Any change to render/sync semantics.

## Verification

```
make build && make test && make lint
./bin/af   # first launch after migration: registry + projects load; create + edit projects round-trip
```

## Plan

[plan.md](./plan.md)
