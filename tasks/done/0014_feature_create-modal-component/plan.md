# Plan — Task 0014 · Modal Component

Cross-links:

- Task description: [`./description.md`](./description.md)
- Modal guideline (already written, prescribes this exact pattern):
  [`../../../docs/guidelines/charm.md`](../../../docs/guidelines/charm.md)
  §`Modal Overlays`
- Reference implementation to port:
  `~/projects/charm/go-playground/modal/{modal.go,form.go}` (outside the repo)

## Context

The task asks for a reusable Modal component that can host `huh` forms, since
Charm's stack does not ship one. `docs/guidelines/charm.md` already documents
the desired pattern in detail (compositor layering, focus stealing, typed
`ResolvedMsg`, `huh.StateCompleted`/`StateAborted` mapping). A working
reference exists at `~/projects/charm/go-playground/modal/` and is the
implementation we want to port — it matches the guideline almost
line-for-line. The project just upgraded to Charm v2
(`charm.land/.../v2`), so the reference's imports are already aligned.

This task only delivers the building block
(`internal/tui/components/modal`). Wiring it into existing TUI flows is out
of scope (other current tasks like 0002 `use-wizards` will consume it).

## Files

Create (new):

- `internal/tui/components/modal/modal.go` — core `Modal`, `Content`
  interface, `ResolvedMsg`, `New`, `Option`/`WithStyle`/`WithZ`, `ID`,
  `Init`, `Update`, `View`, `Layer`, `Render`.
- `internal/tui/components/modal/form.go` — `formContent` adapter +
  `NewForm` constructor for `*huh.Form`.
- `internal/tui/components/modal/modal_test.go` — tests for the core
  lifecycle.
- `internal/tui/components/modal/form_test.go` — tests for the huh-form
  adapter.

Do **not** touch (out of scope):

- `internal/tui/tui.go`, `internal/tui/forms.go` — no callers wire the
  modal yet.
- `docs/guidelines/charm.md` — already describes this pattern; no doc
  update needed.
- ADRs — no architectural decision is changing; we are implementing a
  pattern already approved by the guideline.

## Step-by-step

1. **Create package directory** `internal/tui/components/modal/`.

2. **Port `modal.go`** from the reference. Substantive shape stays
   identical:
   - `type Content interface { Init() tea.Cmd; Update(tea.Msg) (Content, tea.Cmd); View() string; Done() (done, confirmed bool, value any) }`
   - `type ResolvedMsg struct { ID string; Confirmed bool; Value any }`
   - `type Modal struct { id string; content Content; style lipgloss.Style; z int; resolved bool }`
   - `New(id, content, opts...) *Modal`, with `Option`, `WithStyle`,
     `WithZ`.
   - `Init`, `Update` (no-op after `resolved`; emits `ResolvedMsg` on first
     `Done`), `View`, `Layer`, `Render`.
   - Default style: rounded border, single padding, accent foreground —
     kept inline as the reference does, since it is the only styling and
     there is no shared `tui.Styles` value to plug into yet.
   - Package doc comment retained (mirrors the reference) — it functions as
     the "how to use" for callers.

3. **Port `form.go`** from the reference. `formContent` adapter:
   - `Init`/`Update`/`View` forward to `*huh.Form`.
   - `Done()` maps `huh.StateCompleted` → `(true, true, form)`,
     `huh.StateAborted` → `(true, false, nil)`, otherwise
     `(false, false, nil)`.
   - `NewForm(id, *huh.Form, opts...) *Modal` wraps `formContent` in
     `New`.

4. **Write `modal_test.go`.** Drive `Update` synchronously with synthetic
   state (no Bubble Tea program). Cases (one behavior per test, matching
   `testing.md`):
   - `TestModal_UpdateForwardsToContent` — using a hand-rolled fake
     `Content`, send a message and assert it was received.
   - `TestModal_EmitsResolvedMsgWhenContentDone` — fake reports
     `done=true, confirmed=true, value="payload"`; assert the returned
     `tea.Cmd` produces a
     `ResolvedMsg{ID, Confirmed:true, Value:"payload"}`.
   - `TestModal_CancelMapsToConfirmedFalse` — fake reports
     `done=true, confirmed=false`; assert `ResolvedMsg.Confirmed==false`.
   - `TestModal_NoResolvedMsgWhileActive` — fake reports `done=false`;
     cmd is whatever the fake returned (or nil) and never a `ResolvedMsg`.
   - `TestModal_UpdateIsInertAfterResolution` — drive one resolution,
     then send another message; assert no command and fake's `Update` was
     not called again.
   - `TestModal_RenderCompositesOverBackground` — assert the rendered
     output contains the fake content's view text and that the background
     string is present (after `ansi.Strip`, to mirror the in-repo
     convention from `render_*_test.go`).
   - `TestModal_IDReturnsConstructorID` — basic getter.

   Fake `Content` lives in the test file and exposes call counters /
   programmable `Done` return.

5. **Write `form_test.go`.** Skip running a real `huh.Form` event loop —
   that is the library's job. Test the adapter mapping by constructing a
   `*huh.Form` and using table-driven cases that set `form.State`
   directly:
   - `huh.StateCompleted` → `done=true, confirmed=true, value=form`.
   - `huh.StateAborted` → `done=true, confirmed=false, value=nil`.
   - default → `done=false`.

   If `huh.State` cannot be set externally on a built `*huh.Form`, fall
   back to a tiny test using
   `huh.NewForm(huh.NewGroup(huh.NewConfirm().Key("ok")))` and walk one
   message through to reach `StateCompleted`. Prefer the direct approach;
   switch to walking only if needed.

6. **Verify build, tests, lint.** From repo root:
   - `make build`
   - `make test` (must include the new package; expect cached pass for
     others)
   - `make lint`

   All three must finish clean before declaring the task done.

## Verification

End-to-end sanity:

1. `go test ./internal/tui/components/modal/...` — all new tests pass.
2. `go vet ./...` — clean.
3. The new package does not import anything outside
   `charm.land/{bubbletea,lipgloss,huh}/v2` and stdlib, keeping it a leaf
   component (Acyclic Dependencies Principle).
4. Visual smoke is deliberately deferred to the consuming task
   (0002 `use-wizards`) — that task will exercise the modal in the
   running TUI. We are not adding a demo `cmd/` binary for it.

## Out of scope

- Wiring the modal into `internal/tui/tui.go` or any existing flow.
- Multiple-modal stacking semantics beyond what `WithZ` already exposes.
- Mouse hit-testing through `Compositor.Hit` (charm.md notes the pattern
  but no flow needs it yet).
- Background-style theming or a shared `Styles` value — the default style
  is inline and replaceable via `WithStyle`.
- Updates to `docs/guidelines/charm.md` — its `Modal Overlays` section
  was authored describing this pattern and is already accurate.
