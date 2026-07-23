# Adopt drift into profile — review

Multi-agent review of task 0035 (Adopt: repo → profile reverse-write flow).
Build/test/lint pass; every acceptance criterion is satisfied by the diff.
The task adds a well-scoped exception to invariant #6 with the classification
in `sync` and the profile write + commit in `app.Service`. Layering,
`appapi` mirroring, `asset.WriteFile` containment reuse, and the schema-v3
loader tolerance are all correct.

Substantive issues centre on **two real security holes** (symlink-follow
read, state-file provenance trust) plus **one silent regression risk**
(hardcoded `0o644` mode drops executability). The rest are quality-of-code
items: `sync.Apply` mixes writes and Adopt classification, `Service.Apply`
returns two positional `CommitOutcome` values, the string-composition in
`syncCommitOutcomeCmd` is fragile and untested, and the glossary lags the
new domain vocabulary.

Pick one solution per block below by ticking a checkbox, then run
`af.task.review-apply 35` in a fresh session.

## Symlink-follow in `executeAdoptRequests` read (data exfiltration)

> [!WARNING]
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — Keep File Access Inside Intended Roots
> - [docs/guidelines/sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md)

`app.Service.executeAdoptRequests` reads the local repo body via
`os.ReadFile(filepath.Join(proj.Path, req.Path))` — which transparently
follows symlinks. Sync's forward-write path (`writeRendered`) explicitly
refuses to write through a symlink at the target and passes `O_NOFOLLOW`,
but the Adopt reverse-read has no equivalent guard. An attacker (or a
mis-configured tool) who replaces a managed file with a symlink pointing at
`/etc/passwd`, `~/.ssh/id_rsa`, `.env`, or any other readable file gets:

1. `sync.classifyDesired` follows the symlink through `utils.HashFile → os.ReadFile`, sees a hash mismatch, classifies the row as `ChangeDrift`.
2. User picks `DriftAdopt`.
3. `executeAdoptRequests` reads the symlink target's contents.
4. `asset.WriteFile` writes those contents into `<profile>/assets/<type>/<AssetID>/<SourceRel>`.
5. With git integration on, the profile-repo commit records the exfiltrated bytes; a subsequent `git push` to a remote publishes them.

```go
// internal/app/service.go:396-402
abs := filepath.Join(proj.Path, filepath.FromSlash(req.Path))
body, readErr := os.ReadFile(abs)          // ← follows symlinks
if readErr != nil {
    failures = append(failures, AdoptReadError{Path: req.Path, Err: readErr})
    continue
}
if writeErr := asset.WriteFile(target.Dir, req.SourceRel, body, 0o644); writeErr != nil {
```

Pick one:

- [ ] `Lstat` the abs path in `executeAdoptRequests`; refuse Adopt with a typed `UnsafeSymlinkError` (mirror `sync.UnsafeSymlinkError`) when `Mode()&os.ModeSymlink != 0`. Also add the same guard in `sync.classifyDesired` so a symlinked managed file never reaches drift classification in the first place.
- [ ] Open the repo file with `O_RDONLY|O_NOFOLLOW` (same approach `writeRendered` uses for `safeWriteFlags`) and reject the request when the syscall reports `ELOOP`; guard `classifyDesired` in the same commit.

## `state.json` can redirect a `DriftAdopt` into a different asset

> [!WARNING]
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — Treat External Input As Untrusted
> - [docs/guidelines/sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md)

For `DriftAdopt`, `sync.Apply` populates `AdoptRequest` **directly** from
`preview.ManagedState.ManagedFiles[change.Path]` with no cross-check against
what the render pipeline currently produces for that path. `state.json`
lives in the target repo the assistant already writes to, so a tampered
entry can bind any drifted path to any `(assetID, sourceRel)` referring to
an existing asset. `executeAdoptRequests` then reads the drifted body and
writes it into `<profile>/assets/<type>/<other_asset>/<sourceRel>`,
silently corrupting an unrelated asset. `asset.ResolveRelative` keeps the
write inside `target.Dir`, so the blast radius is one asset — but the
*choice* of asset is attacker-controlled. `UnknownAdopt` does not have this
issue because it recomputes the mapping from the rendered plan
(`assetProjectionDirs`).

```go
// internal/sync/sync.go:480-493 — trusts state.json verbatim
prior := preview.ManagedState.ManagedFiles[change.Path]
if prior.AssetID == "" || prior.SourceRel == "" {
    domainErrs = append(domainErrs, AdoptUnavailableError{ ... })
    ...
}
adoptRequests = append(adoptRequests, AdoptRequest{
    Path:      change.Path,
    AssetID:   prior.AssetID,     // ← no cross-check against rendered plan
    SourceRel: prior.SourceRel,
})
```

