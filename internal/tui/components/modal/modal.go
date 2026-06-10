// Package modal provides a reusable overlay-dialog component for Bubble Tea v2
// applications, built on top of the lipgloss v2 compositor.
//
// A Modal wraps any [Content] (typically a form) and is rendered as a layer
// stacked on top of the parent application via lipgloss.Compositor. While a
// modal is open the parent should route messages to it exclusively, which
// gives the modal exclusive focus until it resolves.
//
// Lifecycle:
//
//  1. Construct a Modal with [New] (or [NewForm] for a huh.Form).
//  2. Call [Modal.Init] once on open and batch its command into the parent's
//     return — that is what kicks off any startup work the content needs
//     (cursor blink, initial focus, …).
//  3. The parent owns the modal as an optional field (nil = closed).
//  4. On every parent Update, if the field is non-nil forward the message
//     to Modal.Update only, then watch the returned command for a [ResolvedMsg].
//  5. On ResolvedMsg, inspect Confirmed and Value, then clear the field.
//  6. On every parent View, if the field is non-nil call Modal.Render to
//     composite it over the background string.
package modal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// LifecycleState enumerates the three states a [Content] can be in. It
// replaces an earlier two-bool encoding so illegal combinations cannot be
// expressed.
//
// The name "lifecycle" is deliberate: this enum tracks where a piece of
// modal content sits in its open/confirmed/cancelled lifecycle. The
// unrelated `sync.Resolution` family models per-file apply decisions
// (overwrite/keep/delete) — keep the two vocabularies apart.
type LifecycleState int

const (
	// Active means the content has not yet resolved; the modal stays open.
	Active LifecycleState = iota
	// Confirmed means the user completed the content successfully; the
	// modal will emit a [ResolvedMsg] with Confirmed=true.
	Confirmed
	// Cancelled means the user dismissed the content; the modal will emit a
	// [ResolvedMsg] with Confirmed=false and Value=nil.
	Cancelled
)

// Content is anything that can live inside a [Modal]. It mirrors the standard
// Bubble Tea model lifecycle plus a [Content.Lifecycle] check that lets the
// modal know when to resolve.
type Content interface {
	Init() tea.Cmd
	Update(tea.Msg) (Content, tea.Cmd)
	View() string
	// Lifecycle reports the current state of the content. When state is
	// [Confirmed] the modal will emit a [ResolvedMsg] carrying value; when
	// state is [Cancelled] the modal emits a ResolvedMsg with Confirmed=false
	// and Value=nil regardless of what value is returned here.
	Lifecycle() (state LifecycleState, value any)
}

// ResolvedMsg is dispatched once when the modal's [Content] reports a terminal
// state. Parents should clear their modal field on receipt and react to
// Confirmed / Value as appropriate.
type ResolvedMsg struct {
	ID        string
	Confirmed bool
	Value     any
}

// Modal wraps a [Content] for rendering as an overlay layer. By default the
// modal centers on its parent canvas; callers that want explicit placement
// (e.g. anchor the dialog below the widget that opened it) pass
// [WithAnchor].
type Modal struct {
	id       string
	content  Content
	style    lipgloss.Style
	z        int
	resolved bool
	anchor   *anchorPoint
}

// anchorPoint stores an absolute top-left placement target supplied by
// [WithAnchor]. nil means "center on parent".
type anchorPoint struct{ x, y int }

// Option configures a [Modal] at construction time.
type Option func(*Modal)

// WithStyle wraps the content with the given lipgloss style (typically a
// bordered, padded box). Defaults to a near-monochrome rounded border with
// single-cell padding; pass a themed style here to integrate the modal with
// the rest of the app's palette.
func WithStyle(s lipgloss.Style) Option {
	return func(m *Modal) { m.style = s }
}

// WithZ sets the z-index of the modal layer. The default is 0; callers that
// stack multiple modals must set a distinct z explicitly so the compositor
// can order them.
func WithZ(z int) Option {
	return func(m *Modal) { m.z = z }
}

