---
id: 0047
type: bug
status: pending
topics: tui, charm
---

# Reload screen data on navigation

Applying a plan that adopts an unmanaged path (`UnknownAdopt`) registers a new
asset in the profile, but navigating back to the Edit Profile screen still
renders the old asset table — the adopted asset only appears after the screen
is re-entered from scratch.

## Background

`shell.Model.pushScreen` (`internal/tui/shell/shell.go:192`) runs the new
screen's `Init()` and batches a synthetic `tea.WindowSizeMsg`, so the *push*
direction always lands on freshly loaded data. `shell.Model.popScreen`
(`shell.go:276`) only zeroes and truncates the stack slot — the screen it
reveals keeps whatever it loaded when it was first pushed.

Every data screen already defines `Init() tea.Cmd { return s.loadCmd() }`
(`profiles`, `edit_profile`, `edit_asset`, `plan_project`,
`select_project_assets`, `settings`); `welcomeScreen.Init` returns `nil`. So
re-running `Init` on the revealed screen is exactly "reload my data", with no
interface change and no per-screen edits.

The fix makes reload structural rather than per-screen: a screen becoming
top-of-stack — by push **or** by pop-reveal — runs its `Init`. That turns
`Init` into a re-entrant contract every future screen must honor, which is
recorded in the `Screen` interface doc comment rather than a new ADR.

## Approach

- `popScreen` calls the revealed screen's `Init()` and, when the shell already
  has a window size, batches the same synthetic `tea.WindowSizeMsg` that
  `pushScreen` sends. Single-screen stacks stay a no-op.
- The reload is unconditional. Focus resets to index 0 and table cursors to
  row 0 — the same thing that already happens after every `mutationDoneMsg`,
  which also re-runs `loadCmd`.
- Document the re-entrancy contract on the `Screen` interface (next to the
  existing `InputFocused` rationale) and on `popScreen`.

## Acceptance Criteria

- [ ] Popping a two-screen stack re-runs the revealed screen's `Init`: push a
      sentinel screen, push a second screen, send `PopScreenMsg` → the
      sentinel's `Init` has run twice — `TestPopScreenRunsRevealedInit`.
- [ ] The revealed screen receives the synthetic `tea.WindowSizeMsg` carrying
      the shell's current width/height, mirroring the push path —
      `TestPopScreenResizesRevealedScreen`.
- [ ] `PopScreenMsg` on a single-screen stack changes nothing: the root screen's
      `Init` count stays at 1 and the stack still has one entry —
      `TestPopScreenSingleStackNoReload`.
- [ ] The `Screen` interface doc comment states that `Init` runs every time the
      screen becomes top-of-stack (push or pop-reveal) and must therefore be
      idempotent/reload-safe.

## Out of scope

- Cursor / focus preservation across the reload — the revealed screen lands on
  row 0, focus index 0.
- A separate `Screen.Reload()` method or a `ScreenRevealedMsg` broadcast; the
  fix reuses `Init` instead.
- Any change to individual screens' `loadCmd` / `handleLoaded` bodies —
  including `planProjectScreen.handleLoaded` resetting `ignoredPaths`,
  `unignored`, `pinned` and `showIgnored` when it is revealed.
- Reloading when a **modal** closes — modals are shell- or screen-owned
  overlays, not stack entries.
- Avoiding redundant reloads (e.g. `planProjectScreen` re-running a full
  `PlanProject` when a Settings screen above it is popped). Correct-by-default
  beats cached.
- ADR or arc42 changes; the contract is documented at the interface.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/shell -run 'TestPopScreenRunsRevealedInit|TestPopScreenResizesRevealedScreen|TestPopScreenSingleStackNoReload'`
  green.
- Smoke: `./bin/af` → Profiles → open a profile → open a project → Plan →
  pick an unmanaged (unknown) skill folder row → set its resolution to `Adopt`
  → Apply → the screen pops back to Edit Profile → the newly adopted asset is
  listed in the Assets table without leaving and re-entering the screen.