Pick one:

- [ ] At Apply time compute `assetProjectionDirs(preview.Files)` (already done for `UnknownAdopt`) and, for each `DriftAdopt` row, assert `owningAssetSourceRelFor(change.Path, assetDirs)` returns the same `(AssetID, SourceRel)` the state carries; on mismatch surface `AdoptUnavailableError` with reason `"state provenance stale, re-plan"`.
- [ ] Use the state-recorded `(AssetID, SourceRel)` only as a hint and derive the authoritative pair from the current render plan (source of truth), keeping state as fallback only when the render mapping is ambiguous.

## `loadState` skips validation of `AssetID` / `SourceRel`

> [!WARNING]
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — Treat External Input As Untrusted

`sync.loadState` validates `ManagedFiles` **keys** with `validatePathKey`
(rejecting `..` and absolute paths) and rejects a half-populated v3 entry,
but leaves `entry.AssetID` and `entry.SourceRel` completely unchecked. A
hand-crafted `state.json` can set `SourceRel: "../../../etc/passwd"` or
`SourceRel: "asset.json"`. `asset.ResolveRelative` catches the write, but:

- Failure is deferred to the write site (`FilePathError`), well past the boundary where untrusted input should be normalized.
- The pathspec entry `filepath.Join(target.Dir, filepath.FromSlash(req.SourceRel))` is computed *before* the write check and only bounces on git's own containment. Defense-in-depth is thin.
- Whitespace-only `AssetID` / `SourceRel` slip past the `""` check.

```go
// internal/sync/sync.go:692-702 — AssetID/SourceRel not validated
for key, entry := range state.ManagedFiles {
    if err := validatePathKey(key); err != nil { ... }
    if (entry.AssetID == "") != (entry.SourceRel == "") { ... }
    // entry.AssetID and entry.SourceRel otherwise unchecked
}
```

Pick one:

- [ ] Extend the `loadState` loop: call `validatePathKey(entry.SourceRel)` when non-empty and reject with `StateCorruptError`. Trim + reject whitespace-only `AssetID` / `SourceRel`. Add a test that stamps `SourceRel: "../evil"` into `state.json` and asserts `Plan` returns `StateCorruptError`.
- [ ] Same validation plus validate `AssetID` against the profile's known asset ids at Plan time (surface `StateCorruptError` for unknown ids rather than deferring to `AdoptUnavailableError` in Apply).

## `AdoptRequest` drops `Mode`; profile-side write hardcodes `0o644`

> [!WARNING]
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — Prefer Explicit Types Over Loose Maps (the value type should carry the value it needs)
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — value objects

`sync.AdoptRequest{Path, AssetID, SourceRel}` has no `Mode`.
`Service.executeAdoptRequests` hardcodes `0o644` when calling
`asset.WriteFile`. Meanwhile `render.RenderedFile.Mode` is preserved
end-to-end for the forward path. The Adopt reverse-write silently
normalizes every adopted file to `0o644`; anything the render pipeline had
marked executable is silently downgraded when adopted.

```go
// internal/app/service.go:402
if writeErr := asset.WriteFile(target.Dir, req.SourceRel, body, 0o644); writeErr != nil {
```

Pick one:

- [ ] Add `Mode os.FileMode` to `sync.AdoptRequest`. Populate it from the matching `preview.Files[i].Mode` for drift and from the source file's `os.Stat` mode for unknown. Thread it into `asset.WriteFile`.
- [ ] Declare `const AdoptFileMode os.FileMode = 0o644` next to `asset.WriteFile` with a comment saying mode fidelity is deferred (link to a follow-up task); replace the magic literal at the call site.

## `Service.Apply` returns two positional `CommitOutcome` values

> [!WARNING]
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Interface Segregation
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — return semantics explicit at the call site

`Service.Apply` returns
`(*appapi.Preview, appapi.CommitOutcome, appapi.CommitOutcome, errs.DomainError)`.
Two positional values of the same type are trivially swap-able at every
caller (`Actions.SyncProject`, `planProjectActions.SyncProject`,
`handleSyncDone`, `syncCommitOutcomeCmd`, `fakePlanActions.SyncProject`).
`go vet` will not flag a swap. The plan file itself flagged this trade-off
when picking the shape; ISP + Go conventions both point at a named struct.

