# Split Projects Out Of Profile

## Status

accepted

## Context

Since ADR 0001 the **profile folder** has been the single source of truth
for a user's AI-assistant configuration. In the v1 layout the folder
contained both shareable material — `profile.json` and `assets/` — and
per-machine material — a `projects/` subdirectory of `<project-id>.json`
manifests recording each target repository's absolute path, enabled
agents, and selected asset ids.

The shareable and per-machine parts had lived under the same root because
early adoption was single-user. As the project matured the tension
surfaced:

- Two authors distributing the same profile folder inadvertently ship
  each other's local repo paths, personal agent lists, and asset
  selections.
- Adopting somebody else's profile requires editing the received
  `projects/` subdirectory to remove their state before use.
- CLAUDE.md invariant #5 (one repo path per profile) was enforced by
  walking every registered profile's `projects/` directory on every
  path-availability check — an O(profiles × projects) filesystem scan
  that couples the check to disk layout.

The v1 registry (`~/.agentprofiles.json`) also lived directly under
`$HOME`, making the split a good moment to also collect user-level
agentfiles state under a single well-known dot-directory.

Alternatives considered:

- **Encrypt or gitignore per-machine state inside the profile folder.**
  Rejected: sharing over anything other than a working tree (email, tar,
  copy-paste) has no notion of `.gitignore`; encryption pushes work onto
  users for a leak the tool can prevent structurally.
- **Add a `profile_id` field to every project manifest and leave the
  file layout unchanged.** Rejected: the leak still happens; the change
  is invisible to the aggregate boundary.
- **Move only the registry, leave `projects/` inside each profile.**
  Rejected: the leak concern is per-project, not per-registry — moving
  the registry alone would not solve the problem this ADR addresses.

## Decision

Split the profile aggregate. A profile folder now contains only
shareable material; per-user selections move to a new centralized
user-config directory.

### On-disk layout after

```
~/.agentfiles/profiles.json      (was ~/.agentprofiles.json)
~/.agentfiles/projects.json      (new)
<profileRoot>/profile.json       (unchanged)
<profileRoot>/assets/…           (unchanged)
```

- `~/.agentfiles/` is the **user-config dir**. It shares its name with
  the target-repo managed-state dir (also `.agentfiles/`) deliberately —
  the two live under different anchors ($HOME vs repo root). The
  glossary disambiguates.
- `~/.agentfiles/profiles.json` replaces `~/.agentprofiles.json`. The
  legacy filename lives only as a private constant inside the migration
  package.
- `~/.agentfiles/projects.json` is a new **projects store**: a JSON
  object keyed by `profile_id` whose values are arrays of project
  manifests. Keying by profile id (rather than a flat list with a
  `profile_id` field on each manifest) reflects how the app queries and
  keeps `project.Manifest` oblivious to which profile owns it.

### Package layout after

- New `internal/projectstore` — owns `~/.agentfiles/projects.json`
  (`Load`, `Save`, `Add`, `Update`, `Remove`, `ListByProfile`,
  `RemoveByProfile`, `AllProjects`). Mirrors `registry.Store`'s shape so
  the two centralized files are read and written the same way.
- New `internal/migrate` — one-shot v1→v2 migration invoked from
  `cmd/af/main.go` before the TUI opens. Idempotent by construction.
- `internal/project` shrinks to `Manifest` + `NewDraft` + `SelectAsset`
  + `Validate` + `Normalize`. Save/Delete move to `projectstore`.
- `internal/profile` no longer walks or scaffolds `projects/`.
  `Profile.Projects` (the map field) is retained as a downstream
  convenience projection populated by `app.Service.LoadProfile` from the
  projects store.
- `internal/registry.DefaultPath` returns
  `~/.agentfiles/profiles.json`. Old constants
  `config.RegistryFileName` and `config.ProjectsDirName` are removed.
