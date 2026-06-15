# Service CRUD methods review

Reviewed commit `0f80e8c` against `docs/guidelines/security.md`, `clean_code.md`,
`clean_architecture.md`, `solid.md`, `domain_model.md`, `testing.md`, `go.md`,
`errors.md`, and ADR 0008. The implementation matches the plan and the
changelog; the typed-error catalogue is correctly extended; build and tests
ship green. Findings cluster around three themes: (a) the destructive
operations (`DeleteProfile`, `DeleteAsset`) accept too much trust from the
registry / caller and leave the system in half-applied states on partial
failure; (b) cross-aggregate work that belongs in `profile` / `asset` lives in
`app`; (c) the new error catalogue and the multi-project unselect loop are
under-tested.

Tick exactly one solution per issue, then return so the fixes can be applied.

## DeleteAsset has no atomicity across project saves

> [!WARNING]
>
> - [`docs/guidelines/errors.md` — Accumulator Functions Return `[]errs.DomainError`](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/domain_model.md` — Model Consistency Boundaries](../../../docs/guidelines/domain_model.md)
> - [`docs/guidelines/sync_and_safety.md` — Always Plan Before Apply](../../../docs/guidelines/sync_and_safety.md)

`Service.DeleteAsset` iterates `loaded.ProjectList()` and calls `project.Save`
inside the loop. If the third save fails, projects 1 and 2 are already
persisted with `assetID` stripped, the rest still reference it, and the asset
folder still exists on disk. The caller gets one typed error but the profile
is now in a torn state with no audit trail. The errors guideline tells loops
that "want to surface every failure at once" to accumulate into a slice; this
loop does the opposite — short-circuits on first failure.

```go
// internal/app/service.go:404-417
for _, p := range loaded.ProjectList() {
    idx := slices.Index(p.SelectedAssetIDs, assetID)
    if idx < 0 {
        continue
    }
    p.SelectedAssetIDs = slices.Delete(p.SelectedAssetIDs, idx, idx+1)
    if saveErr := project.Save(loaded.Root, p); saveErr != nil {
        return saveErr // earlier projects already mutated on disk; folder still present
    }
}
if rmErr := os.RemoveAll(target.Dir); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
    return AssetFolderRemoveError{Dir: target.Dir, Err: rmErr}
}
```

- [x] Accumulate per-project save errors with `errors.Join` (or build an
      `errs.Errors` slice), keep going through every project, then attempt
      `os.RemoveAll`. Return the aggregated typed error so re-running the
      method converges idempotently.
- [ ] Keep short-circuit semantics but document the half-applied invariant
      in the godoc above `DeleteAsset` and add a test that proves
      re-running `DeleteAsset` after a failure cleans up the remainder.

## Cross-aggregate orchestration in `DeleteAsset` belongs in `profile`

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md` — Keep policy at the inward side](../../../docs/guidelines/clean_architecture.md)
> - [`docs/guidelines/domain_model.md` — Put Domain Rules In Domain Code, Keep Application Flow Thin](../../../docs/guidelines/domain_model.md)
> - [`docs/architecture/05-building-block-view.md` — `asset` owns asset I/O](../../../docs/architecture/05-building-block-view.md)

`DeleteAsset` encodes the profile-level invariant "if an asset is removed,
every project's `SelectedAssetIDs` must drop the id" inside `app`. The
docstring of `Service` says it is a "thin application layer" that "contains
no business logic of its own." This is business logic. It is also asymmetric:
`DeleteProject` correctly delegates to `project.Delete`, but `DeleteAsset`
performs `utils.WriteJSON` to write the asset manifest (`UpdateAsset`,
`service.go:389`) and `os.RemoveAll` on the asset folder (`service.go:414`)
directly — reaching past the `asset` package boundary that `asset.Init` /
`asset.Load` already own.

```go
// internal/app/service.go:395-418 — domain rule + I/O both in app
for _, p := range loaded.ProjectList() {
    idx := slices.Index(p.SelectedAssetIDs, assetID)
    if idx < 0 { continue }
    p.SelectedAssetIDs = slices.Delete(p.SelectedAssetIDs, idx, idx+1)
    if saveErr := project.Save(loaded.Root, p); saveErr != nil { return saveErr }
}
if rmErr := os.RemoveAll(target.Dir); ...
```

