# 0014 changes

Added a reusable modal-overlay component at
`internal/tui/components/modal`. The package wraps a `Content` interface
(anything with `Init/Update/View/Done`), centers it as a `lipgloss.Layer`
via the v2 compositor, and resolves through a typed
`modal.ResolvedMsg{ID, Confirmed, Value}`. A `huh.Form` adapter
(`modal.NewForm`) maps `huh.StateCompleted` → confirmed-with-form-payload
and `huh.StateAborted` → cancelled, so the parent can keep all
form-state knowledge behind the modal lifecycle.

The implementation is a port of a reference that lives outside the repo
at `~/projects/charm/go-playground/modal/`. It already matches the
"Modal Overlays" section of [`docs/guidelines/charm.md`](../guidelines/charm.md)
line-for-line, so no guideline or ADR update was needed — the guideline
was authored describing exactly this pattern. No existing TUI flow
consumes the component yet; that wiring will come from task 0002
(`use-wizards`).

## Decisions

- **Reuse the go-playground reference verbatim instead of redesigning.** —
  **Why:** the reference already aligns with the project's `charm.md`
  guideline and was the spec the user pointed at. Rewriting it would
  drift from what the guideline already documents.
- **Default to package-level inline `defaultStyle` rather than threading a
  shared `tui.Styles`.** — **Why:** no shared `Styles` value exists in
  the project yet, and forcing one would expand scope. Callers can
  override with `modal.WithStyle(...)` when a coherent theme arrives.
- **Drive tests via a hand-rolled `fakeContent`, not Bubble Tea's
  runtime.** — **Why:** `testing.md` says drive `Update` synchronously;
  Bubble Tea's event loop adds nothing to unit-level coverage of the
  resolution contract.
- **Use `huh.Form.State` as a settable seam in `form_test.go`.** —
  **Why:** the field is exported, so we can assert the state-to-resolution
  mapping without simulating an entire keyboard session. The library
  itself owns testing the full form lifecycle.

Considered and rejected:

- Adding a `cmd/modal-demo` binary to manually exercise the overlay —
  the consuming task (0002) will exercise it inside the real TUI, and a
  one-off demo would just be a maintenance burden.
- Updating `docs/guidelines/charm.md` — its `Modal Overlays` section
  was already written against this design, so changes would be
  busywork rather than substance.
- Splitting `Render` into `Place + Compose` to expose layout/composition
  separately — `Layer` already exposes the positioned layer for callers
  that want to build their own tree; the split would add two API
  surfaces for no current caller.

## Assumptions

- **`charm.land/{bubbletea,lipgloss,huh}/v2` are the canonical import
  paths.** — **Why:** verified during the v2 upgrade earlier this
  session; the v2 modules declare `module charm.land/...` and the
  `github.com/charmbracelet/.../v2` mirror is the same code under a
  different name. Staying on `charm.land` matches the rest of the
  package's imports.
- **The new package is a leaf with no dependency on the rest of
  `internal/`.** — **Why:** `clean_architecture.md` calls out
  Acyclic Dependencies; keeping `modal` free of project-internal
  imports means the rest of the TUI can pull it in without cycle risk
  later.
- **`tea.BatchMsg` is the right shape to walk in test helpers.** —
  **Why:** when `Content.Update` returns a non-nil cmd, `Modal.Update`
  batches it with the resolve cmd; the test helper has to be able to
  pull a `ResolvedMsg` out of either a bare cmd or a batch.

## Other Notes

- No ADR added — no architectural decision is changing; the pattern was
  already approved by `charm.md`.
- No doc updates beyond the changelog and the task's own `plan.md`.

## new package: `internal/tui/components/modal`

Two source files plus tests.

`modal.go` defines the core surface:

```go
type Content interface {
    Init() tea.Cmd
    Update(tea.Msg) (Content, tea.Cmd)
    View() string
    Done() (done, confirmed bool, value any)
}

type ResolvedMsg struct {
    ID        string
    Confirmed bool
    Value     any
}

type Modal struct { /* id, content, style, z, resolved */ }

func New(id string, content Content, opts ...Option) *Modal
func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd)
func (m *Modal) Render(background string, w, h int) string
```

`form.go` wires `*huh.Form` into the `Content` interface:

```go
func (f *formContent) Done() (done, confirmed bool, value any) {
    switch f.form.State {
    case huh.StateCompleted:
        return true, true, f.form
    case huh.StateAborted:
        return true, false, nil
    default:
        return false, false, nil
    }
}

func NewForm(id string, form *huh.Form, opts ...Option) *Modal
```

## tests

`modal_test.go` covers the lifecycle with a programmable
`fakeContent`:

- forwarding of messages to content
- no `ResolvedMsg` while active
- `ResolvedMsg` carries id + confirmed + value on done
- cancel maps to `Confirmed=false, Value=nil`
- post-resolution updates are inert (no cmd, no content call)
- `Render` composites both background and modal body (verified with
  `ansi.Strip`, mirroring `internal/tui/render_*_test.go`)

`form_test.go` is table-driven over `huh.FormState`:

- `StateCompleted` → `(done, confirmed, form)`
- `StateAborted` → `(done, !confirmed, nil)`
- `StateNormal` → `(!done, _, _)`

Plus one end-to-end check that `NewForm` + `Update` produces a
`ResolvedMsg` whose `Value` is the exact `*huh.Form` instance handed in
on construction.
