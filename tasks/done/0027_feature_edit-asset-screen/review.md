# Edit Asset Screen — Task 0027 Review

Seven parallel reviewers (security, clean code, clean architecture, SOLID,
DDD, testing, Go) examined the diff against `master`. The screen ships
substantial UX value and follows the established Edit-screen pattern
faithfully, but four high-impact issues recur across multiple reviewers:

1. **Bubble Tea concurrency violation** — `mutationCmd` closures mutate
   `s.files`, `s.tree`, and `s.originalManifest` from the command
   goroutine. Real race against the next `Update`.
2. **Path-containment safety rail has multiple gaps** — symlinks aren't
   resolved, the delete path doesn't validate at all, the doc lies about
   what the check covers, the `..` prefix test misfires on legitimate
   filenames, and `asset.json` is overwritable.
3. **In-asset file mutations live in the TUI** — the screen owns
   `os.Remove`/`os.WriteFile`/`os.MkdirAll`, the safety check, and three
   typed errors that all belong to the `asset` domain package.
4. **Several covered-by-design behaviors are untested** — save failure,
   path-traversal rejection, editor failure, the modal key-routing guard,
   and the dirty-revert round-trip.

Plus a handful of clarity / interface / naming issues. Pick one solution
per block (tick the `[x]` checkbox) and I'll apply them.

---

## Mutation closures mutate screen state from a goroutine

> [!WARNING]
>
> - [go.md §Keep I/O At The Edges](../../docs/guidelines/go.md)
> - [tui.md §Use Bubble Tea For Richer Screens](../../docs/guidelines/tui.md)
> - [domain_model.md §Keep Application Flow Thin](../../docs/guidelines/domain_model.md)

`afterDeleteFile` (`internal/tui/shell/edit_asset.go:688-702`),
`afterCreateFile` (`:718-738`), and `onSave` (`:659-672`) return a
`mutationCmd` whose closure performs filesystem work **and** writes to
`s.files`, calls `s.tree.SetRoot(...)`, and overwrites
`s.originalManifest`. That closure runs on a Bubble Tea command
goroutine, while `Update` continues processing keystrokes on the model
goroutine. `Update` reads `s.tree.Cursor()`, the mnemonic set keyed off
`s.selectedKind()`, and `s.dirty()` (which deep-equals
`s.originalManifest`). The race detector will flag this; in production
the symptom is intermittent treetable corruption after a `d`/`a`/`e`.
The existing `mutationDoneMsg` handler at `:366` already triggers a
fresh `loadCmd` that goes through `handleLoaded`, which rebuilds
`s.files` and `s.tree` correctly — the closure-side writes are
redundant.

```go
// edit_asset.go:689-694 — writes to screen state from Cmd goroutine
return mutationCmd(func() errs.DomainError {
    if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
        return assetFileRemoveError{Path: rel, Err: err}
    }
    s.files = sortedRelativeFiles(s.asset.Dir)        // RACE
    s.tree.SetRoot(buildAssetTree(s.asset, s.files))  // RACE
    _, err := s.actions.UpdateAsset(...)
    return err
}, ...)
```

- [ ] Drop the `s.files = ...` and `s.tree.SetRoot(...)` lines from all
      three closures; the post-mutation `loadCmd()` re-derives them in
      `handleLoaded`. Move the `originalManifest` snapshot refresh out
      of `onSave`'s closure into the `mutationDoneMsg` handler (extend
      the envelope with a discriminator so only the save path triggers
      the snapshot).
- [x] Introduce a typed `filesChangedMsg{files []string}` returned by
      the Cmd; `Update` applies the tree rebuild. The save path returns
      a `saveSucceededMsg{snapshot asset.Manifest}` so the snapshot
      refresh also lands inside `Update`.

---

## Path-containment safety rail has multiple holes

> [!WARNING]
>
> - [security.md §Keep File Access Inside Intended Roots](../../docs/guidelines/security.md)
> - [security.md §Treat External Input As Untrusted](../../docs/guidelines/security.md)
> - [go.md §Return Actionable Errors](../../docs/guidelines/go.md)

`assertInsideAssetDir` (`edit_asset.go:843-857`) is described as "the
only safety rail we ship for the in-screen file mutations" yet has
several gaps:

1. **No symlink expansion.** The function uses `filepath.Abs` + `Rel`
   only — neither resolves symlinks. If the asset folder contains a
   symlink (placed there by a prior sync, an external tool, or even by
   a previous Create File), `link/passwd` where
   `<assetDir>/link → /etc` passes the check, then `os.MkdirAll` and
   `os.WriteFile` follow the symlink and write outside the root. The
   doc on `assetFilePathError` (`edit_asset_errors.go:36-38`) even
   claims "after symlink + dot-dot expansion" — the code never does
   symlink expansion.
2. **Delete path skips the check entirely.** `afterDeleteFile`
   (`edit_asset.go:683-703`) builds
   `abs := filepath.Join(s.asset.Dir, rel)` and goes straight to
   `os.Remove`. Today the `rel` comes from a `WalkDir` (which doesn't
   follow symlinks), so the worst case is removing a symlink-as-link.
   But the create path validates and the delete path doesn't — that
   asymmetry will rot.
3. **`strings.HasPrefix(rel, "..")` matches legitimate filenames.** A
   user-typed name like `..config` or `..bashrc` is rejected even
   though `filepath.Rel` returned the literal basename without
   traversal. Compare to the correct check in
   `internal/tui/components/help/help.go:194`:
   `check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator))`.
4. **`asset.json` and dotfiles overwritable.** `afterCreateFile`
   (`:705-739`) accepts any `rel` that survives the lexical check,
   including `asset.json` (the manifest itself) and `.hidden` paths.
   `asset.RelativeFiles` explicitly excludes both as "non-content" —
   the create path breaks that invariant.

```go
// edit_asset.go:843
func assertInsideAssetDir(assetDir, abs string) errs.DomainError {
    cleanRoot, err := filepath.Abs(assetDir)   // no EvalSymlinks
    cleanTarget, err := filepath.Abs(abs)      // no EvalSymlinks
    rel, err := filepath.Rel(cleanRoot, cleanTarget)
    if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
        //                  ^^ also matches "..bashrc"
        return assetFilePathError{Path: abs}
    }
    return nil
}
```

- [ ] Harden the existing function in-place: call `filepath.EvalSymlinks`
      on `cleanRoot` and on the deepest existing ancestor of `abs`;
      change the prefix check to
      `rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))`;
      reject `rel` whose base name is `config.AssetManifestFileName` or
      starts with `.`; call it from `afterDeleteFile` as well; update
      the doc on `assetFilePathError` to match the actual guarantees.
- [x] Move the safety rail into the `asset` domain package
      (`asset.ResolveRelative(dir, rel) (string, errs.DomainError)`)
      together with `asset.AddFile` / `asset.RemoveFile` (see the
      next finding); both create and delete call the helper; the
      exclusion set lives next to `RelativeFiles` so it can't drift.

---

## In-asset file mutations belong in the `asset` domain

> [!WARNING]
>
> - [clean_architecture.md §How To Apply This](../../docs/guidelines/clean_architecture.md)
> - [tui.md §Apply Go Boundaries To TUI Flows](../../docs/guidelines/tui.md)
> - [domain_model.md §Put Domain Rules In Domain Code](../../docs/guidelines/domain_model.md)
> - [solid.md §DIP](../../docs/guidelines/solid.md)

`afterDeleteFile` and `afterCreateFile` call `os.Remove`,
`os.MkdirAll`, and `os.WriteFile` directly from the screen layer. The
changelog defends this with "the only invariant is 'stay inside
`asset.Dir`'" but the previous finding shows there are at least three
more invariants (no symlink escape, no `asset.json` overwrite, no
`.hidden` writes). The TUI guideline is explicit: "keep filesystem
reads and writes behind app, persistence, render, and sync functions"
and "don't hide arbitrary file writes inside selection, filtering, or
view-building code." `app.Service` already owns `InitAsset`,
`SaveManifest`, `DeleteAsset`, `LoadAsset`, `UpdateAsset` — the
symmetric `AddAssetFile`/`RemoveAssetFile` are missing.

```go
// edit_asset.go:723-727 — domain operations in the UI layer
if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
    return assetFileCreateError{Path: rel, Err: err}
}
if err := os.WriteFile(abs, nil, 0o644); err != nil { ... }
```

- [x] Add `asset.AddFile(dir, rel string) errs.DomainError` and
      `asset.RemoveFile(dir, rel string) errs.DomainError` that own
      the containment check + the excluded-name policy; expose them
      via `app.Service.AddAssetFile`/`RemoveAssetFile` and through
      `actions.Actions`; the screen calls them via the widened
      `editAssetActions` interface — no `os.*` imports left in
      `edit_asset.go`.
