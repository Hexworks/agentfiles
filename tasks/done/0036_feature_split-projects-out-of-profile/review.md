# Task 0036 — Split projects out of profile — review

Seven parallel reviewers (security, clean code, clean architecture, SOLID, DDD, testing, Go) surveyed the diff. Every acceptance criterion in `description.md` is met and `make build && make test && make lint` succeeds. The most load-bearing findings cluster around three themes:

1. **Profile pretends to own projects it no longer owns on disk.** `Profile.Projects` is still on the aggregate, `Profile.UnselectAsset` still mutates it, and `app.Service` has to loop the mutated ids back through `projectstore.Update`. Multiple reviewers flagged this as leaky (arch), split-brain (SOLID), and a cross-aggregate mutation (DDD). Related: the map-iteration-order return of `UnselectAsset` is nondeterministic.
2. **Projects Store aggregate is missing its headline invariant.** The whole reason the store exists (per ADR 0017) is to enforce single-ownership of a repo path across profiles, but `Store.Add`/`Update` do not enforce it — `app.Service.ensureProjectPathAvailable` does. `registry.Store` enforces its own uniqueness in `Add`; the two aggregates were meant to mirror each other and they don't.
3. **Security hardening on the new user-config surface.** The `~/.agentfiles/` dir and its two JSON files are created with `0o755`/`0o644` (world-readable). The migration also trusts `ref.Path` from the v1 registry unvalidated, and `DeleteProfileWithFolder` compares raw path strings without `EvalSymlinks`.

Beyond those, findings cover: missing failure-path tests in `migrate`, doc drift in `docs/architecture/05-building-block-view.md`, two constructors on `app.Service` that duplicate wiring, and a handful of low-priority clean-code items (error-message shape, `Load`/`loadRaw` duplication, `Logger` severity).

## Profile still owns projects it no longer owns

> [!WARNING]
>
> - [Clean Architecture — Stable Dependencies](docs/guidelines/clean_architecture.md)
> - [Domain Model — Model Consistency Boundaries](docs/guidelines/domain_model.md)
> - [SOLID — Single Responsibility Principle](docs/guidelines/solid.md)

ADR 0017's Decision opens with "Split the profile aggregate", but the `Profile` struct still carries `Projects map[string]*project.Manifest` (`internal/profile/profile.go:45`) and still exposes `UnselectAsset` and `ProjectList` methods that mutate it. `profile.Load` allocates the map empty and returns; `app.Service.LoadProfile` (`service.go:130-136`) hydrates it after the fact. Any caller reaching `profile.Load` directly — including `app.Service.LoadProfiles` at `service.go:586-593` — sees the same aggregate populated or empty depending on which entry point they used. The doc comment on the field even admits this: "the map is retained (rather than replaced by a slice) so downstream callers that look projects up by id keep their existing API" — retained for API compatibility, not domain reasons.

`Profile.UnselectAsset` (`profile.go:149-160`) mutates the map and returns the list of mutated project ids; `app.Service.DeleteAsset` (`service.go:782-800`) then loops those ids back through `s.Projects.Update`. The "unselect on delete" invariant lives across two aggregates coupled by temporal ordering.

```go
// internal/profile/profile.go:41-46 — Profile still owns the field
type Profile struct {
    Root     string
    Manifest Manifest
    Assets   map[string]*asset.Asset
    Projects map[string]*project.Manifest // owned on disk by projectstore, mutated here
}

// internal/app/service.go:787-793 — mutation on one aggregate, persistence on the other
for _, projectID := range loaded.UnselectAsset(assetID) {
    if saveErr := s.Projects.Update(loaded.Manifest.ID, loaded.Projects[projectID]); saveErr != nil {
        failures = append(failures, saveErr)
    }
}
```

Pick one:

- [x] Drop `Profile.Projects` and `Profile.UnselectAsset`. Return a small `app.LoadedProfile{Profile, Projects}` from `app.Service.LoadProfile`. Move the unselect loop into `app.Service.DeleteAsset` — iterate `s.Projects.ListByProfile(...)`, mutate each manifest, `Projects.Update` in one pass.
- [ ] Keep `Profile.Projects` but move the composition inward: give `profile.Load` a `ProjectsProvider` interface it calls to fill the map (implemented by `projectstore.Store`). Restores the "loaded Profile has consistent Projects" invariant without leaking projectstore into profile.
- [ ] Accept the current shape, rename the field to `ProjectsView` (or add a `Loaded bool`), remove `Profile.UnselectAsset`, and move the mutation loop into `app.Service` so the profile package never writes into a foreign aggregate.