// WithAnchor places the modal at an explicit top-left coordinate in the
// parent canvas, overriding the default centering. Coordinates are clamped
// so the rendered modal stays within bounds: if the requested anchor would
// push the modal off the right or bottom edge, [Modal.Layer] shifts it back
// just enough to fit.
//
// Use this to attach the modal to a specific UI element, e.g. open a
// confirmation directly below the button that triggered it.
func WithAnchor(x, y int) Option {
	return func(m *Modal) { m.anchor = &anchorPoint{x: x, y: y} }
}

// defaultStyle returns a fresh, palette-neutral rounded-border style. The
// background color is left to the compositor and the foreground is unset so
// the modal inherits the terminal's defaults; callers that want a themed
// border pass [WithStyle].
func defaultStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)
}

// New constructs a Modal for the given Content. The id is used to identify
// the modal in [ResolvedMsg] (and as the layer ID for hit testing).
//
// content must be non-nil; passing nil panics at construction rather than
// deferring the nil-pointer dereference into Update or View.
func New(id string, content Content, opts ...Option) *Modal {
	if content == nil {
		panic("modal: nil content")
	}
	m := &Modal{
		id:      id,
		content: content,
		style:   defaultStyle(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Modal) ID() string { return m.id }

func (m *Modal) Init() tea.Cmd {
	return m.content.Init()
}

// Update forwards the message to the content. If the content reports it is
// done, Update also emits a [ResolvedMsg]. Once resolved the modal becomes
// inert — further Update calls are no-ops.
func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd) {
	if m.resolved {
		return m, nil
	}
	var cmd tea.Cmd
	m.content, cmd = m.content.Update(msg)
	state, value := m.content.Lifecycle()
	if state == Active {
		return m, cmd
	}
	m.resolved = true
	confirmed := state == Confirmed
	if !confirmed {
		value = nil
	}
	// id is aliased because the closure outlives Update; capturing m.id
	// through m would read whatever the field holds when the cmd actually
	// runs.
	id := m.id
	resolveCmd := func() tea.Msg {
		return ResolvedMsg{ID: id, Confirmed: confirmed, Value: value}
	}
	if cmd == nil {
		return m, resolveCmd
	}
	// Independent: ResolvedMsg and any follow-up from content may interleave.
	return m, tea.Batch(cmd, resolveCmd)
}

// View renders the styled content as a plain string (no compositing). Use
// [Modal.Layer] or [Modal.Render] for placement over a background.
func (m *Modal) View() string {
	return m.style.Render(m.content.View())
}

// Layer returns the modal as a positioned lipgloss layer, centered within a
// canvas of size (parentW, parentH). Use this when assembling your own layer
// tree; for the simple "background + one modal" case prefer [Modal.Render].
//
// If the rendered content exceeds the parent canvas, the layer is clamped to
// the top-left corner rather than positioned at a negative coordinate.
func (m *Modal) Layer(parentW, parentH int) *lipgloss.Layer {
	view := m.View()
	w := lipgloss.Width(view)
	h := lipgloss.Height(view)
	var x, y int
	if m.anchor != nil {
		x = m.anchor.x
		y = m.anchor.y
		if maxX := parentW - w; x > maxX {
			x = maxX
		}
		if maxY := parentH - h; y > maxY {
			y = maxY
		}
	} else {
		x = (parentW - w) / 2
		y = (parentH - h) / 2
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return lipgloss.NewLayer(view).ID(m.id).X(x).Y(y).Z(m.z)
}

// Render composites the modal over background and returns the final string
// ready to drop into tea.View.Content. parentW/parentH must match the
// dimensions of background so centering is correct.
func (m *Modal) Render(background string, parentW, parentH int) string {
	root := lipgloss.NewLayer(background).ID("modal-background")
	root.AddLayers(m.Layer(parentW, parentH))
	return lipgloss.NewCompositor(root).Render()
}
