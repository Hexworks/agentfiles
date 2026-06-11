# 0025 changes

Replaced the Welcome menu's `profilesStub` with the real Profiles screen — the first entity-management surface in the TUI. The screen owns a bubbles/table view of every registered profile, screen-level `[Create New Profile]` / `[Register Profile]` mnemonic buttons, and row-level `[Edit]` / `[Delete]` mnemonic buttons that render inline on the cursor row. Delete is a two-step confirmation: the first dialog gates the registry-side removal, the second decides whether the on-disk profile folder is also wiped. Every mutation flows back through `notifications.From`'s sibling helpers so the user sees a toast + log entry for the outcome.

Edit Profile lands in task 0026; an `editProfileStub` ships now so the row-level `e` action has a valid push target and the row's profile id is forwarded for the future implementation to consume.

## Decisions

- **Mutation cmd uses a custom envelope (`profileMutationDoneMsg`) instead of `tea.Sequence`.** **Why:** `tea.Sequence` wraps its children in an unexported `sequenceMsg`, so tests cannot inspect the dispatched chain. A custom envelope runs the action synchronously inside the Cmd closure, returns a message Update can act on, and lets Update emit a `tea.Batch(notification, reload)` — ordered by virtue of the action having already completed before the message is dispatched.
- **Confirmation flow is in-screen, not a sub-screen.** **Why:** Notifications modal pushes itself as a Screen because its body fills the viewport; Profiles already owns its body and just wants a dialog layered on top. Keeping the modal field local mirrors the Bubble Tea pattern documented in `modal.go` and avoids juggling stack frames per dialog.
- **Mnemonic uniqueness is rebuilt every selection-state transition.** **Why:** `mnemonic.Set.Add` panics on duplicates. Re-running `rebuildSet` on each transition turns the safety net into a continuous invariant rather than a startup-only check, and keeps the empty/populated row sets distinct without leaking the `e`/`d` bindings to status while the table is empty.
- **Status bar carries only `e`/`d`, not `c`/`r`.** **Why:** Parent task 0015's status-bar rule says screen-level labelled buttons must not be repeated. The button row beneath the table already shows `[Create New Profile]` / `[Register Profile]`, so re-listing them would be visual noise.

## Assumptions

- **Welcome screen takes `*actions.Actions` at construction.** **Why:** The screen's "Profiles" entry now needs to spin a `profilesScreen`, which needs the actions handle. Threading actions through `newWelcomeScreen(globals, a)` keeps the dependency explicit and avoids a registry singleton.
- **Profile.Root is the user-facing "Path" column value.** **Why:** `LoadProfiles` returns `[]*profile.Profile`. The `Profile` struct exposes `Root` (filesystem path) and `Manifest.{ID,Name}`. The task mockup labels the column "Path"; `Root` is the natural mapping.
- **Empty-state body shows a hint line rather than a styled empty table.** **Why:** A zero-row bubbles/table renders as a header with blank rows, which is more confusing than helpful. A one-line hint with the two screen-level buttons beneath it preserves the same discoverability while making the state legible.

## Other Notes

- No ADR added: the screen router pattern (ADR 0011) already covers this surface; no new architectural decision was made.
- No new `docs/guidelines/` entries: behavior matches existing `tui.md` patterns (Init-loaded data, mnemonics for sticky shortcuts, modals composited over body).
- `internal/tui/shell/profiles_stub.go` + its test were removed as part of the swap-in.

## Profiles screen Screen implementation

A new `profilesScreen` Screen that owns the bubbles/table model, the four mnemonic buttons, an active-modal field, and the in-flight delete id. `Init` returns a load command; `Update` dispatches per-message-type and forwards key presses to the modal when one is open. Body composites the modal over the rendered table+button-row using `*modal.Modal.Render`.

```go
// before — placeholder push from welcome.go
{label: "Profiles", action: func() tea.Cmd { return pushCmd(newProfilesStub()) }},
```

```go
// after — the real screen is constructed with the actions handle so it
// can load + mutate profiles. The Welcome row only knows about the screen
// constructor; nothing about the row needed to change beyond the type.
{label: "Profiles", action: func() tea.Cmd { return pushCmd(newProfilesScreen(a)) }},
```

## Mutation cmd + notification + reload

The screen wraps every Create / Register / Delete action in a generic `mutationCmd[T]` that runs the action synchronously, then dispatches a `profileMutationDoneMsg`. Update reacts by emitting a `tea.Batch(notificationCmd, loadCmd)` — the action is already complete, so the reload reads the fresh registry without racing.

```go
// after — mutation flow with explicit done envelope
func mutationCmd[T any](action func() (T, errs.DomainError), successText string) tea.Cmd {
    return func() tea.Msg {
        _, err := action()
        if err != nil {
            return profileMutationDoneMsg{text: err.Error(), severity: err.Severity()}
        }
        return profileMutationDoneMsg{text: successText, severity: errs.SeverityInfo}
    }
}

// in Update:
case profileMutationDoneMsg:
    return s, tea.Batch(notificationCmd(m.severity, m.text), s.loadCmd())
```

## Two-step delete

Delete is gated by two sequential confirmation modals identified by `delete-profile-1` and `delete-profile-2`. The first asks whether to delete the profile at all; on Yes, the second asks whether to remove the on-disk folder. The two outcomes route to `DeleteProfile` (registry only) and `DeleteProfileWithFolder` respectively, matching the parent task 0015 spec.

```go
// after — the two-step delete state machine
func (s *profilesScreen) afterDeleteStep1(msg modal.ResolvedMsg) tea.Cmd {
    if !msg.Confirmed {
        s.pendingDeleteID = ""
        s.pendingDeleteName = ""
        return nil
    }
    s.openModal(modal.NewConfirm("delete-profile-2", "Also delete profile folder on disk?", nil))
    return s.modal.Init()
}
```

## Edit Profile stub

A small `editProfileStub` ships so the row-level `[Edit]` action has a valid push target until task 0026 lands the real screen. It mirrors the `settingsScreen` shape: a single `[Back]` mnemonic button (bound to both `b` and `esc`) and a body that names the captured profile id.

```go
// after — captures the profile id so the future Edit Profile screen
// only needs to swap the body, not the call site.
type editProfileStub struct {
    profileID string
    back      *mnemonic.Button
}
```
