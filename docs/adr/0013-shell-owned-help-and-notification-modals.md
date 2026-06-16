# Help And Notifications As Shell-Owned Modals

## Status

accepted

## Context

ADR 0011 established the screen-router stack: the shell intercepts a global
key set (`n` notifications, `s` settings, `?` info/help, `q` quit) before
the active screen, and screens navigate by pushing/popping `Screen` values.
Help and notifications were initially implemented as `Screen`s on that
stack — `infoScreen` and `notificationsScreen`, pushed via the `?` and `n`
keys.

That placement has a flaw. A screen on the stack becomes the active screen,
and the shell forwards keys to the active screen *after* the global-key
check. But the global check itself returns early on a hit and never
forwards. The asymmetry meant that once help was pushed, the help screen
was just another stack entry — and the intent of help (a transient overlay
you dismiss to return exactly where you were) clashed with stack semantics:
help could itself be shadowed, and the global quit/affordance keys behaved
inconsistently depending on whether the overlay was open.

The `modal` component (ADR 0007 lineage, extended this batch with
`WithCaption` and `Active()`) already provided a centered overlay that
resolves through a typed message and does not participate in the navigation
stack. It was the right primitive; help and notifications were using the
wrong one.

## Decision

Help and notifications are **shell-owned modal overlays**, not screens on
the router stack.

- `infoScreen` and `notificationsScreen` are deleted.
- `shell.Model` gains `helpModal *modal.Modal` and
  `notificationsModal *modal.Modal` fields.
- New messages `ShowHelpMsg{Topic}` and `ShowNotificationsMsg` (emitted by
  `showHelpCmd` / `showNotificationsCmd`) open the overlays via
  `openHelp` / `openNotifications`, keyed by `helpModalID` /
  `notificationsModalID`.
- When a modal is active (`modal.Active()`), the shell routes keys to it
  through `routeHelpKey` / `routeNotificationsKey`. The overlay closes on
  `esc` only (the help close key dropped its former `q` binding), and the
  global `q` / `ctrl+c` quit binding stays live the whole time the overlay
  is open.
- The notifications modal renders through `modal.WithCaption("Notifications")`
  with columns renamed to `Level` / `Message` / `Time`.

## Consequences

- The global quit affordance is never shadowed by a transient dialog, which
  is the behavior users expect from a help/notifications popup.
- Help and notifications no longer occupy the navigation stack, so they
  cannot be accidentally pushed under another screen or popped out of
  order; dismissing returns to exactly the prior screen.
- The shell carries two more fields and a small amount of routing, but the
  routing is uniform (both overlays follow the same open/route/close
  shape), and the `modal` component absorbs the rendering.
- ADR 0011's note that "task 0023 replaced `infoStub`/`notificationsStub`
  with `infoScreen`/`notificationsScreen`" is now historical: those screens
  no longer exist as stack entries.

## References

- `internal/tui/shell/shell.go` (`helpModal`, `notificationsModal`,
  `openHelp`, `openNotifications`, `routeHelpKey`, `routeNotificationsKey`)
- `internal/tui/components/modal/modal.go` (`WithCaption`, `Active`)
- ADR 0011 (TUI screen router), ADR 0007 (rendering / modal component)