## Projects Store does not enforce the invariant that justifies its existence

> [!WARNING]
>
> - [Domain Model — Put Domain Rules In Domain Code](docs/guidelines/domain_model.md)
> - [Clean Architecture — Common Closure Principle](docs/guidelines/clean_architecture.md)

The glossary defines _Project Ownership_ as "one target project path may belong to at most one project across every registered profile" and ADR 0017 promotes the projects store to a first-class aggregate root. Yet `projectstore.Store.Add` (`store.go:144-162`) checks only for id-uniqueness within the same profile group; two `Add(profileA, m)` / `Add(profileB, m')` calls with the same `Path` succeed silently. `registry.Store.Add` (`registry.go:97-115`) does enforce its own uniqueness rules. The two centralized stores were meant to mirror each other and don't.

Enforcement lives at `app.Service.ensureProjectPathAvailable` (`service.go:523-545`). If someone writes directly to the store (a hand-edited `projects.json`, a future migration, a test helper) the invariant is skipped.

```go
// internal/projectstore/store.go:144
func (s *Store) Add(profileID string, m *project.Manifest) errs.DomainError {
    // ... Normalize + Validate + within-group duplicate id check ...
    // No cross-group check that m.Path is not already owned.
}
```

Pick one:

- [x] Move repo-path uniqueness into `projectstore.Store.Add`/`Update`. Return a typed `ProjectPathOwnedError{Path, ExistingProfileID, ExistingProjectID}` from the store; `app.Service` translates the profile id into a human name for the TUI. [ ] Add `projectstore.Store.AssertPathAvailable(path, excludingProjectID)` as a named method so the invariant lives in the aggregate's vocabulary even if `app.Service` still triggers it. Keeps the current call graph but makes the rule discoverable inside the package.
- [ ] Accept `app`-level enforcement, but add a doc comment on `projectstore.Store` (and a Consequence in ADR 0017) explicitly stating the projects store has no cross-group invariant so the disparity with `registry.Store.Add` is deliberate.

## `~/.agentfiles/` created world-readable

> [!WARNING]
>
> - [Security — Protect Secrets And Local State](docs/guidelines/security.md)

`utils.WriteJSON` (`internal/utils/fs.go:36,75`) unconditionally uses `os.MkdirAll(..., 0o755)` and `os.WriteFile(..., 0o644)`. `~/.agentfiles/profiles.json` holds absolute profile paths (machine-identifying data) and `~/.agentfiles/projects.json` holds every user-selected repo path. On any multi-user host — shared build server, CI runner, container with sidecars — any local user can read both files.

```go
// internal/utils/fs.go
if err := os.MkdirAll(path, 0o755); err != nil { ... }  // world-executable
if err := os.WriteFile(path, data, 0o644); err != nil { ... }  // world-readable
```

Pick one:

- [x] Add `utils.WriteJSONMode(path, v, dirMode, fileMode)`; have `registry.Store.Save` and `projectstore.Store.Save` (and `migrate.Run`) request `0o700`/`0o600`. Repo-projected files keep `0o755`/`0o644`.
- [ ] Add a shared `EnsureUserConfigDir` helper called from `registry.NewStore`/`projectstore.NewStore`/`migrate.Run` that creates `~/.agentfiles/` with `0o700` on first touch; keep the file mode at `0o644` (still less exposed than the dir).
- [ ] Accept the current permissions and document explicitly in ADR 0017 (Consequences) that agentfiles state is world-readable on multi-user hosts.

## Migration trusts `ref.Path` from v1 registry without validation

> [!WARNING]
>
> - [Security — Treat External Input As Untrusted](docs/guidelines/security.md)