- [x] Move "unselect everywhere" into `profile.UnselectAsset(loaded *Profile, assetID string) errs.DomainError`
      (accumulating with `errors.Join`) and add `asset.Delete(dir)` (idempotent
      on `fs.ErrNotExist`) symmetric to `project.Delete`. `Service.DeleteAsset`
      becomes orchestration: load → `profile.UnselectAsset` → `asset.Delete`.
      Move `AssetFolderRemoveError` into `internal/asset/errors.go`.
- [ ] Keep the orchestration in `app` but extract two private helpers in
      `service.go` (`unselectAssetFromProjects`, `removeAssetFolder`) so each
      method does one job at one level of abstraction.
- [ ] Leave as-is — accept the boundary tension; `app` keeps the rule.

## UpdateAsset trusts caller-controlled `a.Dir`

> [!WARNING]
>
> - [`docs/guidelines/security.md` — Keep File Access Inside Intended Roots](../../../docs/guidelines/security.md)

`Service.UpdateAsset` writes the manifest to `filepath.Join(a.Dir, config.AssetManifestFileName)`
without comparing `a.Dir` against the loaded asset's authoritative directory.
`Asset.Dir` is not part of the serialized manifest; it is a runtime field set
by `profile.Load` (`asset.go:117-120`). A caller that constructs
`&asset.Asset{Manifest: Manifest{ID: "agents", …}, Dir: "/etc"}` — perfectly
legal Go — will write `/etc/asset.json`. The only check is `a.Manifest.Validate()`,
which never inspects `Dir`.

```go
// internal/app/service.go:375-390
if loaded.Assets[a.ID] == nil {           // id check only
    return AssetNotFoundError{AssetID: a.ID}
}
if validateErr := a.Manifest.Validate(); validateErr != nil {
    return validateErr
}
return utils.WriteJSON(filepath.Join(a.Dir, config.AssetManifestFileName), a.Manifest)
//                                  ^^^^^ caller-controlled, never compared to loaded.Assets[a.ID].Dir
```

- [x] Resolve the destination from the trusted side: `dir := loaded.Assets[a.ID].Dir`;
      ignore the caller's `a.Dir`. Change the signature so it cannot be supplied
      at all (take `*asset.Manifest` instead of `*asset.Asset`).
- [ ] Keep the pointer but validate `a.Dir == loaded.Assets[a.ID].Dir`; return
      a typed `AssetDirMismatchError` otherwise.

## DeleteProfile(DeleteFolders) does not verify `ref.Path` is a profile root

> [!WARNING]
>
> - [`docs/guidelines/security.md` — Keep File Access Inside Intended Roots; Treat External Input As Untrusted](../../../docs/guidelines/security.md)

`os.RemoveAll(ref.Path)` runs against whatever path the registry holds. The
registry is JSON in `~/.agentprofiles.json`; if it is tampered with — or if a
future `RegisterProfile` bug stores an unsanitized path — calling
`DeleteProfile(name, DeleteFolders)` becomes a recursive wipe of an arbitrary
directory (root, `$HOME`, anything). The current code rejects only
`fs.ErrNotExist`. No shape check, no "this still looks like a profile root"
check, no rejection of unsafe ancestors.

```go
// internal/app/service.go:334-349
if err := s.Registry.Remove(ref.ID); err != nil { return err }
if folderAction != DeleteFolders { return nil }
if rmErr := os.RemoveAll(ref.Path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
    return ProfileFolderRemoveError{Path: ref.Path, Err: rmErr}
}
```

- [x] Before `os.RemoveAll`, require `utils.Exists(filepath.Join(ref.Path, config.ProfileManifestFileName))`;
      abort with a new typed `ProfileFolderNotARootError` otherwise. Cheap,
      catches the registry-tamper and the path-typo cases.
- [x] Additionally reject empty path, `/`, `$HOME`, and any ancestor of the
      registry file's directory; surface as `UnsafeProfilePathError`.
- [ ] Skip — accept that registry tampering is out of scope for the threat
      model.

## Recursive removal does not guard against symlinks

> [!WARNING]
>
> - [`docs/guidelines/security.md` — symlinks/unexpected files are safety-sensitive](../../../docs/guidelines/security.md)

`os.RemoveAll` on the asset directory (`service.go:414`) and the profile
folder (`service.go:345`) recursively descends and deletes. Go's
`os.RemoveAll` does not follow symbolic links — it unlinks the link itself —
but the project has no test that proves this invariant for the two new
destructive paths, and the security guideline explicitly asks for "test path
traversal and root-escape rejection".

