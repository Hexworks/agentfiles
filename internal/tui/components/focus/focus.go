// Package focus provides a Handler that coordinates keyboard focus across a
// collection of Bubble Tea / bubbles / huh components.
//
// Two navigation styles are supported simultaneously:
//
//  1. Tab and Shift+Tab cycle forward and backward through the registered
//     components in the order they were added.
//  2. Mnemonic keys '0'..'9' jump directly to a registered component.
//     Registering a mnemonic returns a *mnemonic.Button — the host renders
//     this button wherever the [N] indicator should appear (typically inside a
//     panel border title), keeping placement decoupled from the focus logic.
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

	"github.com/hexworks/agentfiles/internal/tui/components/mnemonic"
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

// Modifier is the keyboard modifier prefix applied to mnemonic digits. It
// prevents the handler from stealing plain digit input destined for focused
// text widgets. Compose with `+` between modifiers (e.g. "ctrl+alt").
//
// Note: terminal support varies. Most terminals do not transmit bare ctrl+1
// through ctrl+9 — prefer ModAlt or ModCtrlAlt for portable bindings.
type Modifier string

const (
	// ModNone disables the modifier — mnemonics fire on bare digits. Useful
	// only when no focused widget can legitimately accept digit input.
	ModNone Modifier = ""
	// ModCtrl requires the Control key (limited terminal support for digits).
	ModCtrl Modifier = "ctrl"
	// ModAlt requires the Alt / Option key.
	ModAlt Modifier = "alt"
	// ModCtrlAlt requires Control and Alt together.
	ModCtrlAlt Modifier = "ctrl+alt"
)

// Handler coordinates focus across a fixed set of components.
//
// Exactly zero or one component is focused at any time. The handler starts
// neutral (Focused() returns -1) and stays neutral until the first Tab,
// Shift+Tab, or mnemonic key press, or an explicit FocusIndex call.
type Handler struct {
	entries    []*entry
	byMnemonic map[rune]int
	current    int
	logger     *slog.Logger
	modifier   Modifier
	tabKey     key.Binding
	shiftKey   key.Binding
}

type entry struct {
	raw       any
	focusable Focusable
	mnemonic  rune // 0 means no mnemonic
}

// Option configures a Handler at construction.
type Option func(*Handler)

// WithLogger overrides the default slog logger. Warnings (duplicate
// mnemonics, invalid mnemonic runes, unsupported component types) are emitted
// at slog.LevelWarn.
func WithLogger(l *slog.Logger) Option {
	return func(h *Handler) { h.logger = l }
}

// WithModifier sets the keyboard modifier required for mnemonic digit
// shortcuts. The handler will only fire a mnemonic when the bound digit is
// pressed with this modifier (e.g. WithModifier(ModCtrl) means ctrl+1
// instead of bare 1), which keeps plain digits available as text input for
// focused text widgets.
//
// The default is ModNone (bare digits). Pass an explicit modifier whenever
// any registered component can accept digit input.
func WithModifier(m Modifier) Option {
	return func(h *Handler) { h.modifier = m }
}

