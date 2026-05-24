# Charm Guidelines

Related: [TUI](./tui.md), [Clean Architecture](./clean_architecture.md),
[Clean Code](./clean_code.md), [Go](./go.md), [Domain Model](./domain_model.md),
and [Errors](./errors.md).

[`tui.md`](./tui.md) defines the boundary between TUI and business code. This
guideline defines the internal shape of the TUI layer itself: how to assemble
Bubble Tea programs, when to reach for which Bubbles component, how to share
Lipgloss styles, and when to embed `huh` forms inside a larger model. Read it
together with `tui.md`; this file does not repeat the layering rules already
captured there.

## The Charm Stack

`agentfiles` uses four Charm libraries. Treat their roles as fixed.

| Library     | Role                                                                  |
| ----------- | --------------------------------------------------------------------- |
| `bubbletea` | Runtime: Model–Update–View loop, message dispatch, terminal lifecycle |
| `bubbles`   | Reusable components: list, table, viewport, textinput, help, key, ... |
| `lipgloss`  | Styling and layout: borders, color, ANSI-aware measurement, join      |
| `huh`       | Form builder for "ask a few questions" flows                          |

Imports use the v2 module paths: `charm.land/bubbletea/v2`,
`charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, `charm.land/huh/v2`. Older
examples on the web still show v1 (`github.com/charmbracelet/...` without
`/v2`); port them before copying. v2 splits keyboard and mouse messages, moves
alt-screen and mouse mode into `tea.View`, and removes the global Lipgloss
renderer.

## Application Shell

`agentfiles` is a multi-view app. **Multi-view apps always run in the
alternate screen buffer.** The alt buffer keeps the user's scrollback intact
when the app exits and gives the renderer a stable canvas to clear and
redraw.

In v2, alt-screen is declared on the `tea.View` returned by `View()`, not as a
program option:

```go
func (m model) View() tea.View {
    v := tea.NewView(m.body())
    v.AltScreen = true
    if m.mouseEnabled {
        v.MouseMode = tea.MouseModeCellMotion
    }
    return v
}
```

Single-screen utilities (a one-shot prompt, a `huh.Form` running standalone)
may stay in the inline buffer. Anything that switches between two or more
screens must enable `AltScreen`.

```text
Do:
- enable AltScreen for any app with more than one screen, list, or pager
- centralize program construction in one place (typically the `internal/tui`
  entry point) so program-level options stay consistent
- prefer the default 60 FPS renderer; only tune `WithFPS` if measurements show
  a problem
```

```text
Don't:
- toggle alt-screen mid-flight unless the user is being handed off to an
  external process (use `tea.ExecProcess` for that)
- request mouse support that the app cannot actually use; keyboard must remain
  the primary input path
```

## Multi-View State Machines

Every business entity that has its own listing, detail view, or workflow gets
its own view. **Each entity's use cases start from that entity's view.** The
top-level model is a state machine that owns the view enum and the per-view
sub-models:

```go
type viewID int

const (
    viewProfile viewID = iota
    viewProject
    viewAsset
)

type root struct {
    common  *commonModel
    view    viewID
    profile profileView
    project projectView
    asset   assetView
}
```

Route messages in `Update` by inspecting `m.view`:

```go
switch m.view {
case viewProfile:
    m.profile, cmd = m.profile.Update(msg)
case viewProject:
    m.project, cmd = m.project.Update(msg)
case viewAsset:
    m.asset, cmd = m.asset.Update(msg)
}
```

The same shape applies in `View()`. View transitions happen by assigning to
`m.view`; the next message routes to the new sub-model automatically.

```text
Do:
- name views after the entity they show (`ProfileView`, `ProjectView`)
- keep the view enum and the sub-model fields next to each other so the set
  is easy to enumerate
- start every workflow for entity X from its view: listing profiles, creating
  a profile, editing a profile all begin on `ProfileView`
- pass per-view typed messages back to the root when a transition must happen
  (for example, `openProjectMsg{projectID string}`)
