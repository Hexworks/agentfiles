# 0027 changes

Replaced the `editAssetStub` placeholder with a full **Edit Asset** Bubble
Tea screen. The new screen is the first non-modal screen in the shell
that embeds `huh.Field` widgets directly. It owns one `*asset.Asset`
in memory, derives its file list live from `asset.RelativeFiles`, and
calls `actions.UpdateAsset` after every state-changing operation
(file create / delete, manifest save, post-editor refresh).

Layout matches the parent task 0015 spec: a left-pane `treetable.Model`
showing the asset folder (50 % width), a right pane stacking a read-only
Summary group (Name, Type) over a Customize group (Description, Tags,
Compatible Agents, Exclusive Group), and a bottom button row hosting the
screen-level `[Add]`, `[Save]`, `[Back]` mnemonic buttons. Mnemonics
follow the parent's alphabet `{1, 2, o, d, a, e, b}` and are unique in
every focus + cursor state.

## Decisions

- **No new domain helpers; file mutations live in the screen.** —
  **Why:** `app.Service` exposes `LoadAsset` and `UpdateAsset` but no
  `AddAssetFile` / `RemoveAssetFile`. The Edit Asset screen is the only
  caller, and the only invariant for in-asset file mutations is "stay
  inside `asset.Dir`". Adding a service helper for two `os.Remove` /
  `os.WriteFile` calls would create indirection without any new policy.
  The screen calls them directly and re-derives the file list via
  `asset.RelativeFiles`, then `UpdateAsset` re-saves the manifest.

- **Mnemonic set is empty when a right-column input is focused.** —
  **Why:** `o/d/a/e/b` are screen mnemonics that the screen's
  `mnemonic.Set.Match` consumes before forwarding to the focused
  component. If they stayed registered when a `huh.Input` / `huh.Text`
  is focused, typing the letter would fire the action instead of being
  inserted. Status bar still advertises `e save` and `b back` per the
  parent task example; the user reaches them by Tab-cycling back to the
  treetable.

- **Blur-sync at the focus chokepoint, not per-field.** —
  **Why:** `huh.Field` has no per-field blur callback. The screen calls
  `syncFieldsToAsset()` whenever `focus.Handler.Update` reports
  `handled=true` (covers Tab / Shift+Tab / ctrl+1 / ctrl+2 — every
  focus-changing key) **and** at the top of every screen-level handler
  (`onSave`, `onBack`, `onAdd`, `onDelete`, `onOpen`). Both chokepoints
  are simpler and strictly more correct than wrapping each Field with a
  Blur observer.

- **Unsaved-change detection via manifest snapshot + `reflect.DeepEqual`.** —
  **Why:** The parent task wants Back to confirm only when state has
  changed. The screen takes a deep-copy of `m.a.Manifest` on
  `editAssetLoadedMsg` (slice fields cloned via `append([]string(nil), src...)`)
  and compares the live manifest against it. After a successful Save
  the snapshot is refreshed so subsequent Back exits cleanly.

- **Reused `modals.NewCreateFile` for the Add flow.** —
  **Why:** Task 0022 already shipped a typed CreateFile modal — the
  Add flow piggybacks on it instead of growing a parallel implementation.

- **Exported `modals.ParseTags` + `modals.JoinTags` so the screen can
  share the comma-list round-trip with the Create Asset modal.** —
  **Why:** Two callers, same UX contract. Keeping the helpers unexported
  would force a duplicate in `shell/` that could drift.

## Assumptions

- **The Open flow's `UpdateAsset` call after the editor exits is a
  documented no-op for now** — **Why:** `app.Service.UpdateAsset` only
  re-writes the manifest (see the comment at `service.go:432-437`).
  Per the task spec, the screen still invokes it after every
  `editor.FinishedMsg` so a future per-file hash store will see the
  refresh without re-plumbing the screen.

- **`esc` only pops when the treetable is focused.** — **Why:** the back
  button is the source of `esc` (via `WithExtraBindingKeys("esc")`) and
  it is only registered in the screen-wide `mnemonic.Set` while the
  treetable holds focus. Right-column fields consume `esc` themselves
  (`huh` uses it for abort). The user reaches Back from a focused field
  by Tab-cycling back to the treetable.

## Other Notes

- `internal/tui/shell/edit_asset_stub.go` was removed. The shared
  back-only-stub test (`stub_screens_test.go`) lost its `edit asset`
  case; the remaining stubs (select project assets, plan project) keep
  the shared coverage.

- `internal/tui/shell/edit_profile.go::editProfileActions` widened with
  `LoadAsset` + `UpdateAsset` so the existing `s.actions` field can be
  passed straight to `newEditAssetScreen` without a type assertion. The
  concrete `*actions.Actions` already implements both.

- Existing parser/joiner tests for the tag CSV helpers were renamed to
  reflect the exported names.

- No new ADR; the screen follows the established Edit-screen pattern
  documented by ADR 0011 (TUI screen router). No guideline file added.

## edit_asset_stub → edit_asset

The stub:

```go
// before — placeholder screen
type editAssetStub struct {
    backOnlyScreenBase
    profileID string
    assetID   string
}
```

was replaced with:

```go
// after — full screen with treetable + huh fields + per-file mutation
type editAssetScreen struct {
    actions   editAssetActions
    profileID string
    assetID   string

    asset            *asset.Asset
    originalManifest asset.Manifest
    files            []string

    state editAssetState

    handler     *focus.Handler
    tree        *treetable.Model
    description *huh.Text
    tags        *huh.Input
    compatible  *huh.MultiSelect[string]
    exclusive   *huh.Input
    // … mnemonic buttons, modal state, dimensions
}
```

## edit_profile.go push site

```go
// before — pushed the stub
return pushCmd(newEditAssetStub(s.profileID, a.ID))
```

```go
// after — pushes the real screen via the widened actions interface
return pushCmd(newEditAssetScreen(s.actions, s.profileID, a.ID))
```

## modals.ParseTags / JoinTags export

```go
// before — unexported; only callable inside the modals package
func parseTags(csv string) []string { ... }
func joinTags(tags []string) string { ... }
```

```go
// after — exported so the Edit Asset screen reuses the same comma-list
//         round-trip the Create Asset modal already validated
func ParseTags(csv string) []string { ... }
func JoinTags(tags []string) string { ... }
```