`readV1Registry` (`internal/migrate/migrate.go:132`) decodes `~/.agentprofiles.json` into `registry.ProfileRef` and passes the parsed `ref.Path` straight to `filepath.Join(ref.Path, v1ProjectsDirName)` and then `os.ReadDir` / `os.RemoveAll`. No `filepath.Clean`, no `filepath.IsAbs` check, no symlink `lstat`, no rejection of empty `ref.Path` (which would produce `projects`, resolved against the process CWD). The v1 file is user-editable — a maliciously (or accidentally) hand-edited registry pointing at `/tmp/something` triggers a live `os.ReadDir` and, after harvest, an `os.RemoveAll` on `/tmp/something/projects`.

```go
// internal/migrate/migrate.go:84-88 and 108-111
projectsDir := filepath.Join(ref.Path, v1ProjectsDirName) // untrusted
if !utils.Exists(projectsDir) { log(...); continue }      // no shape check
// later:
if rmErr := os.RemoveAll(projectsDir); rmErr != nil { ... } // follows into wherever
```

Pick one:

- [x] Validate `ref.Path` inside `Run` before use: reject empty; require `filepath.IsAbs`; `filepath.Clean`; `lstat` and skip when it is a symlink or not a directory. Skip (with warning) rather than fail the whole run.
- [ ] Reuse `app.Service.isUnsafeProfilePath` (or hoist it to a shared `internal/paths` helper) and gate every ref before harvest/cleanup.
- [ ] Accept the risk (v1 registry was already user-editable in the previous release) and document in ADR 0017 that migration inherits the trust boundary of the v1 file.

## `DeleteProfileWithFolder` safety check compares raw path strings

> [!WARNING]
>
> - [Security — Keep File Access Inside Intended Roots](docs/guidelines/security.md)

`Service.isUnsafeProfilePath` (`service.go:665-681`) blocks empty, `/`, `.`, `$HOME`, and ancestors of the registry file, but operates on the raw `ref.Path` string. It does not `EvalSymlinks`: a registry entry `~/agentfiles-profile` where that symlinks to `$HOME` slips through because `filepath.Clean` leaves symlinks alone. `os.RemoveAll(ref.Path)` (`service.go:651`) then removes the symlink (fine), but the intent — "refuse to remove `$HOME`" — is bypassable by symlink for anything else.

```go
// internal/app/service.go:673 — literal-string comparison, symlinks slip through
if home, _ := os.UserHomeDir(); home != "" && clean == filepath.Clean(home) { ... }
```

Pick one:

- [ ] `resolved, err := filepath.EvalSymlinks(clean)`; on success run every check against `resolved`. If `EvalSymlinks` fails, fail closed (`UnsafeProfilePathError{Reason: "unresolvable path"}`).
- [x] Require `ref.Path` to sit under an allow-listed root (a `profiles/` dir configured per user) so deletion can never escape into arbitrary user territory even if the string check passes.
- [ ] Accept the current shape (registry file is app-owned) and add a doc comment explaining the trust boundary.

## Migration writes are not atomic (crash window)

> [!WARNING]
>
> - [Security — Keep File Access Inside Intended Roots](docs/guidelines/security.md)

`migrate.Run`'s package doc says "only after they are durable does Run touch the v1 originals" (`migrate.go:12`). In practice `utils.WriteJSON` calls `os.WriteFile` and returns — no `f.Sync`, no directory fsync, no atomic rename. A power loss or OOM-kill between the `WriteFile` return and `os.Remove(v1Path)` (`migrate.go:104`) can leave `~/.agentfiles/profiles.json` as a partial write, at which point the presence-based gate at `migrate.go:68` fires next launch and Run skips — the user is stuck with a corrupt v2 file and a still-present v1.

Same concern in a mid-migration failure: `projects.json` is written before `profiles.json` (`migrate.go:97-103`); if the second write fails, the presence check on the _next_ launch still trips because `projects.json` exists — v1 is stranded, never re-migrated.

Pick one:

- [x] Add `utils.WriteJSONAtomic` that writes to `path+".tmp"`, `f.Sync`, `os.Rename`, and fsync the parent directory. Use inside `migrate.Run` for both v2 files.
- [ ] Change the presence gate to require _both_ v2 files (`profileStore.Path` **and** `projectStore.Path`) — a lone `projects.json` is treated as incomplete and re-migrated.
- [ ] Accept the risk (documented single-user CLI, unlikely to hit a crash between two millisecond-apart writes) and add a Consequence to ADR 0017.

