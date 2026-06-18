# Create Asset From Folder — review

The feature is well-built: the layer directions are correct (`asset→utils`, `app→domain`, `actions→app`, `tui→app/actions`), `CopyDir` correctly skips symlinks so it cannot follow a link out of the source tree, `asset.json` is written last so a stray source manifest cannot clobber the authoritative one, and the bubbletea v2 modal wiring (no blocking I/O in `Update`, loop-var capture, modal-kind reset, nil-guards) is idiomatic. Tests follow the project's given/when/then style with sensible fakes.

The findings below cluster around three roots: (1) domain rules and trusted-path construction leaking into `internal/tui`; (2) the `CreateAssetFromFolder` use case not actually enforcing the "single consistency boundary" its comment promises (no rollback, non-idempotent selection, no result-validity check); and (3) `CopyDir` trusting an unbounded, externally-shaped source folder. The rest are duplication, an error-wrapping nit, test gaps, and two readability nits.

Tick exactly one checkbox per issue to tell me which fix to apply, then come back.

## Business logic (`dirAllUnknown` + source-path resolution) lives in the TUI

> [!WARNING]
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "TUI collects input and renders only"; Stable Dependencies
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code"; do not rely on screen-level validation as the only protection

`dirAllUnknown` (`internal/tui/shell/plan_project.go:97`) encodes a genuine domain eligibility rule — "a folder may be registered only if every descendant file is unknown and there is ≥1 such file; a folder with managed leaves is partly owned already." That is policy, not presentation, yet it exists only in the shell and is the *only* thing preventing registration of a partly-managed folder. `CreateAssetFromFolder` (`internal/app/service.go:167`) never re-asserts it, so the safety relies wholly on the TUI never offering the button.

The screen also computes the trusted filesystem path: `s.registerSourceDir = filepath.Join(s.projectPath, filepath.FromSlash(dirPath))` (`plan_project.go:593`). Reconstructing an absolute source path from a forward-slash plan key is a domain/IO concern. The service then trusts a fully-resolved `SourceDir` it did not resolve or contain — contrast every other asset write method, whose comments stress "a tampered caller cannot redirect the write outside the profile root."

```go
// plan_project.go — eligibility rule + trusted-path resolution both in the TUI
if dirAllUnknown(n) { return []*mnemonic.Button{s.registerAssetBtn(d.path)} }
s.registerSourceDir = filepath.Join(s.projectPath, filepath.FromSlash(dirPath))
```

Choose one:

- [ ] Move the all-unknown predicate into `app` (e.g. derive registerable directory keys from the `Preview`) **and** pass the project-relative `dirKey` through `CreateAssetFromFolderInput`, letting the service join it against the resolved project root and re-assert eligibility — keeps the TUI rendering-only and the invariant at the consistency boundary.
- [ ] Keep `dirAllUnknown` in the shell for the button gate, but re-assert eligibility and resolve/contain `sourceDir` inside `CreateAssetFromFolder` so the use case is safe regardless of caller.
- [ ] Leave as-is — accept the TUI as the sole enforcement point (document the assumption on the use case).

## Register flow can construct an invalid or unrenderable asset

> [!WARNING]
> - [docs/guidelines/asset_authoring.md](../../../docs/guidelines/asset_authoring.md) — type-specific conventions (SKILL.md for skills); generic types need explicit projections inside managed surfaces
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — invariants enforced in domain code

`InitFromFolder` (`internal/asset/asset.go:195`) validates only `manifest.Validate()` (id/name/type shape) — nothing about whether the copied folder satisfies its type's render conventions. The feature's contract is that the re-plan reclassifies the files as managed (`handleRegisterAssetDone`, `plan_project.go:646`), but the copy can yield an asset that renders nothing or errors:

- A `skill` whose source has no `SKILL.md`: `addSkillOutputs` reads `config.SkillStarterFileName` first and errors if absent (`internal/render/render.go`). `Init` can't hit this (it writes a starter SKILL.md); `InitFromFolder` writes no starter, so the convention is silently unmet and the promised re-plan fails to render.
- A generic type (`mcp`/`rule`/`hook`) needs explicit `projections` to map files into managed surfaces. The modal collects only the bare manifest and never populates `Projections`, so the copied content becomes dead weight and the re-classification never happens. For `hook`, content copied from an unmanaged folder would later project into an execution surface (`.claude/hooks/`).

```go
// asset.go:203 — nothing verifies the source satisfies the type's render contract
if err := utils.CopyDir(sourceDir, dir); err != nil { return "", err }
```

Choose one:

- [ ] Restrict the [Register] entry point to content-only types (e.g. `skill`/`rule` with required content), and have `InitFromFolder` (or the use case) verify type-specific minimums after the copy (skill ⇒ SKILL.md present; generic ⇒ non-empty projections passing `surfaces.IsAllowed`), returning a typed domain error otherwise.
- [ ] Validate after copy but allow all types — error out (typed) when the source does not satisfy the chosen type's render contract.
- [ ] Leave as-is — treat an unrenderable asset as user error surfaced on the next plan (document the limitation in the manual).

## `CreateAssetFromFolder` leaves an orphaned asset if `project.Save` fails

> [!WARNING]
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — SRP / the "single consistency boundary" must actually hold
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — partial-failure accumulation (`DeleteAsset` precedent)

The method's doc comment (`internal/app/service.go:159`) promises "three effects … a single consistency boundary," but the code does not enforce it. If `asset.InitFromFolder` succeeds (files copied, manifest written into the profile) and `project.Save` then fails (`service.go:182`), the method returns an error while the profile keeps a half-created, unselected asset. There is no rollback and no `errs.Errors` accumulation — unlike `DeleteAsset` (`service.go:570`), which joins partial failures so a re-run converges.

```go
if _, initErr := asset.InitFromFolder(loaded.Root, manifest, sourceDir); initErr != nil {
    return "", initErr
}
p.SelectedAssetIDs = append(p.SelectedAssetIDs, manifest.ID)
if saveErr := project.Save(loaded.Root, p); saveErr != nil {
    return "", saveErr // asset folder now orphaned in the profile, unselected
}
```

Choose one:

- [ ] On `project.Save` failure, roll back via `asset.Delete(dir)` (the dir `InitFromFolder` returns) and join both errors with `errs.Errors`, matching the `DeleteAsset` convention.
- [ ] Accept convergence-on-retry instead: document that a re-run reselects, and make the duplicate-id path a no-op-continue (rather than `AssetExistsError`) so the orphan can be re-selected.
- [ ] Leave as-is — accept the rare orphan and document it.

## Selection append duplicates `SelectAsset` and is not idempotent

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Needless repetition (rules that can drift)
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — one owner for a domain rule

`CreateAssetFromFolder` appends to `p.SelectedAssetIDs` and calls `project.Save` inline (`internal/app/service.go:181`), re-implementing the selection logic `SelectAsset` already owns (`service.go:618`). The comment justifies bypassing `SelectAsset` (its `loaded.Assets[id] == nil` guard would reject the just-created id), which is fair — but the inline copy drops `SelectAsset`'s dedup. The asset-id guard (`service.go:175`) only checks `loaded.Assets`, not `p.SelectedAssetIDs`, so a dangling selection entry whose id matches the new slug would be appended twice. Two methods now own "how a selection is appended," and they can drift.

```go
p.SelectedAssetIDs = append(p.SelectedAssetIDs, manifest.ID) // SelectAsset de-dupes; this does not
```

Choose one:

- [ ] Extract an aggregate-level `p.SelectAsset(id)` (operates on the in-memory manifest, no reload) shared by both `Service.SelectAsset` and `CreateAssetFromFolder`, so the append-if-absent rule has one owner.
- [ ] Add a `slices.Contains` guard before the inline append to match `SelectAsset`'s idempotence contract.
- [ ] Leave as-is — rely on the `AssetExistsError` precondition and document that dangling selections are out of scope.