```

```text
Don't:
- type-switch on `interface{ tea.Model }` at the root; concrete fields are
  simpler and let the compiler catch wiring mistakes
- use one giant model with global flags for "are we in profile mode now"
- spread profile-related actions across asset or project views
```

## Reusable Entity Components

A view shows an entity. When the same entity appears on two screens (a profile
summary on `ProjectView`, a profile picker inside a form), build one
component and reuse it. Components should expose a small, concrete API:

```go
type ProfileSummary struct {
    profile *profile.Profile
    width   int
    styles  *Styles
}

func NewProfileSummary(p *profile.Profile, s *Styles) ProfileSummary { ... }
func (m ProfileSummary) Update(msg tea.Msg) (ProfileSummary, tea.Cmd) { ... }
func (m ProfileSummary) View() string                                 { ... }
func (m *ProfileSummary) SetSize(w, h int)                            { ... }
```

The pager and stash in Glow follow this pattern: each is a concrete type with
`New`, `Update`, `View`, and `setSize`, held as a field by the root model and
called from its loop.

```text
Do:
- export a concrete `Model` type per entity component with `New`, `Update`,
  `View`, `SetSize` methods
- give every component a constructor that takes the dependencies it needs
  (entity data, shared styles, common state) and nothing else
- reuse the same `*Summary`, `*List`, or `*Picker` component wherever that
  entity shows up
```

```text
Don't:
- copy-paste an entity's rendering between views; copies will drift
- hide arbitrary I/O inside a component constructor — pass it data, don't make
  it fetch
- introduce a generic `EntityView[T]` interface when concrete types are
  enough; reach for interfaces only when you have a real second implementation
```

## Composing Sub-Models

When a parent holds child models, the loop is mechanical:

```go
func (m parent) Update(msg tea.Msg) (parent, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.common.width = msg.Width
        m.common.height = msg.Height
        m.left.SetSize(msg.Width/2, msg.Height)
        m.right.SetSize(msg.Width/2, msg.Height)
    case tea.KeyPressMsg:
        if cmd, handled := m.handleGlobalKey(msg); handled {
            return m, cmd
        }
    }

    var cmd tea.Cmd
    m.left, cmd = m.left.Update(msg)
    cmds = append(cmds, cmd)
    m.right, cmd = m.right.Update(msg)
    cmds = append(cmds, cmd)

    return m, tea.Batch(cmds...)
}
```

Handle global keys (`q`, `ctrl+c`, `?`, focus switch) at the parent **before**
forwarding to children, so a focused textarea does not eat `tab` and prevent
focus rotation.

Use `tea.Batch(...)` for independent concurrent commands. Use `tea.Sequence(...)`
only when ordering matters (e.g., "save, then quit"). Do not start goroutines
inside `Update`; return a `tea.Cmd` that performs the I/O.

## Window Size Propagation

Bubble Tea sends `tea.WindowSizeMsg` at startup and on every SIGWINCH. The
root model must catch it and push the new dimensions through to every
component that has its own layout (viewports, tables, lists, textareas,
`help.Model`):

```go
case tea.WindowSizeMsg:
    m.common.width, m.common.height = msg.Width, msg.Height
    m.list.SetSize(msg.Width-frame, msg.Height-statusBar-helpBar)
    m.help.SetWidth(msg.Width)
```

Always measure rendered strings with `lipgloss.Width` / `lipgloss.Height`
(not `len`). They are ANSI-aware and respect wide characters.

```text
Do:
- recompute layout in `Update` on `WindowSizeMsg` and stash the dimensions on
  a shared `commonModel` that children can read
- set `help.Model.SetWidth(msg.Width)` so help truncates gracefully on narrow
  terminals
- use `MaxWidth` / `MaxHeight` to hard-cap component bounds when wrapping
  would damage layout
```

```text
Don't:
- assume 80 columns; ids, paths, and error messages are routinely longer
- cache rendered output across frames; re-render in `View` every time
- measure ANSI-styled strings with `len`
```

## Focus Model

When more than one component is visible, the user must be able to tell at a
glance which one is focused. Track focus on the parent and signal it
visually:

```go
type splitView struct {
    focus  int
    panes  []paneModel
    styles *Styles
}