## `Profile.UnselectAsset` returns map-iteration-order slice

> [!WARNING]
>
> - [Errors — Accumulator functions require deterministic order](docs/guidelines/errors.md)
> - [Clean Code — Avoid hidden ordering requirements](docs/guidelines/clean_code.md)

`Profile.UnselectAsset` (`internal/profile/profile.go:149-160`) appends mutated project ids into a slice in map iteration order. Go randomizes map iteration. `app.Service.DeleteAsset` (`service.go:787-792`) drives a `Projects.Update` loop over that slice; a mid-loop failure surfaces a different subset of `errs.Errors` on each attempt and the on-disk `state.json` writes happen in an unstable order.

The errors guideline explicitly requires deterministic order for accumulator-shaped functions.

```go
// internal/profile/profile.go:151
for id, p := range l.Projects { // map iteration randomized
    ...
    mutated = append(mutated, id)
}
return mutated
```

Pick one:

- [ ] Collect keys, `slices.Sort` them, iterate the sorted slice.
- [x] Iterate `l.ProjectList()` (already sorted by name) and use `p.ID` as the mutated id.
- [ ] If this issue is resolved by moving the loop into `app.Service` per the "Profile still owns projects" finding, this becomes moot — pick that solution and delete this method.

## Two constructors on `app.Service` duplicate wiring

> [!WARNING]
>
> - [Clean Architecture — Wiring Belongs Near The Edge](docs/guidelines/clean_architecture.md)
> - [SOLID — Dependency Inversion Principle](docs/guidelines/solid.md)

`internal/app/service.go:49-53` defines `New(registryPath string)` which derives the sibling projects-store path from the registry directory. `service.go:59-61` defines `NewWithStores(reg, proj)` which is the real DI seam. `cmd/af/main.go:43` uses `NewWithStores`; every `New` call is in tests. `New` reaches into `filepath.Dir(reg.Path)` to reassemble a path that `projectstore.DefaultPath` already knows how to compute — duplicated filesystem-layout knowledge in the stable app layer.

```go
// internal/app/service.go:49-53
func New(registryPath string) *Service {
    reg := registry.NewStore(registryPath)
    projectsPath := filepath.Join(filepath.Dir(reg.Path), config.ProjectsStoreFileName)
    return NewWithStores(reg, projectstore.NewStore(projectsPath))
}
```

Pick one:

- [x] Delete `New`. Every test creates both stores explicitly (they already know `t.TempDir()`). Rename `NewWithStores` to `New`. One constructor, real DI, no filesystem-layout policy in the app layer.
- [ ] Move `New` to a test helper (e.g. `app.NewForTest(root string) *Service` in `service_test.go`) so the convenience is available to tests without polluting the app API.
- [ ] Keep both but implement `New` via a hoisted `projectstore.SiblingOf(registryPath)` so the sibling-path rule lives in `projectstore`, not `app`.

## Migration tests miss the failure paths that justify write-then-swap

> [!WARNING]
>
> - [Testing — Test One Behavior At A Time](docs/guidelines/testing.md)

Acceptance criterion "v1 write failure leaves originals intact" is not covered. `internal/migrate/migrate_test.go:1-227` has no test that simulates a v2 write failure (unwritable target, disk full sim) and asserts `~/.agentprofiles.json` and each `<ref.Path>/projects/` folder survive. Similarly, no test covers the "projectStore succeeded, profileStore failed" ordering hazard called out in the atomic-writes finding: the next launch's presence check trips even though only one v2 file exists, stranding v1. Finally, no test seeds a corrupt v1 manifest and asserts the migration fails loudly instead of silently dropping user data.

Pick one:

- [x] Add three tests: `TestRun_ProjectStoreSaveFailureLeavesV1Intact`, `TestRun_ProfileStoreSaveFailureAfterProjectStoreSucceededLeavesRecoverable`, `TestRun_MalformedV1ManifestFailsLoudly`. First two use a chmod'd target dir; the third writes invalid JSON and asserts `errors.As` on the wrapped `utils.ReadJSONError`.
- [ ] Add only `TestRun_ProjectStoreSaveFailureLeavesV1Intact` (the direct acceptance criterion) and defer the other two.
- [ ] Accept the current coverage as sufficient and note in ADR 0017 that these failure modes are "code-inspected, not test-enforced".

