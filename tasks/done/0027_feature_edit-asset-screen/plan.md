# Plan — Task 0027: Edit Asset Screen

See [description.md](./description.md) for the task body.

## Context

Replace placeholder `editAssetStub` (`internal/tui/shell/edit_asset_stub.go`)
with a real **Edit Asset** screen that mirrors `editProfileScreen`'s blueprint
(`internal/tui/shell/edit_profile.go`): two-pane focus group, mnemonic.Set
rebuilt on every focus or load transition, modal-composited Confirm /
Create File / Back-Unsaved dialogs, and `mutationCmd` for every write.

The screen owns `*asset.Asset` in memory, derives the file list live via
`asset.RelativeFiles(asset.Dir)` (no cached `Files` slice on the domain
model), creates/deletes physical files directly inside `asset.Dir` because
no service helper exists, and calls `actions.UpdateAsset` after every
state-changing action. Unsaved-state tracking compares the current
manifest against a snapshot taken on `editAssetLoadedMsg`.

The plan deliberately reuses every pattern already proven in
`internal/tui/shell/edit_profile.go` and validated by
`internal/tui/shell/edit_profile_test.go`, so the new screen reads as a
sibling, not a special case.

## Plan

### 1. File layout

- **Add** `internal/tui/shell/edit_asset.go` — the screen.
- **Add** `internal/tui/shell/edit_asset_test.go` — tests, mirroring
  `edit_profile_test.go`'s fixture style.
- **Delete** `internal/tui/shell/edit_asset_stub.go`.
- **Modify** `internal/tui/shell/edit_profile.go`:
  - Widen `editProfileActions` (line 28-35) with `LoadAsset` +
    `UpdateAsset` so the existing `s.actions` field can be passed
    straight to `newEditAssetScreen` without a type assertion.
  - `onEditAsset()` (line 520-526) pushes
    `newEditAssetScreen(s.actions, s.profileID, a.ID)` instead of the
    stub.
- **Modify** `internal/tui/shell/edit_profile_test.go`:
  - Rename `TestEditProfileScreen_AssetsEKeyPushesEditAssetStub`
    (line 247-268) → `…PushesEditAssetScreen`; assert
    `*editAssetScreen` with the preserved `ProfileID() / AssetID()`
    accessors.

No changes to `internal/asset/`, `internal/app/`, or `internal/actions/`:
the existing `LoadAsset` / `UpdateAsset` actions and
`asset.RelativeFiles` / `asset.SaveManifest` helpers cover the screen's
needs. File create / delete inside the asset folder is done in-screen
via `os.WriteFile` / `os.Remove` (no new domain helper) because the
operation is purely filesystem mechanics with no domain invariant beyond
"stay inside `asset.Dir`".

### 2. Struct shape

```go
type editAssetActions interface {
    LoadAsset(in actions.LoadAssetInput) (*asset.Asset, errs.DomainError)
    UpdateAsset(in actions.UpdateAssetInput) (struct{}, errs.DomainError)
}

type ModalKindAsset int
const (
    ModalKindAssetNone ModalKindAsset = iota
    ModalKindAssetDeleteFile
    ModalKindAssetCreateFile
    ModalKindAssetBackUnsaved
)

type editAssetScreen struct {
    actions   editAssetActions
    profileID string
    assetID   string

    asset            *asset.Asset      // live, mutated by syncFieldsToAsset + file ops
    originalManifest asset.Manifest    // snapshot for unsaved-changes compare
    files            []string          // last asset.RelativeFiles(asset.Dir)

    handler     *focus.Handler
    tree        *treetable.Model
    description *huh.Text
    tags        *huh.Input
    compatible  *huh.MultiSelect[string]
    exclusive   *huh.Input

    tagsBuf string // mirror for tags so huh.Input binds directly, parseTags on sync

    openBtn   *mnemonic.Button // o (treetable, leaf rows only)
    deleteBtn *mnemonic.Button // d (treetable, any row)
    addBtn    *mnemonic.Button // a (treetable focus)
    saveBtn   *mnemonic.Button // e
    backBtn   *mnemonic.Button // b + esc

    treeMnemonic *mnemonic.Button // [1]
    descMnemonic *mnemonic.Button // [2]

    set *mnemonic.Set

    modal             *modal.Modal
    modalKind         ModalKindAsset
    pendingDeleteFile string

    width, height int
}

type editAssetLoadedMsg struct {
    a   *asset.Asset
    err errs.DomainError
}
```

The screen keeps exported `ProfileID()` and `AssetID()` accessors so
tests reach the ids without touching unexported fields (matches the
stub contract today).

### 3. Right column — embedded huh fields