func (m splitView) View() string {
    out := make([]string, len(m.panes))
    for i, p := range m.panes {
        base := m.styles.PaneBlurred
        if i == m.focus {
            base = m.styles.PaneFocused
        }
        out[i] = base.Render(p.View())
    }
    return lipgloss.JoinHorizontal(lipgloss.Top, out...)
}
```

Bubbles components have built-in `Focused` and `Blurred` style variants
(`textinput`, `textarea`, `table`). Use them:

```go
ta := textarea.New()
s := ta.Styles()
s.Focused.Base = styles.BorderFocused
s.Blurred.Base = styles.BorderBlurred
ta.SetStyles(s)
```

Focus changes are an action: call `Blur()` on the outgoing component and
`Focus()` on the incoming one. `Focus()` returns a `tea.Cmd` (cursor blink);
`Blur()` does not. Forward `tea.Cmd` results through `tea.Batch`.

```text
Do:
- show focus with a clearly different border, color, or background — never
  with color alone
- back focus with text or a glyph somewhere (a `[ active ]` tag in the title,
  or a `>` in the status bar) so screen readers can describe it
- route all messages to all children every frame; let unfocused components
  ignore the input
```

```text
Don't:
- rely solely on cursor blink to indicate focus
- forward `tab` to a focused textarea before the parent has had a chance to
  consume it
- hold focus state inside more than one place; the parent owns it
```

## Modal Overlays

Some flows are short, focused interventions on top of the current screen
(confirm a delete, run a wizard, pick a profile). Render those as
**modals**: layered over the background, taking exclusive focus, dismissed
only when they resolve. Reference: `bubbletea/examples/clickable/main.go:225-256`.

A modal differs from a sub-view in three ways:

1. It draws on top of the current screen rather than replacing it.
2. It owns input while open — the background is inert.
3. It resolves with a typed message carrying a value (confirmed) or
   nothing (cancelled), then disappears.

### Layering With The Compositor

`lipgloss.Layer` is not just a string. It carries `(x, y, z)` coordinates
and a slice of child layers. `lipgloss.Compositor` flattens that tree, sorts
by z-index, and renders to the single string `tea.View.Content` expects.

```go
func (m model) View() tea.View {
    bg := m.body()
    v := tea.NewView(bg)
    v.AltScreen = true
    if m.modal != nil {
        v.SetContent(m.modal.Render(bg, m.width, m.height))
    }
    return v
}
```

Internally, `Render` builds a layer tree and composes it; higher z renders
on top:

```go
root := lipgloss.NewLayer(background).ID("modal-background")
root.AddLayers(modalLayer.Z(10))
return lipgloss.NewCompositor(root).Render()
```

When the overlay needs mouse interaction, give every clickable layer a
stable ID and route hits with `Compositor.Hit(x, y)`. See the `clickable`
example for the full pattern (declare `tea.View.MouseMode` and translate
hits into typed messages from `tea.View.OnMouse`).

### Focus Stealing

While a modal is open, the root model routes **every** message to it and
returns. No background view sees the message, so its key bindings, tables,
and text inputs go inert without any per-component `SetEnabled(false)` work.

```go
func (m root) Update(msg tea.Msg) (root, tea.Cmd) {
    // Global handlers (ctrl+c, WindowSizeMsg, ResolvedMsg) first.

    if m.modal != nil {
        var cmd tea.Cmd
        m.modal, cmd = m.modal.Update(msg)
        return m, cmd
    }

    // Normal background routing.
}
```

Keep `tea.WindowSizeMsg`, `ctrl+c`, and the modal's resolution message at
the very top of `Update`, *above* the focus-stealing branch. The background
dimensions still need to update under the modal (it re-centers on the next
render) and the user must always be able to abort.

### Resolution Contract

A modal must signal completion as a typed message — never as a mutated
field the parent polls. The parent reacts to the message, reads the
payload, and clears its modal field:

```go
type ModalResolvedMsg struct {
    ID        string
    Confirmed bool
    Value     any
}