- [x] Add a regression test that creates a profile / asset with a symlink
      pointing outside the root, then asserts deletion removes the link but
      leaves the target untouched. No production change needed if the test
      passes.
- [ ] Walk the directory with `filepath.WalkDir`, refuse to delete if any
      entry's `Lstat` reports `os.ModeSymlink`; typed `ContainsSymlinkError`.
      Strongest stance; rejects legitimate symlinks too.
- [ ] Skip — Go's documented behaviour is sufficient.

## FolderAction is a flag argument

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md` — Functions: avoid flag arguments](../../../docs/guidelines/clean_code.md)
> - [`docs/guidelines/solid.md` — Open/Closed Principle](../../../docs/guidelines/solid.md)
> - [`docs/guidelines/domain_model.md` — Keep Application Flow Thin](../../../docs/guidelines/domain_model.md)

`DeleteProfile(ref string, folderAction FolderAction)` runs registry removal
unconditionally and then switches on the flag. That is the canonical
"flag argument that switches between sub-behaviors" smell — half the body
always runs, half is conditional, and the two intents read as
`DeleteProfile(name, KeepFolders)` vs `DeleteProfile(name, DeleteFolders)`
where the choice is the actual user intent. The domain-model guideline
flags the same thing from a different angle: "does the user want the folder
gone?" is a TUI confirmation outcome leaking into the application API. The
asymmetry is visible — `DeleteAsset` always removes its folder, `DeleteProject`
never touches the repo, only `DeleteProfile` asks.

- [x] Split into `DeleteProfile(ref)` (registry-only) and
      `DeleteProfileWithFolder(ref)` (registry + folder); drop the
      `FolderAction` type and the asymmetry vs `DeleteAsset` / `DeleteProject`
      becomes a one-line table in the package doc.
- [ ] Keep the enum; it self-documents at the call site
      (`svc.DeleteProfile(ref, app.DeleteFolders)`). Rename the method to
      `RemoveProfile` so the dual-mode shape is visible in the name.

## Nil-pointer guard returns "not found" with an empty id

> [!WARNING]
>
> - [`docs/guidelines/errors.md` — Accumulator exception 2: programming bugs fail fast](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/clean_code.md` — Naming: make meaningful distinctions](../../../docs/guidelines/clean_code.md)

`UpdateAsset(nil)` and `UpdateProject(nil)` return
`AssetNotFoundError{AssetID: ""}` and `ProjectNotFoundError{ProjectID: ""}`.
The errors guideline carves programming bugs out of the typed-error contract.
Worse, the TUI's `RenderError` will print `unknown asset:` with no id, which
is opaque — the user cannot tell "asset id typo" from "nil pointer leaked
from a UI form".

```go
// service.go:375-378 and :437-440
if a == nil { return AssetNotFoundError{AssetID: ""} }
if p == nil { return ProjectNotFoundError{ProjectID: ""} }
```

- [ ] Drop the nil checks. Nil dereference on `a.ID` / `p.ID` is the correct
      fail-fast for a programmer bug.
- [x] Replace with `panic("nil asset")` / `panic("nil project")` so the
      failure mode is explicit at the panic site.
- [ ] Change the signatures to take values instead of pointers
      (`a asset.Asset`, `p project.Manifest`); nil becomes unrepresentable.

## DeleteProfile order leaves an orphan folder on RemoveAll failure

> [!WARNING]
>
> - [`docs/guidelines/errors.md` — Return Actionable Errors](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/domain_model.md` — Model Consistency Boundaries](../../../docs/guidelines/domain_model.md)

`DeleteProfile` removes the registry entry first; if `os.RemoveAll` then
fails (permission denied, in-use handle), the registry no longer points at
the folder but the folder lingers — and the user only sees the path in the
error message, with no way to discover it again through the registry. The
changelog explicitly justifies the "registry side always succeeds when the
folder is gone" case, but is silent on the "folder-remove fails" case.

- [x] Reverse the order: `os.RemoveAll` first (treating `fs.ErrNotExist` as
      success), then `Registry.Remove`. A failed folder-remove keeps the
      registry entry so the user can retry from the TUI.
- [ ] Keep current order; document the orphan-folder possibility in the
      `DeleteProfile` godoc so callers understand they may need to clean up
      manually on failure.

## LoadProfiles returns a partial slice + error hybrid that no caller consumes

> [!WARNING]
>
> - [`docs/guidelines/errors.md` — Accumulator vs Non-accumulator](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/clean_code.md` — Code Smells: Opacity](../../../docs/guidelines/clean_code.md)