## `projectstore` test coverage has gaps beyond the failure paths

> [!WARNING]
>
> - [Testing — Test One Behavior At A Time](docs/guidelines/testing.md)

Three lower-impact gaps in `internal/projectstore/store_test.go`:

- **`Save` sort-by-name is not directly asserted.** `TestListByProfile_SortedByName` proves `ListByProfile` sorts, but `ListByProfile` also sorts on read (`store.go:227-229`) — if someone deletes the sort in `Save` (`store.go:115-118`) no current test fails.
- **`AllProjects` single-profile intra-group ordering is not tested.** `TestAllProjects_SortedDeterministic` covers multi-profile only; a regression to insertion-order for a single-group user would silently break the majority case.
- **Migration logger is asserted via `strings.Contains` only.** `TestRun_StaleProfilePathSkippedWithWarning` (`migrate_test.go:174-207`) only substring-matches — the format "stale profile path skipped: `<path>`" could regress to "skipping stale" without any test failure.

Pick one:

- [x] Add `TestSave_WritesProjectsSortedByName`, `TestAllProjects_SingleProfileSortedByName`, and tighten the stale-path assertion to `strings.HasPrefix(msg, "stale profile path skipped: ")` + `strings.Contains(msg, "no-such-dir")`.
- [ ] Only tighten the stale-path assertion (highest signal-to-effort ratio) and leave the sort/single-profile gaps as-is.
- [ ] Accept the current coverage.

## `Load`/`loadRaw` duplication and orphan-check bypass

> [!WARNING]
>
> - [Clean Code — Needless repetition](docs/guidelines/clean_code.md)
> - [Security — Treat External Input As Untrusted](docs/guidelines/security.md)

`internal/projectstore/store.go:75-106` (`Load`) and `store.go:127-139` (`loadRaw`) share the same file-existence check + `ReadJSON` + `nil-map` fixup; the only difference is orphan validation. Every internal CRUD path (`Add`, `Update`, `Remove`, `ListByProfile`, `RemoveByProfile`, `AllProjects`) goes through `loadRaw`, which means the orphan check runs _only_ on the app's initial `Load`. A hand-edited `projects.json` with a group keyed under a not-yet-registered profile id will not surface as `OrphanProfileIDError` on CRUD; the entries just live there quietly until they matter (e.g. `AllProjects` returns them and `ensureProjectPathAvailable` uses them to block a legitimate `AddProject`).

```go
// internal/projectstore/store.go:127-139 — loadRaw skips orphan check
func (s *Store) loadRaw() (map[string][]*project.Manifest, errs.DomainError) {
    // ... same as Load without the orphan pass
}
```

Pick one:

- [x] Have `Load` delegate to `loadRaw` and add the orphan/normalize/validate pass on top — kills the duplication. Separately, add a required `known map[string]struct{}` to every CRUD method (or store `known` on the `Store` at construction), so the orphan check runs on every read.
- [ ] Extract a private `readState()` primitive both use; leave orphan-checking on `Load` only, but add a doc note that CRUD paths skip it by design.
- [ ] Accept the split as-is; orphans in a hand-edited file are user error and out of scope.

## `05-building-block-view.md` diagram shows edges that don't exist

> [!WARNING]
>
> - [Clean Architecture — document current reality](docs/guidelines/clean_architecture.md)

The mermaid diagram at `docs/architecture/05-building-block-view.md` shows `app --> migrate`, but `internal/app` does not import `internal/migrate` — the migration runner is invoked from `cmd/af/main.go`. The correct edges are `cmdaf --> migrate` and `cmdaf --> projectstore`. `go list -deps` would surface the mismatch immediately.

Pick one:

- [x] Remove `app --> migrate` edge; add `cmdaf --> migrate` and `cmdaf --> projectstore` edges.
- [ ] Keep `app --> migrate` and rewrite the surrounding paragraph to say "app coordinates with the migration runner via main" — inaccurate but honest about the drift.

## Migration `Run` is closed only to v1→v2 and mixes phases