- `internal/app.Service` gains a `Projects *projectstore.Store` field
  plus a `NewWithStores` injector. Every persistence site (`AddProject`,
  `UpdateProject`, `SelectAsset`, `UnselectAsset`, `DeleteProject`,
  `CreateAssetFromFolder`, cascade in `DeleteProfile*`) routes through
  the projects store. Path-uniqueness (invariant #5) becomes a single
  `s.Projects.AllProjects()` pass instead of an O(profiles × projects)
  filesystem walk.

### Aggregate boundaries after the split

The domain rationale, which the on-disk layout only expresses, is that
shareable assets and per-user selections form two distinct consistency
boundaries and must not be updated as one transaction:

- **Profile aggregate.** Root: `Profile{Manifest, Assets}`. Invariants
  it protects: asset ids are unique within one profile; asset content is
  authoritative (repo files are outputs); every asset in `Assets` was
  loaded from the profile folder. The Profile aggregate no longer owns
  project selections — that leak was the whole reason for the split.
- **Projects Store aggregate.** Root:
  `ProjectsStore{profileID → []project.Manifest}`. Invariants it
  protects: one target repo path belongs to at most one project across
  every registered profile (the reworded CLAUDE.md invariant #5); every
  group's `profileID` corresponds to a registered profile (orphan groups
  surface as `OrphanProfileIDError` on `Load`); project ids are unique
  within one group. The store owns these rules directly: `Add`/`Update`
  return `ProjectPathOwnedError` when a foreign group already claims the
  path.

`app.Service.LoadProfile` composes the two aggregates for downstream
callers by returning `app.LoadedProfile{Profile, Projects}`. That
composition is a read-time convenience; writes still route through the
owning aggregate (asset changes into the profile folder, project
changes into the projects store), so no code mutates one aggregate
inside another's boundary.

### Migration (v1 → v2)

Runs once at startup:

1. **Detect.** If either `~/.agentfiles/profiles.json` or
   `~/.agentfiles/projects.json` exists, skip.
2. **Read v1 registry.** Missing → nothing to do.
3. **Harvest.** For each ref, walk `<ref.Path>/projects/*.json`.
   Missing folder → warning to the injected logger, keep the ref.
4. **Write-then-swap.** Compute the full v2 state in memory, then write
   `projects.json` followed by `profiles.json`. Both must succeed
   before touching v1.
5. **Delete originals.** Remove `~/.agentprofiles.json`; for each ref
   remove `<ref.Path>/projects/`. Failures here go to the logger, not a
   rollback — v2 is already durable.
6. **Idempotency.** Subsequent runs see v2 present and skip at step 1.

### Concurrency and locking

The migration does not lock — no file locking exists elsewhere in the
tool. Two concurrent `af` invocations against the same fresh v1 layout
would both attempt migration; the winner writes v2 and the loser sees
v2 present next time. This is accepted for the single-user CLI shape.

### Orphan handling

`projectstore.Load` treats a group whose key is not in the current
registry as `OrphanProfileIDError`. The file is app-owned (not
user-edited), so an orphan is data corruption and must surface — not be
silently pruned.

## Consequences

Positive:

- Profile folders are shareable without leaking per-machine state.
- The user-config dir concentrates persistent CLI state under one
  well-known root.
- Path-uniqueness now costs one JSON read instead of a walk over every
  registered profile folder.
- The migration is idempotent, presence-based, and injectable (a test
  logger captures warnings without touching stderr), so it is safe on
  every startup.

Negative / accepted:

- The upgrade is not silent: on first launch after this change, the
  user sees migration warnings for any stale registry entry and their
  registry file location changes. This is the price of the split.
- The v1 registry filename (`.agentprofiles.json`) lives on as a
  private constant inside `internal/migrate` until we drop v1 support
  in a future release. Removing it prematurely would strand users who
  skip a version.
- Two concurrent first launches race with no lock. Accepted for a
  single-user CLI; if we later introduce multi-process scenarios, a
  file lock or a lockless CAS on the v2 write can be added inside
  `migrate.Run` without changing callers.

## Notes

- Related invariants: CLAUDE.md §Critical invariants #5 is reworded to
  point at the projects store as the single ownership check.
- Related guidelines: `docs/guidelines/sync_and_safety.md` cross-links
  the new store from the ownership rule.
- Runtime view: `docs/architecture/06-runtime-view.md` picks up a
  startup migration bullet.
- Building-block view: `docs/architecture/05-building-block-view.md`
  adds `projectstore` and `migrate` and refines `profile`, `project`,
  `registry`.