Use `huh.Field` directly (Description = `huh.NewText`, Tags = `huh.NewInput`,
CompatibleAgents = `huh.NewMultiSelect[string]`, ExclusiveGroup =
`huh.NewInput`) reusing the shared constructors in
`internal/tui/modals/fields.go`. `huh.Field` already satisfies
`focus.Focusable` (form.go:143-149), so the fields register with
`focus.Handler` without an adapter.

The Summary group (Name, Type — read-only) is rendered as two lipgloss
rows; no input widget.

Focus handler is `focus.New(focus.WithModifier(focus.ModCtrl))`. Order
of registration drives the Tab cycle:

1. `tree` — `AddMnemonic('1')` → captured into `treeMnemonic` and shown
   in the treetable's panel title.
2. `description` — `AddMnemonic('2')` → captured into `descMnemonic`
   and shown next to the Description label.
3. `tags`, `compatible`, `exclusive` — plain `Add()` (Tab-reachable).

Tags round-trip uses the existing `parseTags` / `joinTags` from
`internal/tui/modals/validators.go` and
`internal/tui/modals/create_asset.go`. Reusing them keeps the
comma-list contract symmetric with the Create Asset modal.

### 4. Blur-saves-to-asset wiring

`syncFieldsToAsset()` reads every input's current value and writes it
back to `s.asset.Manifest`:

```go
func (s *editAssetScreen) syncFieldsToAsset() {
    if s.asset == nil { return }
    s.asset.Description = s.description.GetValue().(string)
    s.asset.Tags = parseTags(s.tagsBuf)
    s.asset.CompatibleAgents = s.compatible.GetValue().([]string)
    s.asset.ExclusiveGroup = s.exclusive.GetValue().(string)
}
```

Call it at two chokepoints in `handleKey`:

1. After `s.handler.Update(m)` returns `handled=true` (covers Tab,
   Shift+Tab, ctrl+1, ctrl+2 — every focus-changing key) — call sync
   before `rebuildSet()`.
2. At the top of every screen-level handler (`onSave`, `onBack`,
   `onAdd`, `onDelete`, `onOpen`) — covers a user pressing a mnemonic
   without first Tabbing away.

Rationale: `huh` does not expose a per-field blur callback hook. A
per-Update sync at the handler chokepoint is simpler and strictly more
correct than wrapping every Field with a Blur observer, and avoids the
sticky-value bug the testing pyramid otherwise pushes onto each field.

### 5. Mnemonic set per focus state

`rebuildSet()` registers buttons into a fresh `mnemonic.Set` based on
`(handler.Focused(), selected tree node kind)`:

| Focus state                                        | Set contents                                  |
| -------------------------------------------------- | --------------------------------------------- |
| Treetable (0), cursor on **leaf**                  | `{treeMnemonic, descMnemonic, openBtn, deleteBtn, addBtn, saveBtn, backBtn}` |
| Treetable (0), cursor on **directory**             | `{treeMnemonic, descMnemonic, deleteBtn, addBtn, saveBtn, backBtn}` |
| Treetable (0), **empty asset folder** (no rows)    | `{treeMnemonic, descMnemonic, addBtn, saveBtn, backBtn}` |
| Right column (1-4): description / tags / multi / exclusive | `{}` (empty)                          |

