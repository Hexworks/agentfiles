# Diff option for drifted files in Plan view — review

The feature is well-built: the `tui/shell → actions → app → domain` direction
holds, `DiffFile` is read-only and never feeds the repo back into render
(invariant 6), the boundary type `DiffBodies` sits in the leaf `appapi`, and
`build`/`test`/`lint` all pass. Clean-architecture and idiomatic-Go reviews
found nothing to change (error typing/`Unwrap`/`Severity`, the safe closure
capture, and the `go-udiff` direct-require are all correct; the `desiredBody`
linear scan and whole-body `string(...)` conversions are bounded and justified).

The findings below are, in priority order: one **security** issue where the
review disagrees with a deliberate changelog decision (symlink disclosure), a
second defense-in-depth path-validation gap, one robustness gap in `BuildDiff`,
a real **testing** hole in the modal layer, and a cluster of **ubiquitous-
language** / clean-code items. Tick exactly one checkbox per issue.

## Symlink-through local read can disclose out-of-repo file contents in the diff modal

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Keep File Access Inside Intended Roots" / "Protect Secrets And Local State"
> - [docs/guidelines/sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md)

`DiffFile` reads the local body with a bare `os.ReadFile` that follows symlinks
(`internal/app/service.go:309`). `desiredBody` only confirms the _slash path
key_ is a managed render target; it says nothing about what the on-disk inode
is. If a hostile or booby-trapped project repo replaces a managed file — e.g.
`.claude/skills/foo/SKILL.md` — with a symlink to `~/.ssh/id_rsa`, `~/.env`, or
`/etc/passwd`, pressing `[Diff]` on that update/drift row reads the link target
and renders its full contents into the scrollable modal (and the terminal
scrollback / any screen-share).