case ModalResolvedMsg:
    if msg.Confirmed {
        m.apply(msg.Value.(*huh.Form))
    }
    m.modal = nil
```

`Confirmed=false` (cancel) is a no-op on the parent. The `Value` field
carries the typed payload the caller needs — for a `huh.Form` modal, that
is the form itself, so the parent can read fields via `form.GetString(key)`
and friends.

### Embedded huh Forms In Modals

The most common modal hosts a `huh.Form`. Map its terminal states to the
resolution contract:

| huh.Form state       | Modal result          |
| -------------------- | --------------------- |
| `huh.StateCompleted` | Confirmed, Value=form |
| `huh.StateAborted`   | Cancelled (esc)       |

Gotcha: a `huh.NewConfirm` field with `Affirmative`/`Negative` buttons
submits the form on **either** button — both transition the form to
`StateCompleted`. To distinguish "save" from "cancel" the parent must read
the bool with `form.GetBool(key)` in the resolved handler. Treating
`StateCompleted` alone as "save" silently saves whatever the user filled
in even when they clicked the negative button.

```text
Do:
- treat the modal as opaque from the background's perspective: open it,
  wait for ResolvedMsg, react to the payload
- give every modal a stable ID so the parent can route its ResolvedMsg
- center the modal against the parent's current width and height; recompute
  on every WindowSizeMsg
- match the cross-entity key convention: esc cancels, enter confirms when
  the choice is unambiguous
- render the modal with a visible border or contrast background so it reads
  as separate from the layer below
- read huh.Confirm choices via form.GetBool(key), not via form.State alone
```

```text
Don't:
- forward keys to the background "just in case" — focus stealing must be
  total or it isn't focus stealing
- use a modal for long-lived state; if the user revisits it, it belongs on
  its own view
- stack modals deeper than two layers without a strong reason
- pull the open/close decision into the modal itself; the parent owns the
  lifecycle and the resolution payload
```

## Key Bindings — Consistent and Visible

Every screen with more than basic Enter/Esc/quit behavior defines a key map
with the `bubbles/key` package and renders it through `bubbles/help`. This is
the **only** sanctioned way to expose available actions to the user.

Define a `KeyMap` struct, implement `ShortHelp` and `FullHelp`, and store a
`help.Model` on the parent that owns the keys:

```go
type profileKeys struct {
    Up     key.Binding
    Down   key.Binding
    Create key.Binding
    Edit   key.Binding
    Delete key.Binding
    Open   key.Binding
    Help   key.Binding
    Back   key.Binding
    Quit   key.Binding
}

func (k profileKeys) ShortHelp() []key.Binding {
    return []key.Binding{k.Create, k.Edit, k.Delete, k.Help, k.Quit}
}

func (k profileKeys) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.Open},
        {k.Create, k.Edit, k.Delete},
        {k.Help, k.Back, k.Quit},
    }
}
```

Bindings carry their own display text:

```go
Create: key.NewBinding(
    key.WithKeys("c"),
    key.WithHelp("c", "create profile"),
),
```

In `Update`, match keys with `key.Matches(msg, m.keys.Create)` — never compare
raw strings. In `View`, render the help bar with `m.help.View(m.keys)` and
toggle expanded help on `?` by flipping `m.help.ShowAll`.

### Disabling Bindings In Context

When an action is not currently valid (no row selected, no unsaved changes),
disable its binding. The help bar then automatically hides it and the
`key.Matches` check returns false:

```go
m.keys.Edit.SetEnabled(m.list.SelectedItem() != nil)
m.keys.Delete.SetEnabled(m.list.SelectedItem() != nil)
m.keys.Save.SetEnabled(m.dirty)
```

Recompute these whenever the underlying state changes, so the next render is
truthful.

### Cross-Entity Key Convention

Keep verbs consistent across every entity that supports them. A user who
learns `c` creates a profile should expect `c` to create a project too.

| Key         | Action                                | Where it applies                |
| ----------- | ------------------------------------- | ------------------------------- |
| `c`         | Create new entity                     | every entity list view          |
| `e`         | Edit selected entity                  | every entity list / detail view |
| `d`         | Delete selected entity (with confirm) | every entity list view          |
| `r`         | Rename selected entity                | where rename is distinct        |
| `enter`     | Open / drill into selected entity     | every list view                 |
| `esc`       | Cancel current action / dismiss modal | everywhere                      |
| `backspace` | Back to previous view                 | nested views                    |
| `/`         | Filter / search the current list      | list views                      |
| `?`         | Toggle full help                      | every screen                    |
| `q`         | Quit (only on root views)             | root views                      |
| `ctrl+c`    | Quit from anywhere                    | everywhere                      |
| `tab`       | Next focus                            | multi-pane screens              |
| `shift+tab` | Previous focus                        | multi-pane screens              |

If a screen needs a verb that is not in the table, pick a mnemonic letter and
record it next to the others so it can be reused for the same action on the
next entity that needs it.

```text
Do:
- match keys with `key.Matches`, not by `msg.String()`
- give every binding a `WithHelp(keyLabel, description)` so the help view is
  self-describing
