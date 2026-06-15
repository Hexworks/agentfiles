// Package focus provides a Handler that coordinates keyboard focus across a
// collection of Bubble Tea / bubbles / huh components.
//
// Navigation is keyboard-driven via Tab (forward) and Shift+Tab (backward)
// through the registered components in the order they were added.
//
// The handler accepts any component that satisfies the Focusable interface.
// huh.Field already matches that shape directly. For bubbles widgets whose
// Focus/Blur signatures differ (textinput.Blur, textarea.Blur, table.Focus,
// table.Blur all lack a tea.Cmd return), Add adapts them automatically. For
// widgets that lack Focus/Blur entirely (list.Model, viewport.Model, custom
// tree-tables) the caller writes its own Focusable adapter — typically a
// wrapper that toggles the widget's KeyMap.
//
// Lifecycle: the handler is expected to live as long as the view that owns
// it. When the view is dismissed, call Close to drop all references so the
// handler and its registered components become eligible for garbage
// collection.
package focus

import (
	"fmt"
	"log/slog"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Focusable is the uniform contract registered components must satisfy. The
// signature matches huh.Field exactly so huh fields slot in without an
// adapter. For widgets whose underlying methods do not match, Add wraps the
// common bubbles cases automatically; callers wrap everything else themselves
// before calling Add.
type Focusable interface {
	Focus() tea.Cmd
	Blur() tea.Cmd
}

// Handler coordinates focus across a fixed set of components.
//
// Exactly zero or one component is focused at any time. The handler starts
// neutral (Focused() returns -1) and stays neutral until the first Tab,
// Shift+Tab, or explicit FocusIndex call.
type Handler struct {
	entries  []*entry
	current  int
	logger   *slog.Logger
	tabKey   key.Binding
	shiftKey key.Binding
}

type entry struct {
	raw       any
	focusable Focusable
}

// Option configures a Handler at construction.
type Option func(*Handler)

// WithLogger overrides the default slog logger. Warnings (unsupported
// component types) are emitted at slog.LevelWarn.
func WithLogger(l *slog.Logger) Option {
	return func(h *Handler) { h.logger = l }
}

// New constructs an empty Handler.
func New(opts ...Option) *Handler {
	h := &Handler{
		current:  -1,
		logger:   slog.Default(),
		tabKey:   key.NewBinding(key.WithKeys("tab")),
		shiftKey: key.NewBinding(key.WithKeys("shift+tab")),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Add registers a component for Tab/Shift+Tab navigation. The order of Add
// calls determines the cycle order. The component must be one of the
// recognized types (Focusable, *textinput.Model, *textarea.Model,
// *table.Model); anything else is rejected with a warning and not registered.
func (h *Handler) Add(c any) {
	f, ok := adapt(c)
	if !ok {
		h.logger.Warn("focus: unsupported component type, skipping",
			"type", fmt.Sprintf("%T", c))
		return
	}
	h.entries = append(h.entries, &entry{raw: c, focusable: f})
}

// Remove unregisters a component. The match is by reference equality against
// the value originally passed to Add. Returns true if removed.
//
// If the removed component was focused, the handler returns to neutral.
func (h *Handler) Remove(c any) bool {
	for i, e := range h.entries {
		if e.raw != c {
			continue
		}
		h.entries = append(h.entries[:i], h.entries[i+1:]...)
		switch {
		case h.current == i:
			h.current = -1
		case h.current > i:
			h.current--
		}
		return true
	}
	return false
}

// Update intercepts Tab / Shift+Tab. Returns (handled, cmd). When handled is
// true the host should not forward the same message to the focused
// component — the handler consumed it. When handled is false the host is
// free to dispatch the message normally (typically to FocusedComponent).
func (h *Handler) Update(msg tea.Msg) (bool, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false, nil
	}
	if len(h.entries) == 0 {
		return false, nil
	}
	switch {
	case key.Matches(kp, h.tabKey):
		return true, h.focusIndex(h.next())
	case key.Matches(kp, h.shiftKey):
		return true, h.focusIndex(h.prev())
	}
	return false, nil
}

// Focused returns the index of the currently focused component, or -1 if
// none.
func (h *Handler) Focused() int { return h.current }

// FocusedComponent returns the raw component originally passed to Add for the
// currently focused entry, or nil if none. Hosts use this to route subsequent
// messages to the right widget without tracking indices themselves.
func (h *Handler) FocusedComponent() any {
	if h.current < 0 || h.current >= len(h.entries) {
		return nil
	}
	return h.entries[h.current].raw
}

// FocusIndex programmatically focuses the entry at i (blurring whatever was
// previously focused). Out-of-range returns nil with no effect.
func (h *Handler) FocusIndex(i int) tea.Cmd {
	return h.focusIndex(i)
}

// Close drops all references so the handler and its registered components
// become eligible for garbage collection. Call this when the owning view is
// dismissed. After Close the handler is unusable.
func (h *Handler) Close() {
	h.entries = nil
	h.current = -1
}

func (h *Handler) focusIndex(i int) tea.Cmd {
	if i < 0 || i >= len(h.entries) {
		return nil
	}
	if h.current == i {
		return nil
	}
	var blurCmd tea.Cmd
	if h.current >= 0 && h.current < len(h.entries) {
		blurCmd = h.entries[h.current].focusable.Blur()
	}
	h.current = i
	focusCmd := h.entries[i].focusable.Focus()
	return tea.Batch(blurCmd, focusCmd)
}

func (h *Handler) next() int {
	if h.current < 0 {
		return 0
	}
	return (h.current + 1) % len(h.entries)
}

func (h *Handler) prev() int {
	if h.current < 0 {
		return len(h.entries) - 1
	}
	return (h.current - 1 + len(h.entries)) % len(h.entries)
}

// adapt converts the supported component types to the Focusable shape. The
// Focusable case must come first so pre-wrapped widgets are used as-is; this
// also catches huh.Field values since Field's Focus/Blur signatures already
// match.
func adapt(c any) (Focusable, bool) {
	switch v := c.(type) {
	case Focusable:
		return v, true
	case *textinput.Model:
		return textInputAdapter{m: v}, true
	case *textarea.Model:
		return textAreaAdapter{m: v}, true
	case *table.Model:
		return tableAdapter{m: v}, true
	}
	return nil, false
}

type textInputAdapter struct{ m *textinput.Model }

func (a textInputAdapter) Focus() tea.Cmd { return a.m.Focus() }
func (a textInputAdapter) Blur() tea.Cmd  { a.m.Blur(); return nil }

type textAreaAdapter struct{ m *textarea.Model }

func (a textAreaAdapter) Focus() tea.Cmd { return a.m.Focus() }
func (a textAreaAdapter) Blur() tea.Cmd  { a.m.Blur(); return nil }

type tableAdapter struct{ m *table.Model }

func (a tableAdapter) Focus() tea.Cmd { a.m.Focus(); return nil }
func (a tableAdapter) Blur() tea.Cmd  { a.m.Blur(); return nil }