```go
// internal/app/service.go:329
func (s *Service) Apply(profileRef, projectID string, r appapi.Resolutions) (
    *appapi.Preview, appapi.CommitOutcome, appapi.CommitOutcome, errs.DomainError,
) {
```

Pick one:

- [ ] Introduce `appapi.ApplyOutcome { Preview *Preview; Sync, Adopt CommitOutcome }` and return `(ApplyOutcome, errs.DomainError)`. Update `actions/projects.go`, `tui/shell/plan_project.go`, and every test fake to destructure by name.
- [ ] Keep the signature; document the positional order once in a Godoc on `Service.Apply` and add a `//nolint:` note on each fake so a future swap is loud rather than silent.
- [ ] Leave as-is — the interface is narrow, one call site behind it, and the plan explicitly chose this shape.

## `AdoptUnavailableError` is emitted by `internal/app` but declared in `internal/sync`

> [!WARNING]
> - [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — Typed errors per package

`internal/app/service.go:393` appends
`llmsync.AdoptUnavailableError{Path: req.Path, Reason: "profile asset missing"}`.
That struct was authored in `internal/sync/errors.go` for two specific
sync-side conditions (legacy v2 entry; unknown row without owning asset).
Reusing it from app for a third condition ("profile asset went missing
between Plan and Apply") entangles the two packages: any change to the
sync-side error vocabulary now has to consider an app-layer call site, and
`errors.As` in the TUI cannot distinguish the app-layer case from the
sync-layer ones. The new `AdoptReadError` was added correctly to
`internal/app/errors.go`; this sibling should follow the same pattern.

```go
// internal/app/service.go:393
failures = append(failures, llmsync.AdoptUnavailableError{
    Path: req.Path, Reason: "profile asset missing",
})
```

Pick one:

- [ ] Declare `app.AdoptTargetMissingError{Path, AssetID}` in `internal/app/errors.go` and emit that instead. Leave `sync.AdoptUnavailableError` scoped to the two Plan/Apply conditions it documents.
- [ ] Rename `sync.AdoptUnavailableError` → `sync.AdoptProvenanceMissingError` (narrowing its meaning to reverse-mapping keys), then emit a new `app.AdoptTargetMissingError` for the app-layer case.

## `sync.Apply` mixes filesystem writes with Adopt classification

> [!WARNING]
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Single Responsibility Principle
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Functions

`sync.Apply` (internal/sync/sync.go:389) spans ~180 lines and holds five
reasons to change: validate resolution paths, write/delete repo files,
classify `DriftAdopt`/`UnknownAdopt` into `AdoptRequests`, invoke the
reverse-mapping helper, and write `state.json`. The `ChangeDrift → DriftAdopt`
and `ChangeUnknown → UnknownAdopt` sub-switches are ~70 lines of
pure classification logic sitting inside a loop whose primary job is
filesystem mutation. An Adopt row never mutates the repo — grouping it with
`writeRendered` / `removeFile` blurs "what changes on disk" with "what the
caller has to do next in the profile" and forces future readers to hold
both concerns while auditing safety.

```go
// internal/sync/sync.go:465-494 (adopt classification inside the write loop)
case DriftAdopt:
    if preview.ManagedState == nil { ... PreviewInvariantError ... }
    prior := preview.ManagedState.ManagedFiles[change.Path]
    if prior.AssetID == "" || prior.SourceRel == "" {
        domainErrs = append(domainErrs, AdoptUnavailableError{...})
        preserveDriftBaseline(change.Path)
        continue
    }
    adoptRequests = append(adoptRequests, AdoptRequest{...})
    preserveDriftBaseline(change.Path)
```

Pick one:

- [ ] Extract `classifyAdoptForDrift(change, state) (AdoptRequest, bool, DomainError)` and `classifyAdoptForUnknown(change, assetDirs) (AdoptRequest, bool, DomainError)` helpers so the outer switch is one line per branch and the Adopt-classification rules live next to each other (mirrors the existing `classifyDesired` split from Plan).
- [ ] Extract one helper per `ChangeKind` branch (`applyCreateUpdate`, `applyDrift`, `applyDelete`, `applyUnknown`) so the outer switch becomes a five-line dispatcher and each per-kind function sits at one level of abstraction.
- [ ] Leave as-is — 180 lines is above the guideline but the invariants are dense with tests and further refactor risks obscuring the accumulator pattern.

## `preserveDriftBaseline` closure duplicates the open-coded reset in `DriftAdopt`

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Needless repetition

The closure at `sync.go:428` and the `DriftAdopt` branch at `sync.go:472-479`
both spell out the same "if `ManagedState` is nil, emit `PreviewInvariantError`
and `delete(recordedHashes, path)`" logic. The `DriftAdopt` branch then
falls through to `preserveDriftBaseline(change.Path)` on the missing-
provenance sub-case, so the closure ends up called after being reimplemented
inline for the nil-state case.

```go
// closure at sync.go:428
preserveDriftBaseline := func(path string) {
    if preview.ManagedState == nil {
        domainErrs = append(domainErrs, PreviewInvariantError{Kind: "ChangeDrift", Reason: "ManagedState nil"})
        delete(recordedHashes, path)
        return
    }
    ...
}

// duplicated open-coded reset at sync.go:472-479 (DriftAdopt branch)
if preview.ManagedState == nil {
    domainErrs = append(domainErrs, PreviewInvariantError{Kind: "ChangeDrift", Reason: "ManagedState nil"})
    delete(recordedHashes, change.Path)
    continue
}
```

Pick one:

- [ ] Restructure `DriftAdopt` so it calls `preserveDriftBaseline(change.Path)` for both the nil-state and legacy-v2 failure cases (the closure already handles both; drop the inline duplicate).
- [ ] Promote the closure to a named method on a helper struct and route both call sites through it.

## `owningAssetIDFor` duplicates the parent-walk in `owningAssetSourceRelFor`

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Needless repetition

`owningAssetIDFor` (sync.go:915) and `owningAssetSourceRelFor` (sync.go:939)
walk parent directories with identical termination conditions (`.`, `/`,
`IsAssetContainerRoot`, `Dir(dir) == dir`). Plan calls the first to populate
`FileChange.OwningAssetID`; Apply calls the second on the same path to get
both id and source-rel. The comment on the second even notes it "mirrors
owningAssetIDFor so callers get both keys atomically" — yet they remain
separate implementations.

Pick one:

- [ ] Delete `owningAssetIDFor` and have Plan call `owningAssetSourceRelFor`, discarding the `sourceRel` return. One walk, one place to change.
- [ ] Keep the split for documentation but factor the walk into a private `walkToAssetDir(path, assetDirs) (dir string, entry assetDirEntry, ok bool)` used by both.

## `syncCommitOutcomeCmd` string-composition is brittle and untested

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Fragility
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — Assert Behavior

`commits.go:66` reaches into the string it just built and rewrites the
trailing `)` with `; profile ...)`. This only works because the sync branch
happens to emit `)` as its suffix. Add a fourth outcome variant, reformat
the sync branch's parenthesization, or add a base with a `)` in its name,
and this trims the wrong character silently. On top of that, none of the
four cases (both Committed / sync-only / adopt-only / neither) or the two
Failed branches have any test coverage — `grep -rn "syncCommitOutcomeCmd"`
returns only the definition and one caller.

```go
// commits.go:62-67
if adoptCommitted, ok := adopt.(appapi.Committed); ok {
    if text == base {
        text = base + " (profile " + adoptCommitted.SHA + ")"
    } else {
        text = text[:len(text)-1] + "; profile " + adoptCommitted.SHA + ")"  // ← brittle
    }
}
```

Pick one:

- [ ] Extract a pure `mergeCommitText(base string, sync, adopt appapi.CommitOutcome) string` helper that builds a `[]string` of committed-fragments (`"committed <sha>"`, `"profile <sha>"`) and joins them with `"; "` inside a single trailing `" ("+…+")"` wrapper. Add a table-driven `TestMergeCommitText` covering all four Committed combinations plus the two Failed cases.
- [ ] Rewrite `syncCommitOutcomeCmd` to compute `syncSHA` and `adoptSHA` locals up front (empty on non-`Committed`) and build the final string once from those. Add a table-driven test for the six cases and assert the returned `tea.Cmd` batches the expected warn toasts.

## Test coverage gaps in the Adopt boundary

> [!WARNING]
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — Cross-Boundary Integration, Smallest Useful Test

Several new pieces lack direct coverage:

- `asset.WriteFile` (files.go:133) — new containment gate for Adopt. Only tested transitively through the happy `SKILL.md` path in `TestService_Apply_Adopt*`. Escape / reserved-name branches (dotfile, `..`, `asset.json`, symlink) are wholly untested; a refactor that swaps in `os.WriteFile` directly would ship green.
- `AdoptReadError` (service.go:399) — no test asserts that a missing repo file produces the typed error and does not roll back peer adopts in a batch.
- Adopt has no real-git test. Every existing Adopt test uses `fakeCommitter`; the forward flow already ships `TestApply_RealGitRecordsProjectCommit` / `TestApply_RealGitProfileNestedInsideOuterRepo` with `requireGitBinary`+`runShellGit`, and the same harness is unused for the reverse flow — the exact pattern task 0042 flagged as "both sides drift together, the test proves nothing".

```go
// service.go:406 — pathspec built from filepath.Join, sent to os/exec via committer, but only fake-tested
writtenPathspec = append(writtenPathspec,
    filepath.Join(target.Dir, filepath.FromSlash(req.SourceRel)))
```

Pick one:

- [ ] Add all three: `internal/asset/files_test.go` with table-driven negative cases for `WriteFile` (dotfile, `..`, `asset.json`, symlink); a peer test `TestService_Apply_AdoptRepoFileMissingSurfacesReadError` in `internal/app` that removes the repo file before Apply and asserts `errors.As(&AdoptReadError{})` plus peer-success in a two-request batch; `TestService_Apply_Adopt_RealGitRecordsProfileCommit` mirroring `TestApply_RealGitRecordsProjectCommit`.
- [ ] Add only the real-git test (highest bug-catching value given ADR 0042 precedent); defer the other two to a follow-up test-hardening task.
- [ ] Add only the `asset.WriteFile` negative tests plus the `AdoptReadError` peer test; defer the real-git test to a follow-up.

## `time` import is in the wrong group

> [!WARNING]
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — package cohesion / gofmt conventions

`internal/app/service.go:26` places `"time"` in the module-path import
group instead of the stdlib group. `gofmt` will not fix this, but every
other file in the repo follows the two-group layout, so this is a
copy-paste artifact.

```go
import (
    "errors"
    "io/fs"
    "os"
    "path/filepath"
    "slices"
    "sync"

    "github.com/hexworks/agentfiles/internal/appapi"
    // ...
    llmsync "github.com/hexworks/agentfiles/internal/sync"
    "github.com/hexworks/agentfiles/internal/utils"
    "time"                                     // ← wrong group
)
```

Pick one:

- [ ] Move `"time"` into the stdlib group with `"errors"`, `"io/fs"`, etc.
- [ ] Move it and wire `goimports` into `make lint` so this class of drift is caught automatically.

## `path` field name shadows imported `path` package in `plan_project.go`

> [!WARNING]
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Naming, opacity

`internal/tui/shell/plan_project.go:6` imports `"path"`, and `planNode`
(line 68) has a `path` field. Inside `treeActionsFn` the code writes
`path := d.path` — silently shadowing the imported package. The file
already uses `path.Base(dirKey)` at line 797, so a future edit that reaches
for `path.Base` inside the shadowed scope will get a confusing compile
error.

Pick one:

- [ ] Rename the local variable(s) to `dirPath` (matching the field's semantics: a directory key on dir rows).
- [ ] Alias the import as `pathpkg "path"`, matching the convention already used in `internal/sync/sync.go`.

## Glossary and ADR lag the new Adopt vocabulary

> [!WARNING]
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — Use The Project Language
> - [docs/guidelines/documentation.md](../../../docs/guidelines/documentation.md) — document current reality

Several new domain terms are missing from `docs/glossary.md` and one ADR
consequence is not recorded:

- **Source Of Truth** entry still reads strictly (no cross-link to Adopt); CLAUDE.md invariant #6 was amended but the canonical vocabulary was not.
- **Commit Trigger** entry enumerates "three points"; the profile-repo adopt commit is a fourth.
- **File Change** entry does not mention `OwningAssetID` (populated at Plan time for unknown rows nested inside a known asset dir).
- **Adopt Request** value object (`{Path, AssetID, SourceRel}`) crosses the `sync → app` seam but is not in the glossary.
- **Managed File Entry** (`{Hash, AssetID, SourceRel}`, schema v3) is not in the glossary; the half-populated-v3-is-corruption invariant is only in a code comment.

Pick one:

- [ ] Update all five glossary entries above (add `## Adopt Request`, `## Managed File Entry`; extend Source Of Truth, Commit Trigger, File Change); also add the half-populated-v3-is-corruption invariant to ADR 0020's Consequences section.
- [ ] Update the three highest-visibility items (Source Of Truth carve-out, Commit Trigger fourth point, new `## Adopt Request` entry); defer `Managed File Entry` and File Change tweaks to a follow-up docs task.