- render `help.Model` on every screen that has more than Enter/Esc/Quit
- gate destructive bindings (`d`) behind a `Confirm` step
- call `SetEnabled` whenever an action becomes valid or invalid
```

```text
Don't:
- invent a new key for "create" on one screen and a different one on another
- show bindings in the help bar that do nothing in the current state
- require chorded shortcuts (`ctrl+alt+x`) when a single letter would do
- override globally meaningful keys (`q`, `?`, `ctrl+c`) inside child
  components
```

## Asynchronous Work — Commands and Typed Messages

I/O happens in `tea.Cmd` closures that return a typed message. Update routes
on the message type. This keeps `Update` synchronous and testable.

```go
type profilesLoadedMsg struct{ profiles []*profile.Profile }
type loadFailedMsg struct{ err errs.DomainError }

func loadProfiles(svc *app.Service) tea.Cmd {
    return func() tea.Msg {
        ps, err := svc.ListProfiles()
        if err != nil {
            return loadFailedMsg{err: err}
        }
        return profilesLoadedMsg{profiles: ps}
    }
}
```

In `Update`:

```go
case profilesLoadedMsg:
    m.profiles = msg.profiles
    return m, nil
case loadFailedMsg:
    m.err = msg.err
    return m, nil
```

```text
Do:
- define a named struct for every async result, including failure
- preserve `errs.DomainError` values inside failure messages so the TUI can
  pick a severity and render the right color and icon (see `errors.md`)
- start spinners on the same `Cmd` that launches the work and stop them on
  the result message
- use `tea.Tick` / `tea.Every` for periodic UI updates, never `time.Sleep`
  inside a command
```

```text
Don't:
- start goroutines inside `Update`
- perform blocking I/O directly in `Update` or `View`
- collapse success and failure into a single message with an `err` field; the
  type switch should make the two paths obvious
- use commands merely to move data within one update cycle; assign to the
  model directly
```

## Embedding `huh` Forms

`huh` is a Bubble Tea model itself. When a form is one step inside a larger
workflow, embed `*huh.Form` as a field on the screen model and forward its
lifecycle:

```go
type createProfileView struct {
    form *huh.Form
    done bool
}

func newCreateProfileView() createProfileView {
    return createProfileView{
        form: huh.NewForm(
            huh.NewGroup(
                huh.NewInput().Key("name").Title("Profile name").
                    Validate(huh.ValidateNotEmpty()),
                huh.NewSelect[string]().Key("kind").Title("Kind").
                    Options(huh.NewOption("Library", "library"),
                            huh.NewOption("Service", "service")),
            ),
        ),
    }
}

func (m createProfileView) Init() tea.Cmd { return m.form.Init() }

func (m createProfileView) Update(msg tea.Msg) (createProfileView, tea.Cmd) {
    form, cmd := m.form.Update(msg)
    if f, ok := form.(*huh.Form); ok {
        m.form = f
    }
    if m.form.State == huh.StateCompleted && !m.done {
        m.done = true
        return m, submitCreateProfile(m.form.GetString("name"), m.form.GetString("kind"))
    }
    return m, cmd
}