`LoadProfiles` is an accumulator (walks a collection, has independent
failures) but returns the project's non-accumulator shape (single
`errs.Errors`-wrapped value). It also returns a _partial_ success slice
alongside the error — callers must check `err != nil` and _also_ use
`loaded`. No production caller does both today; the TUI either succeeds or
aborts. The combination has no precedent elsewhere in the codebase.

```go
// service.go:309-328
if len(loadErrs) > 0 {
    return loaded, errs.Errors(loadErrs)  // partial + error
}
return loaded, nil
```

- [x] Change the signature to `([]*profile.Profile, []errs.DomainError)` —
      matches the accumulator pattern used by `render.Build` and
      `app.AddProject`. The TUI can list all healthy profiles and all
      failures explicitly.
- [ ] Keep the single-error signature but drop the partial slice on error:
      `if len(loadErrs) > 0 { return nil, errs.Errors(loadErrs) }`. Simpler
      contract; gives up the partial-recovery story.
- [ ] Leave as-is and document the hybrid contract in the godoc so future
      callers know to use both return values.

## Repeated `LoadProfile → map lookup → not-found` prelude across six methods

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md` — Code Smells: Needless repetition](../../../docs/guidelines/clean_code.md)

`LoadAsset`, `UpdateAsset`, `DeleteAsset`, `LoadProject`, `UpdateProject`,
`DeleteProject` share the same five-line prelude. If the resolution rule
changes (e.g. case-insensitive lookup, additional side-effect on miss), six
sites need to stay in sync.

```go
loaded, err := s.LoadProfile(profileRef)
if err != nil { return ..., err }
x := loaded.Assets[id]               // or loaded.Projects[id]
if x == nil { return ..., XNotFoundError{...} }
```

- [x] Add private helpers `resolveAsset(profileRef, assetID) (*profile.Profile, *asset.Asset, errs.DomainError)`
      and `resolveProject(...)` so each public method becomes load → act → return.
- [ ] Leave as-is — the duplication is shallow and each public method reads
      top-to-bottom without indirection.

## `folderAction != DeleteFolders` accepts unknown values as Keep

> [!WARNING]
>
> - [`docs/guidelines/go.md` — Prefer Explicit Types](../../../docs/guidelines/go.md)

```go
// service.go:342
if folderAction != DeleteFolders {
    return nil
}
```

With only two values today the `!=` form is equivalent to `== KeepFolders`,
but it silently accepts any future variant (e.g. a hypothetical
`DeleteFoldersWithBackup`) as Keep — the wrong default.

- [x] Switch to `if folderAction == KeepFolders { return nil }` so a new
      variant forces a compile-time decision via a `switch`.
- [ ] Convert to `switch folderAction` with an explicit default; new variants
      surface during code review even without exhaustiveness checks.
- [ ] Leave as-is — only two cases will ever exist.

## Tests miss typed-error failure paths

> [!WARNING]
>
> - [`docs/guidelines/errors.md` — Tests: assert on typed errors](../../../docs/guidelines/errors.md)
> - [`docs/guidelines/testing.md` — Assert Behavior, Not Mock Mechanics](../../../docs/guidelines/testing.md)

Three new typed errors exist precisely to signal real I/O failures:
`ProfileFolderRemoveError` (`app/errors.go:75`), `AssetFolderRemoveError`
(`app/errors.go:95`), `ProjectDeleteError` (`project/errors.go:42`). Only the
`fs.ErrNotExist` branch is tested for each; the typed wrapping branch is
never reached. The error-guideline rule "assert on typed errors, not on
`err.Error()` substrings" is observed in spirit, but the typed errors
themselves have zero `errors.As` coverage.

Additionally, `TestLoadProfiles_AggregatesPerProfileLoadErrors`
(`service_load_profiles_test.go:39-67`) only asserts `len(leaves) != 0`,
which would pass even if the slice held an anonymous `fmt.Errorf`. The
guideline specifically calls for `errors.As` on the concrete leaf.

- [x] Add failure-injection tests for all three typed-error branches
      (e.g. `os.Chmod` the parent dir to remove write permission, then
      `errors.As` for the typed value and assert `Unwrap()` preserves the
      underlying `*os.PathError`); tighten the `LoadProfiles` test to
      `errors.As(err, &utils.ReadJSONError{})` and assert one leaf.
- [ ] Add only the `LoadProfiles` typed-leaf assertion; treat the
      folder-remove branches as covered by the missing-folder test (weakest).

## DeleteAsset's multi-project unselect is tested with one project

> [!WARNING]
>
> - [`docs/guidelines/testing.md` — Test One Behavior At A Time](../../../docs/guidelines/testing.md)

`TestDeleteAsset_UnselectsAssetFromProjects` adds a single project. The
production code iterates `ProjectList()` (sorted, multi-project) and saves
per iteration; the multi-project case, the deterministic-iteration claim,
and the mid-loop failure path all go unexercised. (If you tick "accumulate
with `errors.Join`" above, the failure test will become tractable; if you
keep short-circuit semantics, an idempotency-on-retry test should go here.)

- [x] Add `TestDeleteAsset_UnselectsAcrossMultipleProjects` (three projects,
      all selecting the asset; assert every project ends cleared) and
      `TestDeleteAsset_PartialFailureLeavesRecoverableState` (force a save
      to fail; assert re-running converges to clean state).
- [ ] Add only the multi-project happy-path test; skip the failure-injection
      variant.

## `UpdateAsset(nil)` / `UpdateProject(nil)` guards untested

> [!WARNING]
>
> - [`docs/guidelines/testing.md` — pin every documented branch](../../../docs/guidelines/testing.md)

If the nil-pointer guards survive the decision in the "Nil-pointer guard"
issue above, they need tests. If they are dropped (panic / value receiver),
this issue is moot.

- [ ] If the guard stays, add `TestUpdateAsset_NilReturnsTypedError` and the
      project equivalent; assert `errors.As` for the chosen type.
- [x] If the guard is dropped, skip — nil panics are the contract.

## Registry.Remove untested for sorted/stable order

> [!WARNING]
>
> - [`docs/guidelines/testing.md` — Assert Behavior](../../../docs/guidelines/testing.md)

`Registry.Save` (`registry.go:84-90`) sorts profiles by name on every write.
`TestRemove_DeletesProfileRef` only registers one profile, so the post-remove
sort + persist path is never observed on a multi-profile registry.

- [x] Add `TestRemove_PreservesSortedOrder`: add three out-of-order
      profiles, `Remove` the middle one, `Load`, assert the slice is sorted
      by name with no duplicates.
- [ ] Skip — `Save` is already covered by the duplicate-path test; the
      sort path is exercised transitively.

## "Orphaned files" is a real domain concept but not in the glossary

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md` — Use The Project Language](../../../docs/guidelines/domain_model.md)
> - [`docs/glossary.md` — canonical vocabulary](../../../docs/glossary.md)