> [!WARNING]
>
> - [SOLID — Open/Closed Principle](docs/guidelines/solid.md)
> - [Clean Code — Functions do one job at one level](docs/guidelines/clean_code.md)

`migrate.Run` (`internal/migrate/migrate.go:64-114`) interleaves gating checks, harvest, two persistence calls, and best-effort cleanup in a single ~50-line body. The write-then-swap invariant lives implicitly in statement order. A future v3 migration cannot be added without editing `Run` itself and reasoning about ordering against the existing v1→v2 block.

Pick one:

- [x] Extract `alreadyMigrated`, `harvestV1(refs, log)`, `writeV2(projectsByProfile, refs)`, `cleanupV1(v1Path, refs, log)`; `Run` becomes 6–8 lines. Adding a v3 becomes a sibling file with the same shape.
- [ ] Extract only `cleanupV1` — the best-effort tail — so the boundary "after this line everything is best-effort" is explicit; leave the rest of `Run` inline.
- [ ] Accept the one-shot design and drop the "future migrations" framing from the package doc so readers do not expect extensibility that is not there.

## `Migrate.Logger` erases severity

> [!WARNING]
>
> - [Errors — Severity Is A Domain Concern](docs/guidelines/errors.md)
> - [Go — Return Actionable Errors](docs/guidelines/go.md)

`migrate.Logger` is `func(string)`. The three log sites in `Run` surface qualitatively different events — "stale profile path skipped" is operator-visible warning-shape; the two "could not remove v1 …" lines are best-effort informational. All three land through the same `StderrLogger` prefix. Tests can only substring-match; downstream tools cannot filter by level.

Pick one:

- [x] Change to `Logger func(errs.Severity, string)` and pass `errs.SeverityWarning` for stale-path skips vs. `errs.SeverityInfo` for cleanup failures. Update `StderrLogger` and the test harness.
- [ ] Introduce a small `type Event struct { Kind, Message string; Err error }`; `Logger func(Event)`; `StderrLogger` translates to string. Richer contract, richer tests.
- [ ] Accept the current shape and add a doc comment on `Logger` documenting the intended level per call site.

## `migrate` reaches into `registry.Registry` value shape for v1 decoding

> [!WARNING]
>
> - [Clean Architecture — Stable Abstractions](docs/guidelines/clean_architecture.md)

`readV1Registry` (`internal/migrate/migrate.go:132`) unmarshals the v1 file into `registry.Registry` and `registry.ProfileRef`. That works only because v1 and v2 happen to share the same struct shape; the moment `registry.ProfileRef` gains a v2-only field the migration silently re-encodes v1 records as v2 without a mapping step. ADR 0017 promises that "the v1 filename lives only as a private constant inside migrate" but the v1 _schema_ has no such isolation.

Pick one:

- [x] Define private `v1Registry` / `v1ProfileRef` structs inside `internal/migrate`; decode into them; map field-by-field into `registry.Registry` on write. The seam makes future divergence explicit and testable.
- [ ] Add a doc comment on `registry.ProfileRef` stating that `internal/migrate` uses this struct as a v1 decoder and any field change must include a v1 fixture test. Cheaper but relies on humans reading the comment.
- [ ] Accept the coupling and document it in ADR 0017 Consequences.

## `projectstore.Store.Load` returns raw map instead of `State`

> [!WARNING]
>
> - [Go — Prefer explicit types over loose maps](docs/guidelines/go.md)

`projectstore.Store.Load` returns `map[string][]*project.Manifest` (`store.go:75`) while `Save` takes the same map (`store.go:112`). The package already has a `State` type with `Version` and `Projects`. `registry.Store.Load` returns `*Registry` (the wrapper); the two stores were said to mirror each other and diverge on this signature. Any future schema-version handling has nowhere to live.

Pick one:

- [x] Return `*State` from `Load` and accept `*State` in `Save`; mirror `registry.Store` exactly. Update `migrate.Run` accordingly.
- [ ] Rename `Load` to `LoadProjects` (or `LoadMap`) so the type mismatch with `registry.Store.Load` is at least honest at the name level.
- [ ] Accept the raw-map shape and drop `State.Version` from the public API to remove the illusion of versioning.

## `DefaultPath` swallows `os.UserHomeDir` error