## `CopyDir` reads whole files into memory with no size/count ceiling

> [!WARNING]
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — treat external input as untrusted; make risky operations visible
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — idiomatic streaming copy

`CopyDir` (`internal/utils/fs.go:109`) slurps each file via `os.ReadFile` then writes it, with no per-file or total cap, over a `sourceDir` that is whatever folder the user planned (potentially a checked-in binary, a `node_modules` subtree, etc., if it happens to be all-unknown). A large file forces the whole content into a single `[]byte`; the idiomatic form streams via `io.Copy`. The user also cannot see what will be copied before confirming.

```go
data, readErr := os.ReadFile(path) // fs.go:109 — unbounded read of an arbitrary source file
```

Choose one:

- [ ] Switch to streaming `io.Copy` (explicit open/create with `info.Mode().Perm()`), and/or enforce a per-file + total-size cap returning a typed error.
- [ ] Surface the file count / total size in the Create Asset modal before confirm (visibility) while keeping `os.ReadFile`.
- [ ] Leave as-is — document the assumption that asset folders are small, hand-curated content.

## `CopyDir` preserves untrusted source file modes into the profile

> [!WARNING]
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — restrictive permissions for user-local files

`CopyDir` reproduces each source file's permission bits via `info.Mode().Perm()` (`internal/utils/fs.go:114`). Every other write path in the repo uses a fixed `0o644` (`WriteJSON`, the `WriteFile` callers in `asset.go`). A world/group-writable file in an unowned source folder would land world-writable inside `~/profiles/.../assets/`. (Downstream render forces `0o644`, so this is confined to the profile folder; `Perm()` does strip setuid/setgid/sticky.)

```go
WriteFile(filepath.Join(dst, ...), data, info.Mode().Perm()) // fs.go:114 — external mode trusted
```

Choose one:

- [ ] Normalize copied file modes to `0o644` like every other write path in the repo.
- [ ] Mask out group/other write bits at minimum: `info.Mode().Perm() &^ 0o022`.
- [ ] Leave as-is — accept source modes inside the profile folder.

## `CopyDir` double-wraps an already-typed `DomainError`

> [!WARNING]
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — "Propagating Domain Errors": do not re-wrap a `DomainError` in another `DomainError`

In the `WalkDir` callback, `utils.WriteFile` already returns a typed `DomainError` (`WriteFileError`/`EnsureDirError`), but it is returned as a plain `error` and then re-wrapped in `CopyDirError` (`internal/utils/fs.go:114-121`). The leaf stays reachable via `Unwrap`, so this is not a correctness bug, but a write failure is reported as "copy directory …: write file …" and the immediate severity becomes `CopyDirError`'s. Only the genuinely external failures (`os.ReadFile`, `d.Info()`, the walk `err`) need the `CopyDirError` wrap.

```go
if writeErr := WriteFile(...); writeErr != nil {
    return writeErr // already a DomainError — let it surface without the CopyDirError layer
}
```

Choose one:

- [ ] After `WalkDir`, return a `DomainError` leaf directly (detect via `errors.As`) instead of wrapping it in `CopyDirError`; wrap only raw `os` errors.
- [ ] Accept the unified failure mode and document on `CopyDirError` that it deliberately presents one error surface (`Unwrap` keeps the leaf inspectable).

## `asset.InitFromFolder` duplicates `Init`'s scaffold prelude

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Needless repetition
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — OCP: keep the layout rule closed, vary only the body-seeding step

`InitFromFolder` (`internal/asset/asset.go:195`) repeats `Init`'s prelude verbatim (`internal/asset/asset.go:152`): `manifest.Validate()` → derive `dir` from `root`/`AssetsDirName`/type/id → `utils.EnsureDir(dir)` → write manifest last. Only the middle differs (`CopyDir` vs. per-type starter writes). The on-disk layout rule now lives in two places and must change in lockstep (task 0010 already plans to extract starter content).