func (m createProfileView) View() string { return m.form.View() }
```

Read values back with `form.GetString(key)` / `GetInt` / `GetBool` after
`form.State == huh.StateCompleted`. Set the key with `.Key("name")` on the
field. Option values must be stable ids; option labels are the human text.

For standalone, single-purpose prompts (one CLI invocation, no surrounding
TUI), call `huh.NewForm(...).Run()` directly. Apply `WithTheme(...)` with the
detected background and honor the project's accessible-mode toggle.

```text
Do:
- group fields into one `huh.Group` per logical step; let the user advance
  group by group
- use `huh.Confirm` for binary choices; reserve `Select` for three or more
  options
- bind values with `.Value(&v)` for standalone forms, `.Key("name")` for
  embedded ones
- write field validators that check shape (`required`, `min length`); leave
  domain validation (id collision, ownership) to the app layer
```

```text
Don't:
- mix `Value(&v)` and `GetString` for the same field; pick one
- run a `huh.Form` standalone when the surrounding view is already a Bubble
  Tea program — embed it instead, so global keys (`q`, `?`) still work
- duplicate option lists that already have a domain owner; build options
  from the canonical source
```

## Lipgloss Styling

Define styles once in a shared module and pass them to components. Avoid
inline `lipgloss.NewStyle().Bold(true).Foreground(...)` chains inside hot
rendering paths.

```go
type Styles struct {
    Title          lipgloss.Style
    Muted          lipgloss.Style
    Selected       lipgloss.Style
    BorderFocused  lipgloss.Style
    BorderBlurred  lipgloss.Style
    Error          lipgloss.Style
    Success        lipgloss.Style
}

func NewStyles(hasDarkBackground bool) *Styles {
    lightDark := lipgloss.LightDark(hasDarkBackground)
    return &Styles{
        Title: lipgloss.NewStyle().Bold(true).
            Foreground(lightDark(lipgloss.Color("#1f2937"), lipgloss.Color("#fafafa"))),
        // ...
    }
}
```

Detect background once at startup with `lipgloss.HasDarkBackground(os.Stdin,
os.Stdout)` and pass the result down. In v2 there is no global renderer; the
helper lives on each style construction.

Use the layout primitives:

- `lipgloss.JoinHorizontal(pos, blocks...)` for side-by-side panes.
- `lipgloss.JoinVertical(pos, blocks...)` for stacked sections.
- `lipgloss.Place(w, h, hPos, vPos, content)` for centering a single block.

Reach for the `lipgloss/table`, `lipgloss/list`, and `lipgloss/tree`
subpackages when the data fits a standard structure. For interactive lists
and tables, prefer the `bubbles` equivalents — they handle focus, filtering,
and key bindings.

```text
Do:
- define named styles for roles (title, muted, selected, focused, error)
- recompute layout in `View` from the latest dimensions; do not cache it
- use color plus an extra signal (border, weight, glyph) for every meaningful
  state
- measure with `lipgloss.Width` / `lipgloss.Height`
```

```text
Don't:
- attach business rules to styles ("if user is admin, bold"); branch outside
  the style, then apply the chosen style