This is the exact threat the **Adopt** path defends against a few lines down in
the same file (`service.go:454` refuses to `os.ReadFile` a symlink, comment
names `id_rsa`/`.env`, per task 0035 review issue #1); the sync write path uses
the same guard. The changelog (`docs/changelog/2026-07-24_0045-…md:48-50`)
deliberately waives it: _"the diff is display-only and never propagates content,
so the Adopt-style Lstat refusal is unnecessary here."_ That reasoning rebuts
**propagation** but not **disclosure** — the Adopt guard defends against both,
and rendering `id_rsa` onto the operator's screen is a real disclosure
primitive with no signal that the "managed file" was actually a link out of the
repo.

```go
// internal/app/service.go:309 — follows a symlink to anywhere on the filesystem
local, readErr := os.ReadFile(filepath.Join(proj.Path, filepath.FromSlash(path)))
if readErr != nil {
    return appapi.DiffBodies{}, DiffLocalReadError{Path: path, Err: readErr}
}
```

- [ ] Mirror the Adopt guard: `os.Lstat` the joined path first and, if
      `info.Mode()&os.ModeSymlink != 0`, return a typed error (reuse
      `llmsync.UnsafeSymlinkError` as Adopt does, or add `DiffLocalSymlinkError`)
      instead of reading — the modal already renders typed diff errors via
      `diffview.ErrorText`.
- [x] Allow intra-repo links only: `EvalSymlinks` the resolved target and refuse
      when it escapes `proj.Path` (matching the pathselector `FollowSymlinks`
      containment rule).
- [ ] Keep as-is and update the changelog to state disclosure was consciously
      accepted (record the residual risk rather than the incomplete rationale).

## `DiffFile` local read is not defense-in-depth validated against `..` path keys

> [!WARNING]
>
> - [docs/guidelines/security.md](../../../docs/guidelines/security.md) — "Treat External Input As Untrusted" / "Keep File Access Inside Intended Roots"

`DiffFile`'s only containment check for the free-form `path` argument is the
implicit `desiredBody(plan, path)` map hit before `os.ReadFile`
(`service.go:305-309`). Today that is sound — render only emits
`surfaces.IsAllowed` targets with no `..` — but it couples `DiffFile`'s path
safety to an invariant enforced entirely in a _different_ package (`render`),
with no local assertion and no test pinning it. Every other on-disk join of a
`FileChange`/render key with a project root runs `validatePathKey` first
(`internal/sync/sync.go`), whose whole purpose is to stop a `..` key escaping
via `filepath.Join`. `path` is a string crossing the `actions → app` boundary;
the guideline says validate such input at the boundary rather than trust an
upstream package's output shape.

```go
// internal/app/service.go:305 — no validatePathKey; safety rides on desiredBody's map hit
desired, found := desiredBody(plan, path)
if !found {
    return appapi.DiffBodies{}, DiffDesiredMissingError{Path: path}
}
local, readErr := os.ReadFile(filepath.Join(proj.Path, filepath.FromSlash(path)))
```

- [x] Call the shared path-key validator (export `llmsync.ValidatePathKey` or add
      an equivalent local guard) on `path` before the join, returning a typed
      error on `..`/absolute/wrong-separator.
- [ ] Keep the `desiredBody`-only approach but add a regression test asserting a
      crafted `../../etc/passwd` path is rejected (not read), pinning the coupling
      to render's clean output.
- [ ] Accept as-is — the `found` gate is a sufficient invariant today (no change).

## `BuildDiff`'s `default` branch silently handles non-diffable change kinds

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Open/Closed Principle

`BuildDiff` (`internal/tui/components/diffview/diff.go:37-52`) switches on `kind`
with only an explicit `ChangeDrift` case; `ChangeUpdate` **and**
`ChangeCreate`/`ChangeDelete`/`ChangeUnknown` all fall into `default` and render
with the update direction. The "only update+drift are diffable" rule is enforced
solely by the caller (`treeActionsFn` gating the `[Diff]` button). If a future
`ChangeKind` is added or the gate is loosened, `BuildDiff` silently produces a
nonsensical diff (e.g. an empty local body for a create row) instead of failing
visibly.

```go
switch kind {
case appapi.ChangeDrift:
    oldLabel, newLabel = "managed", "local"
    oldBody, newBody = string(desired), string(local)
default: // absorbs Update AND Create/Delete/Unknown
    oldLabel, newLabel = "current", "incoming"
    oldBody, newBody = string(local), string(desired)
}
```

- [x] Make `ChangeUpdate` an explicit case; for any other non-drift kind return
      `NoDifferencesMessage` (or a "not diffable" sentinel) so a widened gate
      degrades visibly.
- [ ] Keep the `default` fallback but add a unit test asserting a non-diffable
      kind is never handed to `BuildDiff` in practice (pin the gate as the single
      enforced invariant).

## Diff modal behaviors (esc-close, error render, no-differences pane) are only asserted as `s.modal != nil`

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Cross-Boundary Integration" / "Assert Behavior, Not Mock Mechanics"

`internal/tui/components/diffview/modal.go` is at 0% coverage — `New`, `View`,
`Update` (the `esc`→`Cancelled` branch), `SetSize`, `ErrorText`, and `Lifecycle`
are never exercised. Several acceptance criteria live entirely here and are only
proven indirectly:

- **AC 5** (`esc` closes the scrollable modal, returns to the tree):
  `TestPlanProjectScreen_DiffButtonDispatchesDiffFile` asserts the modal _opens_
  but never sends `esc` to confirm it closes and that `handleResolved`'s
  `planModalDiff` path clears `s.modal`.
- **AC 7** (typed error rendered inside the modal, not blank):
  `TestPlanProjectScreen_DiffLocalReadErrorRendersInModal` only checks
  `s.modal != nil`; it never calls `s.modal.View()` to prove the error string is
  visible — an empty-modal-on-error regression would pass.
- **AC 4** (byte-identical bodies show `No differences` "instead of an empty
  pane"): `TestBuildDiff_EqualBodiesShowsNoDifferences` proves only that the
  _builder_ returns the sentinel string, never that it reaches the pane.

These are pure functions over strings/ints — the cheapest possible unit tests.

```go
// none of these are asserted today:
m := diffview.New("id", "path", diffview.ErrorText(errors.New("boom")), 80, 24)
// View() contains "Failed to produce diff" and "boom"? — untested
// Update(KeyPressMsg esc) → Lifecycle() == modal.Cancelled?        — untested
// SetSize(1,1) does not panic?                                     — untested
```

- [x] Add `internal/tui/components/diffview/modal_test.go` covering `New().View()`
      contains title+body, `Update(esc)` yields `Cancelled`, `SetSize(1,1)` does
      not panic, and `ErrorText(err)` appears in `View()`; plus a shell test that
      feeds `modal.ResolvedMsg` for `planModalDiff` and asserts `s.modal == nil`.
- [ ] Minimum: extend the two existing shell tests to send `esc` (assert close)
      and to assert `s.modal.View()` contains the error / `No differences` string.

## Direction tests do not pin the diff header labels

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Assert Behavior, Not Mock Mechanics"

`BuildDiff` encodes direction in two places: the `-`/`+` line sides _and_ the
`oldLabel`/`newLabel` header. `TestBuildDiff_UpdateDirection` /
`_DriftDirection` (`diff_test.go:15,33`) assert only the `+`/`-` content lines;
the header labels — the user-facing signal of which side is which — are never
checked. Swapping the label constants keeps both tests green while mislabeling
every diff. (See also the label-vocabulary issue below: whichever labels win,
the tests should pin them.)

```go
// update: header should read "--- current" / "+++ incoming"
// drift:  header should read "--- managed" / "+++ local"
if !strings.Contains(out, "current") || !strings.Contains(out, "incoming") { ... }
```

- [x] Add label assertions to both direction tests.
- [ ] Accept — the `+`/`-` body-side assertions already prevent a body swap (no
      change).

## Update-row labels "current"/"incoming" diverge from the glossary's "managed"/"local"

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language"
> - [docs/glossary.md](../../../docs/glossary.md)

The glossary's vocabulary for the two sides of a managed file is
**desired/managed** vs **local/drift**. The Drift branch uses exactly these —
`"managed"`/`"local"` (`diff.go:41`). The Update branch invents two net-new
user-facing labels, `"current"`/`"incoming"` (`diff.go:44`), that appear nowhere
in the glossary or codebase. Both rows of the same modal name the same
underlying pair (local body vs managed body) with two different vocabularies —
the exact drift "Use The Project Language" warns against.

```go
case appapi.ChangeDrift:
    oldLabel, newLabel = "managed", "local"   // glossary-aligned
    oldBody, newBody = string(desired), string(local)
default: // ChangeUpdate
    oldLabel, newLabel = "current", "incoming" // net-new terms
    oldBody, newBody = string(local), string(desired)
```

- [x] Reuse established terms on both branches (Update → old=`"local"`,
      new=`"managed"`), keeping the direction flip but dropping the net-new words.
- [ ] Keep `"current"/"incoming"` (clearer for an Apply preview) but add a
      glossary note declaring them as the Update-diff labels so UI/docs/code do
      not silently diverge.

## "Diff" is a new durable domain term with no glossary entry

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language"
> - [docs/glossary.md](../../../docs/glossary.md)

"Diff" is now a first-class concept: a `[Diff]` row action, `appapi.DiffBodies`,
`Service.DiffFile`, and two typed errors named after it. The glossary defines
every neighbouring concept — `Change Kind`, `File Change`, `Preview`, `Drift`,
`Adopt`, `Resolution` — but has no `Diff` entry, so a reader has no anchor tying
"Diff" to the existing `desired`/`local` vocabulary, nor a statement of its real
domain constraint (read-only, scoped to Update/Drift rows only).

- [x] Add a `## Diff` glossary entry: read-only comparison of a managed file's
      _desired_ body against its on-disk _local_ body, offered only on
      `ChangeUpdate`/`ChangeDrift` rows; cross-link `Change Kind`, `Drift`,
      `Render Plan`. Reconcile the label vocabulary from the previous issue while
      adding it.
- [ ] Skip — treat "Diff" as a generic UI verb, not a durable domain term (no
      glossary change).

## Diff modal duplicates the help modal's frame arithmetic

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Needless repetition"

`diffInner` (`modal.go:70-80`) is a byte-for-byte copy of
`help.helpInner` (`internal/tui/components/help/help.go`), including the
`- 2 - 7` chrome math; the two `View()` methods differ only in the `"Diff: "` /
`"Help: "` title prefix, and `applySize`/`SetSize`/`Update`/`keymap` are
identical. The "same frame as the `?` help modal" behavior is correct and
intended, but the sizing rule now lives in two places and can silently drift
(change the footer height in one and the modals stop matching). `View` also
recomputes `innerW := c.width - 2` inline (`modal.go:114`) without the `< 1`
clamp `diffInner` applies — inherited from `help.go`, guarded by `max(0,…)`, so
not a bug, but a second expression of the same "inner width" concept.

```go
// modal.go and help.go, identical:
vH := height - 2 - 7
if vH < 1 {
    vH = 1
}
```

- [x] Extract the shared frame math (`inner(width,height)` + the chrome constant,
      and optionally an embeddable viewport-frame) into one helper both dialogs
      call, so the "identical frame" invariant holds by construction.
- [ ] Accept the duplication but add a `// keep in sync with help.helpInner` note
      on both sides (only two callers exist today).

## `handleResolved` routes modal kinds via `if`, hiding the deliberate `planModalDiff` no-op

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "avoid hidden ordering requirements"

`handleResolved` (`plan_project.go:936-946`) clears the modal, then branches only
on `planModalRegisterAsset`, falling through to `return nil` for `planModalDiff`.
This is correct (diff has no post-action) but invisible: a reader cannot tell
`planModalDiff` was consciously handled vs forgotten, and if a future kind needs
a post-action the `if` will silently skip it. Minor companion nit: the modal id
`"plan-diff"` is a bare literal at the call site (`plan_project.go:799`).

```go
if kind == planModalRegisterAsset {
    return s.afterRegisterAsset(msg, dirKey)
}
return nil // planModalDiff and planModalNone land here implicitly
```

- [x] Convert to `switch kind` with an explicit
      `case planModalDiff, planModalNone: return nil` documenting the deliberate
      no-op; optionally promote `"plan-diff"` to a named `const`.
- [ ] Add a one-line comment naming the no-op kinds and leave the `if` as-is.