When a right-column field is focused, the set is intentionally empty so
that printable `o`, `d`, `a`, `e`, `b` flow as text into the huh field
instead of triggering screen actions. The user reaches Save/Back by
Tab-cycling back to the treetable. `esc` still pops because `handleKey`
treats `tea.KeyEsc` as a back-hint **only** when the treetable is focused
(the back button's `WithExtraBindingKeys("esc")` is only in-set when
treetable is focused). This matches the parent task example ("e save b
back tied to focus state").

Mnemonic alphabet across every state: `{1, 2, o, d, a, e, b}` — verified
exhaustively in `TestEditAssetScreen_MnemonicUniquenessExhaustive`.

### 6. Tracking unsaved changes

On `editAssetLoadedMsg`:

```go
s.asset = m.a
s.originalManifest = cloneManifest(m.a.Manifest)
s.tagsBuf = joinTags(m.a.Tags)
// hydrate huh field values from the loaded manifest
```

`cloneManifest` deep-copies the slice fields (`Tags`, `CompatibleAgents`,
`Projections`) via `append([]string(nil), src...)` so subsequent edits
don't mutate the snapshot.

`dirty()` returns `!reflect.DeepEqual(s.asset.Manifest, s.originalManifest)`.

`onBack()`:

```go
s.syncFieldsToAsset()
if !s.dirty() {
    return popCmd()
}
s.openModal(modal.NewConfirm("back-unsaved", "Discard unsaved changes?", nil), ModalKindAssetBackUnsaved)
return s.modal.Init()
```

`afterBackUnsaved(msg)` returns `popCmd()` iff `msg.Confirmed`.

### 7. Modal kinds

| Kind                          | Opener                | Resolved handler            | Effect                                                                              |
| ----------------------------- | --------------------- | --------------------------- | ----------------------------------------------------------------------------------- |
| `ModalKindAssetDeleteFile`    | `onDelete()` (row `d`) | `afterDeleteFile`           | `mutationCmd` → `os.Remove(filepath.Join(asset.Dir, rel))`, refresh `files`, rebuild tree, `UpdateAsset(&s.asset.Manifest)` |
| `ModalKindAssetCreateFile`    | `onAdd()` (`a`)        | `afterCreateFile`           | extract `modals.CreateFileInput`; `mutationCmd` → `os.MkdirAll(filepath.Dir(abs))`, `os.WriteFile(abs, nil, 0o644)`, refresh `files`, rebuild tree, `UpdateAsset` |
| `ModalKindAssetBackUnsaved`   | `onBack()` when dirty  | `afterBackUnsaved`          | `popCmd()` if confirmed                                                             |

`handleResolved` clears `s.modal`, `s.modalKind`, and pending state at
the top, mirroring `editProfileScreen.handleResolved`.

The Open flow does **not** use a modal:

```go
func (s *editAssetScreen) onOpen() tea.Cmd {
    s.syncFieldsToAsset()
    node := s.tree.SelectedNode()
    if !isLeaf(node) { return nil }
    abs := filepath.Join(s.asset.Dir, relPath(node))
    return editor.Open(abs)
}
```

`Update` matches `editor.FinishedMsg` and fans out to
`tea.Batch(notificationCmd, updateAssetCmd)` — the latter just re-saves
the manifest (no-op until per-file hashes are introduced) per the
task spec.

### 8. Tests

`editAssetFixture` mirrors `editProfileFixture`: a real `app.Service`,
real registry, seeded profile + seeded asset on disk. Helpers:
`seedAssetFile(t, dir, rel, body)`, `fakeLoadedAsset(t, dir, manifest, files)`.

Test functions to ship:

- `TestEditAssetScreen_InitLoadsAssetFromActions` — Init produces
  `editAssetLoadedMsg{a: non-nil}`.
- `TestEditAssetScreen_TreetableFocusedLeafSet_HasExpectedMnemonicsInOrder` —
  `{1, 2, o, d, a, e, b}`.
- `TestEditAssetScreen_TreetableFocusedDirectorySet_OmitsOpen` —
  `{1, 2, d, a, e, b}`.
- `TestEditAssetScreen_TreetableFocusedEmptyFolderSet_OmitsRowMnemonics` —
  `{1, 2, a, e, b}`.
- `TestEditAssetScreen_RightColumnFocusedSet_IsEmpty` — focus.Handler at
  index 1 → set has zero buttons.
- `TestEditAssetScreen_MnemonicUniquenessExhaustive` — cross-product
  `(focusIdx ∈ {0,1,2,3,4}) × (cursorState ∈ {leaf, dir, empty})`; for
  each state rebuild the set inside `defer recover()`; assert no panic
  and no duplicate mnemonic across visible buttons.
- `TestEditAssetScreen_OpenButtonRenderedOnlyOnLeafNodes` — assert
  `"[Open]"` substring presence depends on cursor row kind.
- `TestEditAssetScreen_OpenLeafTriggersEditor` — pressing `o` on a leaf
  emits the editor cmd (assert non-nil; sub-test the FinishedMsg path
  in a separate test that injects `editor.FinishedMsg{Err: nil}` and
  asserts a `mutationDoneMsg` follows).
- `TestEditAssetScreen_DeleteFileConfirmedRemovesFileAndCallsUpdate` —
  seed file → `d` → resolve confirmed → assert `os.Stat` fails on the
  file and a `mutationDoneMsg` with `SeverityInfo` is dispatched.
- `TestEditAssetScreen_DeleteFileRejectedMakesNoServiceCall` — symmetric.
- `TestEditAssetScreen_AddFileConfirmedCreatesPhysicalFileAndCallsUpdate` —
  resolve with `modals.CreateFileInput{Path: "scripts/hello.sh"}`;
  assert file exists; assert `mutationDoneMsg`.
- `TestEditAssetScreen_TagsRoundTrip` — load with `Tags: ["foo","bar"]`
  → `s.tagsBuf == "foo, bar"`; set `s.tagsBuf = "baz, qux"`; call
  `syncFieldsToAsset()`; assert `s.asset.Tags == ["baz","qux"]`.
- `TestEditAssetScreen_SyncCopiesFieldsToAsset` — mutate each huh
  field's underlying value via its pointer-bound state struct; call
  `syncFieldsToAsset()`; assert each manifest field reflects the
  change.
- `TestEditAssetScreen_BackWithCleanChangesPopsDirectly` — load → `b`
  → cmd produces `PopScreenMsg`.
- `TestEditAssetScreen_BackWithUnsavedChangesOpensConfirmModal` —
  load → mutate `s.tagsBuf` → `b` → `s.modal != nil`,
  `s.modalKind == ModalKindAssetBackUnsaved`.
- `TestEditAssetScreen_BackUnsavedConfirmedPops` — confirmed=true →
  `PopScreenMsg`.
- `TestEditAssetScreen_BackUnsavedRejectedKeepsScreen` — confirmed=false
  → no pop, modal cleared.
- `TestEditAssetScreen_SavePersistsManifest` — mutate description, press
  `e`, assert `mutationDoneMsg`; reload via `f.Service.LoadAsset` and
  assert description persisted on disk.
- `TestEditAssetScreen_EscTriggersBackOnlyWhenTreetableFocused` — esc
  on focus=0 → pop / back-confirm modal; esc on focus=1 → no pop
  (consumed or ignored).
- `TestEditAssetScreen_StatusKeysTreetableFocused` — bar contains
  `o`, `d`, `e`, `b` bindings (plus the always-shown global keys).
- `TestEditAssetScreen_StatusKeysExcludeScreenLevelAddButton` — `a`
  is not in `StatusKeys()`.
- `TestEditAssetScreen_BodyDoesNotFillViewport` — natural-height
  contract.
- `var _ Screen = (*editAssetScreen)(nil)` — compile-time interface
  satisfaction.

Focus-mnemonic keypresses use
`tea.KeyPressMsg{Code: '1', Mod: tea.ModCtrl}` (or the canonical form
`focus.Handler.parseMnemonicPress` matches — check the existing
`focus_test.go` for the exact construction).

### 9. Wire-up changes

- `internal/tui/shell/edit_profile.go:28-35` — widen
  `editProfileActions` with `LoadAsset` + `UpdateAsset`.
- `internal/tui/shell/edit_profile.go:520-526` — push
  `newEditAssetScreen(s.actions, s.profileID, a.ID)`.
- `internal/tui/shell/edit_asset_stub.go` — delete.
- `internal/tui/shell/edit_profile_test.go:247-268` — rename the test
  and assert against `*editAssetScreen`.

### 10. Actions interface

Declared at the top of `edit_asset.go`, matching the placement of
`editProfileActions` at `edit_profile.go:28`:

```go
type editAssetActions interface {
    LoadAsset(in actions.LoadAssetInput) (*asset.Asset, errs.DomainError)
    UpdateAsset(in actions.UpdateAssetInput) (struct{}, errs.DomainError)
}
```

Constructor `newEditAssetScreen(a editAssetActions, profileID, assetID string)`
panics on nil/empty inputs (matches `newEditProfileScreen`).

### 11. Docs

- No new ADR. The screen follows the established Edit-screen pattern
  already covered by ADR 0011 (TUI screen router) and the building-block
  view. `docs/architecture/05-building-block-view.md` does not
  enumerate individual entity screens, so no edit is needed there.
- No new guideline under `docs/guidelines/`.
- Changelog: `docs/changelog/2026-06-12_0027-edit-asset-screen.md`
  (filled from `./changelog-template.md`).

## Verification

```
make build && make test && make lint
./bin/af   # Profiles → Edit Profile → Edit Asset →
           # ctrl+1 focus treetable, o on a file (edit, save+quit), see Asset updated toast,
           # ctrl+2 focus Description, Tab to Tags, type "foo, bar",
           # b → confirm modal → Yes → returns to Edit Profile.
           # Re-enter; Tags reads "foo, bar".
           # ctrl+1 + a → Create File modal → "scripts/hello.sh" → file appears in tree.
           # d on that file → confirm → file deleted on disk.
```

## Critical files

- `/home/addamsson/projects/agentfiles/internal/tui/shell/edit_asset.go` (new)
- `/home/addamsson/projects/agentfiles/internal/tui/shell/edit_asset_test.go` (new)
- `/home/addamsson/projects/agentfiles/internal/tui/shell/edit_profile.go` (widen interface + push real screen)
- `/home/addamsson/projects/agentfiles/internal/tui/shell/edit_asset_stub.go` (delete)
- `/home/addamsson/projects/agentfiles/internal/tui/shell/edit_profile_test.go` (assert real screen type)
- Reuses: `internal/tui/components/treetable`, `internal/tui/components/focus`,
  `internal/tui/components/mnemonic`, `internal/tui/components/modal`,
  `internal/tui/modals` (`NewCreateFile`, `parseTags`, `joinTags`, field
  constructors), `internal/tui/editor`, `internal/asset` (`RelativeFiles`),
  `internal/actions` (`LoadAsset`, `UpdateAsset`).