// New constructs an empty Handler.
func New(opts ...Option) *Handler {
	h := &Handler{
		byMnemonic: map[rune]int{},
		current:    -1,
		logger:     slog.Default(),
		tabKey:     key.NewBinding(key.WithKeys("tab")),
		shiftKey:   key.NewBinding(key.WithKeys("shift+tab")),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// mnemonicKey returns the full key sequence string the handler matches for a
// given digit rune, taking the configured modifier into account. The format
// matches tea.KeyPressMsg.String() exactly (e.g. "ctrl+1", "1").
func (h *Handler) mnemonicKey(r rune) string {
	if h.modifier == ModNone {
		return string(r)
	}
	return string(h.modifier) + "+" + string(r)
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

// AddMnemonic registers a component and binds it to a mnemonic key. The key
// must be one of '0'..'9'. On success returns a mnemonic.Button whose action
// focuses the component; the host renders this button wherever the [N]
// indicator should appear.
//
// If the key is outside '0'..'9' or is already bound, a warning is logged and
// the component is still registered for Tab navigation but receives no
// mnemonic — the return value is nil.
func (h *Handler) AddMnemonic(c any, mnemonicKey rune) *mnemonic.Button {
	f, ok := adapt(c)
	if !ok {
		h.logger.Warn("focus: unsupported component type, skipping",
			"type", fmt.Sprintf("%T", c))
		return nil
	}
	if !isDigit(mnemonicKey) {
		h.logger.Warn("focus: mnemonic must be '0'..'9', registering without mnemonic",
			"key", string(mnemonicKey))
		h.entries = append(h.entries, &entry{raw: c, focusable: f})
		return nil
	}
	if _, dup := h.byMnemonic[mnemonicKey]; dup {
		h.logger.Warn("focus: mnemonic already bound, registering without mnemonic",
			"key", string(mnemonicKey))
		h.entries = append(h.entries, &entry{raw: c, focusable: f})
		return nil
	}
	idx := len(h.entries)
	h.entries = append(h.entries, &entry{raw: c, focusable: f, mnemonic: mnemonicKey})
	h.byMnemonic[mnemonicKey] = idx
	// Resolve via the mnemonic rune at trigger time rather than the index
	// captured here — Remove may have shifted indices in between.
	r := mnemonicKey
	return mnemonic.New(string(r), r, func() tea.Cmd {
		return h.focusByMnemonic(r)
	}, mnemonic.WithBindingKey(h.mnemonicKey(r)))
}

// Remove unregisters a component, dropping its mnemonic binding if any. The
// match is by reference equality against the value originally passed to Add /
// AddMnemonic. Returns true if removed.
//
// If the removed component was focused, the handler returns to neutral.
func (h *Handler) Remove(c any) bool {
	for i, e := range h.entries {
		if e.raw != c {
			continue
		}
		if e.mnemonic != 0 {
			delete(h.byMnemonic, e.mnemonic)
		}
		h.entries = append(h.entries[:i], h.entries[i+1:]...)
		h.rebuildMnemonicIndex()
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

// Update intercepts navigation keys. Returns (handled, cmd). When handled is
// true the host should not forward the same message to the focused
// component — the handler consumed it. When handled is false the host is
// free to dispatch the message normally (typically to FocusedComponent).
//
// Handled keys are: Tab, Shift+Tab, and any bound mnemonic digit prefixed by
// the configured modifier (see [WithModifier]). With ModNone, bare digits
// fire mnemonics and will shadow text input.
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
	if r, ok := h.parseMnemonicPress(kp); ok {
		if idx, bound := h.byMnemonic[r]; bound {
			return true, h.focusIndex(idx)
		}
	}
	return false, nil
}

// parseMnemonicPress extracts the digit rune from a key press if it matches
// the handler's modifier+digit shape, ignoring anything else.
func (h *Handler) parseMnemonicPress(kp tea.KeyPressMsg) (rune, bool) {
	s := kp.String()
	if h.modifier == ModNone {
		if len(s) == 1 && isDigit(rune(s[0])) {
			return rune(s[0]), true
		}
		return 0, false
	}
	prefix := string(h.modifier) + "+"
	if len(s) != len(prefix)+1 {
		return 0, false
	}
	if s[:len(prefix)] != prefix {
		return 0, false
	}
	last := rune(s[len(prefix)])
	if !isDigit(last) {
		return 0, false
	}
	return last, true
}

// Focused returns the index of the currently focused component, or -1 if
// none.
func (h *Handler) Focused() int { return h.current }

// FocusedComponent returns the raw component originally passed to Add /
// AddMnemonic for the currently focused entry, or nil if none. Hosts use
// this to route subsequent messages to the right widget without tracking
// indices themselves.
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
	h.byMnemonic = nil
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

func (h *Handler) focusByMnemonic(r rune) tea.Cmd {
	idx, ok := h.byMnemonic[r]
	if !ok {
		return nil
	}
	return h.focusIndex(idx)
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

func (h *Handler) rebuildMnemonicIndex() {
	h.byMnemonic = map[rune]int{}
	for i, e := range h.entries {
		if e.mnemonic != 0 {
			h.byMnemonic[e.mnemonic] = i
		}
	}
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

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