- [ ] Keep the file mutations in the screen but at least move the
      safety check + the excluded-name list into the `asset`
      package as exported helpers (`asset.ResolveRelative` +
      `asset.IsContentName`) so the rule is testable next to the type
      it constrains. (Weaker fix; carries the layer breach.)

---

## Domain errors declared in the TUI shell; underlying error not unwrappable

> [!WARNING]
>
> - [errors.md §Wrapping External Errors](../../docs/guidelines/errors.md)
> - [clean_architecture.md §Architecture Boundaries](../../docs/guidelines/clean_architecture.md)
> - [domain_model.md §Put Domain Rules In Domain Code](../../docs/guidelines/domain_model.md)

`internal/tui/shell/edit_asset_errors.go` declares three typed errors
whose `Error()` text describes domain concepts ("asset file", "asset
folder", "escapes the asset folder"). The `asset` package already
houses sibling errors (`AssetWalkError`, `AssetFolderRemoveError`,
`UnsupportedAssetTypeError`). ADR 0008 ("DomainError everywhere")
expects each domain package to own its failure modes so the TUI can
dispatch on `Severity()` without caring about concrete types.

Additionally, `assetFileRemoveError` and `assetFileCreateError` wrap an
`os` error in an `Err error` field but never expose it via `Unwrap()`.
The convention is set by `utils.HashFileError.Unwrap`,
`utils.WriteFileError.Unwrap`, etc. Today `errors.Is(err, os.ErrPermission)`
silently returns false against any of these.

```go
// edit_asset_errors.go:11-20
type assetFileRemoveError struct {
    Path string
    Err  error
}
func (e assetFileRemoveError) Error() string { ... }
func (e assetFileRemoveError) Severity() errs.Severity { ... }
// missing: Unwrap() error { return e.Err }
```

- [x] Move the three structs to `internal/asset/errors.go` as exported
      `FileRemoveError`, `FileCreateError`, `FilePathError`; add
      `Unwrap() error { return e.Err }` to the two that wrap an `os`
      error; have the new `asset.AddFile`/`RemoveFile` helpers return
      them.
- [ ] Add `Unwrap()` to the three structs in place and leave them in
      the shell package. (Weaker fix; doesn't resolve the boundary
      issue.)

---

## In-memory `*asset.Asset` diverges from on-disk truth between blur and save

> [!WARNING]
>
> - [domain_model.md §Preserve Source-Of-Truth Semantics](../../docs/guidelines/domain_model.md)
> - [domain_model.md §Model Consistency Boundaries](../../docs/guidelines/domain_model.md)

`syncFieldsToAsset` (`edit_asset.go:750-758`) writes form values
directly into `s.asset.Manifest` on every focus change, every screen
handler, and every keystroke. The `*asset.Asset` is the same pointer
the action returned. Between blur and Save, the in-memory aggregate
diverges from disk — any concurrent caller that re-resolves the asset
(e.g., the `loadCmd` after `mutationDoneMsg`, a future second tab, a
notification subsystem peeking at the manifest) sees half-edited
fields. `UpdateProject` deliberately avoids this by loading from disk
inside the action and overwriting only specific fields; the new screen
does the opposite.

```go
// edit_asset.go:750-758 — direct mutation of the shared aggregate
func (s *editAssetScreen) syncFieldsToAsset() {
    s.asset.Description = s.state.description
    s.asset.Tags = modals.ParseTags(s.state.tagsCSV)
    s.asset.CompatibleAgents = append([]string(nil), s.state.compatible...)
    s.asset.ExclusiveGroup = s.state.exclusive
}
```

- [x] Hold an `editAssetForm` struct that mirrors the form values and
      keep `s.asset` immutable after `handleLoaded`. Assemble an
      `asset.Manifest` only at `UpdateAsset` call time. `dirty()`
      compares the form struct against a snapshot of itself, not the
      manifest.
- [ ] Have `actions.LoadAsset` return a `*asset.Asset` value (not the
      live pointer the service caches), so the screen edits a private
      copy. Keeps the current sync chokepoint working but removes the
      aliasing.

---

## `dirty()` couples to `reflect.DeepEqual` nil-vs-empty quirks

> [!WARNING]
>
> - [go.md §Use Behavior-Focused Tests](../../docs/guidelines/go.md)
> - [domain_model.md §Be Pragmatic](../../docs/guidelines/domain_model.md)
> - [domain_model.md §Keep Persistence A Detail](../../docs/guidelines/domain_model.md)

`dirty()` (`edit_asset.go:762-767`) is one `reflect.DeepEqual` call.
`DeepEqual` treats `nil` and `[]string{}` as distinct, so the dirty
detector silently depends on `ParseTags("")` returning `nil` (it does),
on `cloneManifest` cloning into `nil` when the source slice is empty
(it does, today), and on every future `Manifest` field being equally
disciplined. Add one slice or map field that distinguishes the two
shapes and the dirty detector silently flips behavior. There is no
test that covers "load → no edits → not dirty" or "load → edit →
revert → not dirty."

```go
// edit_asset.go:762-767
func (s *editAssetScreen) dirty() bool {
    if s.asset == nil { return false }
    return !reflect.DeepEqual(s.asset.Manifest, s.originalManifest)
}
```

- [x] Add `(Manifest) Equal(other Manifest) bool` in `internal/asset/`
      that uses `slices.Equal` for each slice field (treats nil and
      empty as equal). The TUI calls the domain predicate.
- [ ] Replace `reflect.DeepEqual` with a comparison of marshalled JSON
      bytes (round-tripped via `json.Marshal`) so dirty matches
      "what `SaveManifest` would write." Slightly more allocation per
      keypress; trivially correct.

---

## `editProfileActions` widened for methods it never calls (ISP)

> [!WARNING]
>
> - [solid.md §Interface Segregation Principle](../../docs/guidelines/solid.md)

`editProfileActions` (`edit_profile.go:28-37`) declares `LoadAsset` and
`UpdateAsset`, but `editProfileScreen` never invokes either method.
The two methods exist solely so `s.actions` can be forwarded to
`newEditAssetScreen` without a type assertion. Any future test fake
for Edit Profile must stub two methods it never exercises; any future
Edit Asset method addition (e.g., `RenameAsset`) widens the Edit
Profile contract for no consumer.

```go
// edit_profile.go:28-37 — methods used only by the child screen
type editProfileActions interface {
    LoadProfile(in actions.LoadProfileInput) (*profile.Profile, errs.DomainError)
    LoadAsset(in actions.LoadAssetInput) (*asset.Asset, errs.DomainError)      // unused here
    CreateAsset(in actions.CreateAssetInput) (string, errs.DomainError)
    UpdateAsset(in actions.UpdateAssetInput) (struct{}, errs.DomainError)      // unused here
    ...
}
```

- [ ] Keep `editProfileActions` narrow; have the screen accept the
      concrete `*actions.Actions` for child-screen construction (or a
      `func(profileID, assetID string) Screen` factory injected at
      construction time). The asset-specific interface stays inside
      `edit_asset.go` only.
- [x] Compose the two interfaces explicitly:
      `editProfileActions interface { editProfileOwnActions; editAssetActions }`.
      Same surface, but the dependency is visibly the union.

---

## `inputFocused` is a silently opt-in interface — easy to forget (LSP)

> [!WARNING]
>
> - [solid.md §Liskov Substitution Principle](../../docs/guidelines/solid.md)
> - [clean_architecture.md §Component Cohesion](../../docs/guidelines/clean_architecture.md)

`screenWantsRawKey` (`screen.go:53-61`) consults `inputFocused` via a
type assertion and returns `false` when the assertion fails. Only
`*editAssetScreen` implements `InputFocused()` today. Any future
screen that embeds a `huh.Input` / `huh.Text` but forgets the
one-line method silently loses `s`, `n`, `?`, `q` keystrokes to the
shell's global intercept — no compile error, no panic, no test
failure. The `Screen` contract carries an implicit "if you host
text input, you must also implement `inputFocused`" rule that callers
must remember.

```go
// screen.go:57-60
if f, ok := s.(inputFocused); ok {
    return f.InputFocused()
}
return false
```

- [x] Promote `InputFocused() bool` to the required `Screen`
      interface; screens with no text input return `false` in a one-line
      method. Compile-time signal at the cost of one trivial method per
      screen.
- [ ] Keep the optional interface but add a focus-handler primitive
      (`focus.Handler.HostsTextInput() bool`) that screens delegate to
      so they can't accidentally drift from the focus state.

---

## Magic focus indices in `renderCustomize`

> [!WARNING]
>
> - [clean_code.md §Naming](../../docs/guidelines/clean_code.md)

`renderCustomize` (`edit_asset.go:573-581`) compares the handler's
focus index against the literals `1`, `2`, `3`, `4` to pick the
focused border. The numbers are not constants and are not declared
near the registration calls (`newEditAssetScreen:144-150`). The same
literals also appear in tests (`edit_asset_test.go:231,238,602,644`,
`edit_asset_toggle_test.go:23`). A single drift between registration
order and a literal will silently border the wrong panel.

```go
panelBorderFor(focus == 1).Render(s.description.View()),
panelBorderFor(focus == 2).Render(s.tags.View()),
panelBorderFor(focus == 3).Render(s.compatible.View()),
panelBorderFor(focus == 4).Render(s.exclusive.View()),
```

- [ ] Introduce package-private constants
      (`focusIdxTree`, `focusIdxDescription`, `focusIdxTags`, …) used
      by both wiring and renderer (and tests).
- [x] Capture each per-field index as a field on `editAssetScreen` at
      registration time (`s.descIdx = s.handler.Add(s.description)`)
      and read those fields in `renderCustomize`.

---

## `editAssetState` shadows the screen's widget fields

> [!WARNING]
>
> - [clean_code.md §Naming](../../docs/guidelines/clean_code.md)

`editAssetState` (`edit_asset.go:70-75`) holds `description`,
`tagsCSV`, `compatible`, `exclusive` — the same names as the screen's
own `*huh.Text` / `*huh.Input` / `*huh.MultiSelect` widget pointers
(`:95-98`). `s.compatible` is the MultiSelect widget; `s.state.compatible`
is its bound `[]string`. Two parallel namespaces a reader must track.

```go
type editAssetState struct {
    description string
    tagsCSV     string
    compatible  []string
    exclusive   string
}
```

- [x] Rename the struct to `editAssetForm` and the fields to
      `descriptionText`, `tagsCSV`, `compatibleAgents`, `exclusiveGroup`
      so widget vs value is unambiguous.
- [ ] Inline the four fields directly on `editAssetScreen` (drop the
      wrapper struct) and prefix them with `bound` (e.g.
      `boundDescription`) so the binding role is visible.

---

## `fileData` / `fileKind` / `kindAssetDir` naming

> [!WARNING]
>
> - [clean_code.md §Naming](../../docs/guidelines/clean_code.md)
> - [domain_model.md §Use The Project Language](../../docs/guidelines/domain_model.md)

`fileData` (`edit_asset.go:62`) uses the vague `data` suffix the
clean-code guide forbids. `fileKind` has values `kindAssetDir` and
`kindAssetFile` — asymmetric `Asset` prefix on the enum constants but
not the enum type. A directory node is tagged `kind: kindAssetDir`
even when it's the asset root, and the root case is then special-cased
by `rel == ""` in three callers (`treeActionsFn`, `selectedRel`,
`rebuildSet`). The "rel empty == root" convention is implicit.

```go
type fileData struct {
    kind fileKind
    rel  string
}
const (
    kindAssetDir fileKind = iota
    kindAssetFile
)
```

- [x] Rename payload to `assetNode`; enum to `nodeKind` with
      `nodeRoot`, `nodeDir`, `nodeFile`. The root special-case
      (`rel == ""`) disappears.
- [ ] Push the dir/file distinction into the domain by having
      `asset.RelativeFiles` return a richer type (currently `[]string`),
      and consume that here.

---

## `handleLoaded` does six unrelated jobs

> [!WARNING]
>
> - [clean_code.md §Functions (one level of abstraction)](../../docs/guidelines/clean_code.md)

`handleLoaded` (`edit_asset.go:390-408`) handles error notification,
nil-guard, asset adoption, manifest snapshot, file-list scan,
four-field form-state hydration, treetable rebuild, focus reset, and
mnemonic-set rebuild. The four-field hydration (`:400-403`) is the bit
most likely to fall behind: adding a new `asset.Manifest` field
requires editing both this hydrator and the symmetric
`syncFieldsToAsset` writer (`:750-758`), which sit far apart in the
file with no link between them.

- [x] Extract `hydrateForm(*asset.Asset)` paired with
      `syncFieldsToAsset` (the symmetric writer) in their own section
      or file so adding a field touches one obvious place.

---

## Three near-identical `UpdateAsset` mutation closures

> [!WARNING]
>
> - [clean_code.md §Code Smells (Needless repetition)](../../docs/guidelines/clean_code.md)

`onSave` (`edit_asset.go:659-672`), `afterDeleteFile` (`:683-703`),
`afterCreateFile` (`:705-739`), and `handleEditorFinished` (`:421-430`)
each build the same `s.actions.UpdateAsset(actions.UpdateAssetInput{...})`
call inside a `mutationCmd` closure. Only `onSave` additionally
refreshes `originalManifest` on success. If `UpdateAssetInput` ever
grows a field, the maintainer must update four places.

- [x] Extract `(s *editAssetScreen) saveManifest() errs.DomainError`
      and call from every closure. The save path additionally refreshes
      the snapshot inside `Update` after `mutationDoneMsg` (see the
      first finding).

---

## `afterCreateFile` mixes five concerns

> [!WARNING]
>
> - [clean_code.md §Functions](../../docs/guidelines/clean_code.md)

`afterCreateFile` (`edit_asset.go:705-739`) extracts the modal value,
trims, validates, mkdirs, writes the file, refreshes the file list,
rebuilds the tree, and saves the manifest — eight steps in one
closure. The escape, mkdir-fails, and write-fails branches all share
one test (`AddFileConfirmedCreatesPhysicalFile`) that only exercises
the happy path.

- [x] Split into `createPhysicalFile(rel) errs.DomainError` (file
      mechanics only) plus an `Update`-side rebuild step. Keep
      `afterCreateFile` as `validate → call domain helper → return
  done message`.

---

## 858-line `edit_asset.go` blends actions interface, screen, modal

## kinds, three free helpers, and a screen-internal type

> [!WARNING]
>
> - [clean_code.md §Source Structure](../../docs/guidelines/clean_code.md)

`edit_asset.go` houses 858 lines: the `editAssetActions` interface,
`ModalKindAsset` + four constants, `fileKind`/`fileData`, the
`editAssetScreen` struct, ~30 methods, and free helpers
`focusAwareTreetableStyles`, `emptyTreeRoot`, `splitWidth`,
`assetSummaryValue`, `cloneManifest`, `sortedRelativeFiles`,
`buildAssetTree`, `assertInsideAssetDir`. The free helpers
`cloneManifest`, `buildAssetTree`, `assertInsideAssetDir` have no
dependency on the screen.

- [x] Move pure helpers into `edit_asset_helpers.go`;
      `assertInsideAssetDir` + `cloneManifest` into `internal/asset/`
      (see prior findings); `buildAssetTree` next to `treetable.Node`
      (or into a dedicated `tui/components/treetable/build.go`).

---

## `onOpen` triple-defense fires three impossible guards

> [!WARNING]
>
> - [clean_code.md §Functions (Avoid surprising side effects)](../../docs/guidelines/clean_code.md)

`onOpen` (`edit_asset.go:614-628`) early-returns `nil` on three
conditions (`s.asset == nil`, non-file row, empty `rel`). The first
is impossible — the `o` mnemonic isn't in the set until
`handleLoaded` ran; the third overlaps with the second (root row is
the only `rel == ""` case). The same three-step guard repeats in
`onDeleteFile` and `onAdd`.

- [x] Replace the triple-guard with `selectedFileRel() (rel string, ok bool)`
      that returns `false` for non-file rows; drop the `s.asset == nil`
      check.

---

## `routeToFocusedComponent` swallows keys for unknown focusables

> [!WARNING]
>
> - [go.md §Return Actionable Errors](../../docs/guidelines/go.md)
> - [solid.md §Liskov Substitution Principle](../../docs/guidelines/solid.md)

`routeToFocusedComponent` (`edit_asset.go:460-476`) switches on
`*treetable.Model` and `huh.Field`. Any other focusable returns `nil`
silently. Adding a fifth focusable type silently eats key presses.

- [x] Add a `default:` branch that panics in dev (or logs
      `errs.SeverityError`) so a new focusable type fails loud.

---

## `pendingDeleteFile` vs `pendingOpenFile` lifecycles look the same but

## aren't

> [!WARNING]
>
> - [clean_code.md §Comments](../../docs/guidelines/clean_code.md)

Both fields share the `pending*` prefix (`edit_asset.go:113-114`) but
have different lifecycles: `pendingDeleteFile` survives one
modal-resolve round trip; `pendingOpenFile` survives an _external
editor invocation_ (a `tea.ExecProcess` suspend/resume). Nothing in
the field declarations or surrounding comment explains why open needs
to remember the row when the editor returns.

- [ ] Add a comment on `pendingOpenFile` explaining the async-resume
      rationale.
- [x] Rename to `editingFile` and `deletingFile` so the lifecycle
      difference is visible at the field level.

---

## `treetableMinHeight = 8` named "min" but used as fixed height

> [!WARNING]
>
> - [clean_code.md §Naming](../../docs/guidelines/clean_code.md)

`treetableMinHeight` (`edit_asset.go:255`) is declared 25 lines after
its only consumer (`buildTree` at `:230`). The screen never enforces
a floor — the constant is just handed to `treetable.WithHeight`.
The `Min` suffix is misleading; compare to `modalMinWidth` /
`modalMinHeight` (`screen.go:88`), where the size helper actually
clamps.

- [x] Rename to `treetableHeight` (or `defaultTreetableHeight`) and
      declare it next to `buildTree`.

---

## `ModalKindAsset*` exported in an internal package

> [!WARNING]
>
> - [clean_code.md §Naming (Make meaningful distinctions)](../../docs/guidelines/clean_code.md)

`ModalKindAsset` + its four constants (`edit_asset.go:41-48`) mirror
`ModalKind` from `edit_profile.go:42-51`; both are exported, both live
in `package shell` (internal), so the capital letter buys nothing.
`ModalKindCreateAsset` exists in both types and means different things
in each — easy to import the wrong one in a test.

- [x] Unexport: `modalKindAsset` + `modalKindAssetNone`, etc.; same
      for `ModalKind`.

---

## Source-of-truth: no save-failure test, no path-traversal test, no

## editor-failure test, no modal-guard test, no dirty-revert test

> [!WARNING]
>
> - [testing.md §Start With The Smallest Useful Test](../../docs/guidelines/testing.md)
> - [testing.md §Test One Behavior At A Time](../../docs/guidelines/testing.md)

Five branches that are described in the source as load-bearing have
zero coverage:

1. **Save failure.** `onSave` (`edit_asset.go:659-672`) refreshes
   `originalManifest` _only_ on success. No test exercises the error
   branch — a future refactor that always refreshes ships green.
2. **Path traversal rejection.** `assertInsideAssetDir` is described
   as "the only safety rail." No test feeds it `../escape.txt` or
   asserts that the create handler emits an error notification.
3. **Editor failure.** `handleEditorFinished` (`:410-431`) has an
   error branch that emits `SeverityError` and skips `UpdateAsset`.
   `TestEditAssetScreen_EditorFinishedTriggersUpdateAsset` only walks
   the success branch.
4. **Modal key guard.** `Update` (`:343-358`) ships an allow-list of
   message types that bypass `forwardToModal` while a modal is open.
   No test verifies that an `e` keystroke while the back-unsaved
   confirm is open _doesn't_ trigger `onSave`.
5. **Dirty revert.** `dirty()` should clear when the user edits then
   un-edits. No test covers the round trip; the
   `reflect.DeepEqual`-on-slice fragility is silent.

- [x] Add five focused tests: - `SaveFailureKeepsManifestDirty` - `AddFileRejectsPathTraversal` (plus a unit test directly
      against `assertInsideAssetDir`) - `EditorFinishedErrorEmitsNotificationAndSkipsUpdate` - `KeyPressForwardedToModalWhenOpen` - `DirtyClearsWhenStateRevertsToOriginal`
- [ ] Defer until a follow-up task; document the gap as a known
      limitation in the task description.

---

## Tags / unsaved-changes tests bypass the huh binding

> [!WARNING]
>
> - [testing.md §Assert Behavior, Not Mock Mechanics](../../docs/guidelines/testing.md)

Multiple tests assign `s.state.tagsCSV = "newtag"` directly
(`edit_asset_test.go:436, 495, 509, 525`). The huh field binds via
`huh.NewInput().Value(&s.state.tagsCSV)`; writing the struct field
bypasses the `huh.Input.Update → Value()` write path entirely — the
exact path the regression test `edit_asset_toggle_test.go` proves was
broken before for `MultiSelect`. The unsaved-changes / back tests
therefore don't actually prove a _user typing_ triggers `dirty()`.

- [x] Change at least one back-with-unsaved test to drive a
      `tea.KeyPressMsg{Code: 'x', Text: "x"}` through `s.Update` with
      the tags field focused, then assert `s.dirty()`. Keep the
      direct-state shortcuts in mnemonic / focus tests where the
      binding isn't the unit under test.

---

## `MnemonicUniquenessExhaustive` only checks for panics, not uniqueness

> [!WARNING]
>
> - [testing.md §Test One Behavior At A Time](../../docs/guidelines/testing.md)

The plan §8 promised a cross-product of focus indices and cursor
states, with a "no duplicate mnemonic across visible buttons"
assertion. The shipped test (`edit_asset_test.go:245-268`) only walks
focus indices against one fixture and relies on `mnemonic.Set.Add`
panicking on duplicates. If `Set.Add` ever becomes non-panicking, the
test silently passes.

- [x] After each `s.rebuildSet()`, walk the set's buttons, collect
      their runes, and assert `len(unique) == len(buttons)`
      explicitly. Add a third fixture variant covering the empty-asset
      case in the exhaustive matrix.

---

## Brittle `[Open]` substring assertion

> [!WARNING]
>
> - [testing.md §Assert Behavior, Not Mock Mechanics](../../docs/guidelines/testing.md)

`TestEditAssetScreen_OpenButtonRenderedOnlyOnLeafNodes`
(`edit_asset_test.go:270-301`) strips ANSI then asserts
`strings.Contains(view, "[Open]")`. The same intent is already
encoded by `treeActionsFn`'s return value, which is far cheaper to
assert against.

- [x] Replace the string check with a direct assertion on
      `s.treeActionsFn()(node)` returning `[openBtn, deleteBtn]` for
      leaves and `[deleteBtn]` / `nil` for dirs / root. Keep one
      string check for the leaf row only as a render smoke test.

---

## `TestShell_GlobalKeysSkippedWhenInputFocused` mis-located

> [!WARNING]
>
> - [testing.md §Keep Tests Isolated](../../docs/guidelines/testing.md)

The test (`edit_asset_test.go:654-679`) is named `TestShell_…` and
covers shell-scoped behavior, but lives in `edit_asset_test.go` and
needs an entire profile + asset fixture to obtain a screen instance.

- [x] Move to `shell_test.go` (using a tiny `InputFocused` stub
      screen) or rename to
      `TestEditAssetScreen_InputFocusedSuppressesGlobalKeys`.

---

## Back-flow test name shapes drift

> [!WARNING]
>
> - [testing.md §Test One Behavior At A Time](../../docs/guidelines/testing.md)

The four back-flow tests use mixed naming shapes
(`BackWithCleanChangesPopsDirectly`,
`BackWithUnsavedChangesOpensConfirmModal`,
`BackUnsavedConfirmedPops`,
`BackUnsavedRejectedKeepsScreen`) — first two say
`BackWith{Clean,Unsaved}Changes`, last two say `BackUnsaved`. A reader
scanning the file can't tell the four tests describe the same state
machine.

- [x] Rename to one shape:
      `BackWhenClean_Pops`,
      `BackWhenDirty_OpensConfirm`,
      `BackWhenDirty_Confirmed_Pops`,
      `BackWhenDirty_Rejected_KeepsScreen`.

---

## `editAssetFixture.AssetDir` field misaligned (gofmt)

> [!WARNING]
>
> - [go.md §Keep Packages Cohesive](../../docs/guidelines/go.md)

`editAssetFixture` (`edit_asset_test.go:25-32`) has `AssetDir string`
that breaks alignment with the other fields. `make fmt` normalizes
this.

- [x] Run `make fmt` over `internal/tui/shell/`.

---

## `handleResolved` modal dispatch is closed for extension (OCP)

> [!WARNING]
>
> - [solid.md §Open/Closed Principle](../../docs/guidelines/solid.md)

`handleResolved` (`edit_asset.go:597-612`) switches over
`ModalKindAsset`; adding a kind means editing four places (const block,
type, switch, new `afterX` method). With only three kinds today the
cost is small, but a similar switch already exists in
`edit_profile.go`.

- [x] Register `(kind, handler)` pairs at construction time:
      `s.modalHandlers[kind] = s.afterDeleteFile` etc.;
      `handleResolved` becomes `s.modalHandlers[kind](msg)`.
- [ ] Accept the switch; document the boilerplate cost in
      `solid.md`'s OCP section as the project's pragmatic stance.

---

Once you've ticked one box per block, tell me — I'll apply the
selected fixes, run `make fmt && make lint && make build && make test`,
and commit.