> [!WARNING]
>
> - [Go — Return Actionable Errors](docs/guidelines/go.md)

`projectstore.DefaultPath` (`store.go:52-55`) and `registry.DefaultPath` (`registry.go:52-55`) discard the error from `os.UserHomeDir()` and return a path relative to an empty `home`. Failure produces `.agentfiles/projects.json` — a CWD-relative path — instead of surfacing the environment problem. `migrate.v1RegistryPath` (`migrate.go:120-126`) does the right thing (returns `("", false)` on error). This task copied the anti-pattern from `registry.DefaultPath` verbatim rather than fixing it.

Pick one:

- [x] Change both `DefaultPath` funcs to `(string, error)`; `cmd/af/main.go` already exits on init errors.
- [ ] Panic on empty home in `DefaultPath` — matches the "no home means we cannot proceed" reality.
- [ ] Accept as-is; document the CWD-relative fallback on both `DefaultPath` doc comments.

## CLAUDE.md `project` bullet under-describes the trimmed API

> [!WARNING]
>
> - [Clean Architecture — Common Closure Principle](docs/guidelines/clean_architecture.md)

`CLAUDE.md:35` reads: "`project` — per-project manifest struct (target path + selected agents + selected asset ids) plus `Validate`/`Normalize`." It omits `NewDraft` and `SelectAsset`, both public methods on the trimmed package (`internal/project/project.go:35` and `:49`) and part of the API the app layer calls. The plan explicitly listed both as part of the trimmed surface.

Pick one:

- [x] Update the `project` bullet in CLAUDE.md to "…plus `NewDraft`, `SelectAsset`, `Validate`, `Normalize`."
- [ ] Leave CLAUDE.md and update `docs/architecture/05-building-block-view.md` instead (also incomplete on the same package).

## ADR 0017 does not name the aggregates after the split

> [!WARNING]
>
> - [Domain Model — Keep Boundaries Clear](docs/guidelines/domain_model.md)

ADR 0017 Decision opens with "Split the profile aggregate" but spends most of its text on filesystem layout. The domain rationale — why per-user selections form a different consistency boundary than shareable assets — is buried in Context and never restated as "these are the two aggregates and here is the invariant each protects". Same bias in `docs/glossary.md`: the _Projects Store_ entry describes it as "the centralized project selection file" (a file, not an aggregate) and _Profile_ still calls the folder "the shareable, authoritative source of asset content" without naming the aggregate-root shift. There is also no glossary entry for `migrate` or `User Config Dir`-level migration terminology.

Pick one:

- [x] Add an "Aggregate boundaries after the split" subsection to ADR 0017 naming both aggregates, their roots (`Profile{Manifest, Assets}` and `ProjectsStore{profileID → []Manifest}`), and the invariants each protects. Rewrite the `Projects Store` and `Profile` glossary entries to lead with the aggregate framing. Add a `## Migration` glossary entry.
- [ ] Just add the glossary entries and skip the ADR subsection; the ADR body already carries the ideas even if not named.
- [ ] Accept the current documentation shape.

## Store persistence conflates validation with I/O

> [!WARNING]
>
> - [SOLID — Single Responsibility Principle](docs/guidelines/solid.md)

`projectstore.Store` describes itself as owning "loading and saving" but its `Load`, `Add`, and `Update` methods also invoke `project.Manifest.Normalize`/`Validate`. When a caller sends an already-normalized in-memory manifest through `Add` (which is the case for `app.Service.AddProject`, since `project.NewDraft` already normalizes), the store re-runs the same rules. And on `Load`, a v1-migrated manifest that was valid at write time but fails a newer `Validate` becomes a fatal startup error rather than a UI-level repair prompt.

Pick one:

- [ ] Keep validation only at the layer that owns the _change_ (`app.Service` before `Add`/`Update`); let `Load` return raw manifests plus a separate `Verify(known)` step callers can invoke.
- [ ] Leave the current shape but rename the store's doc comment to acknowledge it owns "persistence + invariant re-check" so the contract is explicit.
- [x] Introduce a `projectstore.Validator` seam so `Store` remains a pure persistence type and the invariant lives with the domain.

---

Read the review, tick exactly one checkbox per section for the solution you want applied. When ready, run `af.task.review-apply 0036` in a fresh session.