```go
// appears in both Init and InitFromFolder:
dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)
if err := utils.EnsureDir(dir); err != nil { return "", err }
```

Choose one:

- [ ] Extract a private `scaffold(root, manifest, seed func(dir string) errs.DomainError)` owning validate → derive dir → EnsureDir → seed → write manifest; `Init` passes the starter seed, `InitFromFolder` passes `func(dir){ return utils.CopyDir(sourceDir, dir) }`.
- [ ] Extract just `assetDir(root, manifest) string` shared by both.
- [ ] Leave as-is — two short adjacent functions; defer until task 0010 reworks scaffolding.

## Test coverage gaps

> [!WARNING]
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — test the non-obvious behavior; cover error paths

The new code is unit-tested at the right pyramid level, but several load-bearing paths are unverified: (1) the headline "asset.json written last so a stray source manifest can't clobber" guarantee — no test ever puts an `asset.json` in `sourceDir`; (2) `CopyDir` error path (`CopyDirError`), nonexistent source, and empty source; (3) the `onRegisterAsset` → `handleResolved` wiring (the `filepath.Join`/`FromSlash` path-build and the source-dir snapshot/reset) — tests start one step downstream at `afterRegisterAsset`; (4) `CreateAssetFromFolder` copy-failure leaving no partial selection; (5) `handleRegisterAssetDone` error branch; (6) a nested subtree copied through `InitFromFolder`/`CreateAssetFromFolder` (only flat single-file sources are tested).

```go
func TestInitFromFolder_SourceManifestDoesNotClobberAuthoritative(t *testing.T) {
    source := t.TempDir()
    os.WriteFile(filepath.Join(source, "asset.json"),
        []byte(`{"id":"evil","name":"Evil","type":"rule"}`), 0o644)
    dir, _ := InitFromFolder(t.TempDir(), Manifest{ID: "good", Name: "Good", Type: TypeSkill}, source)
    loaded, _ := Load(dir)
    if loaded.ID != "good" { t.Fatalf("source asset.json clobbered ours: %+v", loaded.Manifest) }
}
```

Choose one:

- [ ] Add all listed tests (clobber-prevention, `CopyDir` error/empty/nonexistent, `onRegisterAsset`+`handleResolved` wiring, copy-failure no-partial-selection, `handleRegisterAssetDone` error branch, nested-subtree copy).
- [ ] Add the high-value subset only: clobber-prevention, `CopyDir` nonexistent-source error, and the `onRegisterAsset` path-join + modal-routing tests.
- [ ] Leave coverage as-is.

## Minor: `InitFromFolder` doc comment narrates the steps

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — comments should explain WHY, not restate the next lines

The comment (`internal/asset/asset.go:189`) recaps each step ("validated, created, copied in"), restating the code. Only the WHY clause — manifest written last so a stray `asset.json` can't clobber — is load-bearing.

```go
// InitFromFolder is the source-from-folder counterpart of Init. The manifest is
// written last so a stray asset.json in the source cannot clobber it.
```

Choose one:

- [ ] Trim to the counterpart-of-`Init` framing plus the write-last invariant; drop the step list.
- [ ] Leave as-is.

## Minor: `dirAllUnknown` closure with two mutable accumulators

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Opacity; one level of abstraction

`dirAllUnknown` (`internal/tui/shell/plan_project.go:97`) mixes outer mutable state (`files`, `allUnknown`) with a side-effecting recursive closure. A pair of pure helpers (or one returning `(total, unmanaged int)`) reads more clearly. Behavior is covered by `TestDirAllUnknown`. (If the first issue moves this rule into `app`, apply this cleanup there instead.)

```go
files := 0
allUnknown := true
var walk func(*treetable.Node) // mutates both as a side effect
```

Choose one:

- [ ] Replace with a helper returning `(total, unmanaged int)` and judge `total > 0 && total == unmanaged` at the top level.
- [ ] Leave as-is — small and tested.