`DeleteAsset` and `DeleteProject` both leave already-synced files behind in
project repos as a deliberate decision. The changelog and the method
comments call them "orphaned files", but the glossary does not. The concept
interacts directly with `ChangeUnknown` / `ChangeDelete` and will recur in
the upcoming TUI tasks — naming it now prevents drift later.

- [x] Add an "Orphaned File" entry to `docs/glossary.md`, cross-link from
      `ChangeDelete` and `ChangeUnknown`, update the `DeleteAsset` /
      `DeleteProject` godocs to use the canonical term.
- [ ] Skip — out of scope for a code-only task.

## Service is growing toward a god-struct (three aggregates, 14 methods)

> [!WARNING]
>
> - [`docs/guidelines/solid.md` — SRP / ISP](../../../docs/guidelines/solid.md)

`Service` now coordinates profile, asset, and project CRUD plus `Plan` /
`Apply` — three change-axes in one struct. The SOLID guideline is explicit
that the split is only worth it when it makes the next change clearer; the
TUI screens listed in the 0015 family will each touch only one of the three
aggregates.

- [ ] Split into `ProfileService`, `AssetService`, `ProjectService` (each
      holding `*registry.Store`); the TUI wires whichever it needs per
      screen. Migration is mechanical.
- [x] Leave as-is — `Service` is a thin facade and the per-screen split
      lives in the TUI layer, where it actually changes behaviour.