- scatter inline `NewStyle()` chains across files
- rely on terminal width assumptions; recalculate on `WindowSizeMsg`
```

## Choosing A Bubbles Component

| Need                                                | Component               |
| --------------------------------------------------- | ----------------------- |
| Single-line input (name, search, command)           | `bubbles/textinput`     |
| Multi-line input (notes, body, code)                | `bubbles/textarea`      |
| Scrollable static content (rendered markdown, logs) | `bubbles/viewport`      |
| Selectable list with filter and status              | `bubbles/list`          |
| Tabular data, possibly with column sort             | `bubbles/table`         |
| Pick a file or directory                            | `bubbles/filepicker`    |
| Indicate indeterminate work                         | `bubbles/spinner`       |
| Indicate measurable progress                        | `bubbles/progress`      |
| Page through a fixed set of items                   | `bubbles/paginator`     |
| Bind keys and render help                           | `bubbles/key`, `/help`  |
| Time elapsed / countdown                            | `bubbles/stopwatch`, `bubbles/timer` |

Reuse a Bubbles component before reaching for a custom widget. Each component
ships its own `KeyMap`; expose those bindings through your screen's help if
they should be visible.

Common wiring gotchas:

- `Update` always returns the new component value. Assign it back:
  `m.list, cmd = m.list.Update(msg)`. Dropping the return is the most common
  bug.
- `Focus()` returns a `tea.Cmd` (blink). `Blur()` does not. Pipe the focus
  command through `tea.Batch`.
- `list.Model.SetSize(w, h)` and `table.Model.SetWidth/Height` must be called
  on every `WindowSizeMsg`.
- Spinner/progress components carry an internal id; create them once and
  store them on the model, do not reconstruct on every update.

## Quitting And Suspension

Quit cleanly through `tea.Quit`. Treat `ctrl+c` as a request to abort, not a
crash:

```go
case tea.KeyPressMsg:
    switch {
    case key.Matches(msg, m.keys.Quit):
        return m, tea.Quit
    case msg.String() == "ctrl+c":
        return m, tea.Interrupt
    }
case tea.InterruptMsg:
    // final cleanup, then quit
    return m, tea.Quit
```

If the user runs an external editor, hand control over with `tea.ExecProcess`
and let it return a typed message when the command finishes. The terminal is
restored automatically while the subprocess runs.

Suspend support (`ctrl+z`) is optional but trivial: return `tea.Suspend` on
the key and react to `tea.ResumeMsg` if the model needs to reset cursor or
animations.

## Testing

Drive `Update` directly with synthetic messages to verify state transitions
and key routing. Use fakes for the `app.Service` calls a command would make.
Snapshot tests against full ANSI output are brittle; assert on the smaller
piece of state or the substring you actually care about.

For form behavior, the underlying service tests cover domain validation;
TUI-level tests should cover only the wiring: that a completed `huh.Form`
emits the right submit command, that an error message places the model into
its error state, that resize messages re-flow children.

See [`testing.md`](./testing.md) for the project-wide testing conventions.

## Review Checklist

Before finishing a Charm-layer change, check:

1. Does every multi-view flow run in `AltScreen`?
2. Does each business entity have its own view, and do its workflows start
   there?
3. Are reusable entity components shared across views instead of copied?
4. Does `WindowSizeMsg` propagate to every child that has its own layout?
5. Is the focused component visually distinct in a way that is not color-only?
6. Are all non-trivial actions bound through `bubbles/key`, with help
   rendered via `bubbles/help`?
7. Do bindings follow the cross-entity key convention (`c` create, `e` edit,
   `d` delete, ...)?
8. Are bindings disabled (`SetEnabled(false)`) whenever the action is not
   currently valid?
9. Does every async command return a typed message, with failures carrying an
   `errs.DomainError`?
10. Are styles defined once and shared, with adaptive colors driven by the
    detected background?
11. For screens that use a modal: does the parent route messages to the
    modal before any background dispatch, clear the modal field on its
    typed resolve message, and re-center the modal on `WindowSizeMsg`?

## References

- [Bubble Tea](https://pkg.go.dev/charm.land/bubbletea/v2) and its
  [v2 upgrade guide](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [Lip Gloss](https://pkg.go.dev/charm.land/lipgloss/v2) and its
  [v2 upgrade guide](https://github.com/charmbracelet/lipgloss/blob/main/UPGRADE_GUIDE_V2.md)
- [Huh](https://pkg.go.dev/charm.land/huh/v2) and its
  [v2 upgrade guide](https://github.com/charmbracelet/huh/blob/main/UPGRADE_GUIDE_V2.md)
- [Commands in Bubble Tea (Charm blog)](https://charm.land/blog/commands-in-bubbletea/)
- [Glow](https://github.com/charmbracelet/glow) — a production-grade reference
  app built on the same stack
