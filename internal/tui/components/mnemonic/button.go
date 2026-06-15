// Package mnemonic provides a Button component that renders a labelled
// `[Label]` widget bound to a single-character shortcut. The mnemonic character
// is highlighted inside the label so the user can see which key activates the
// button at a glance.
//
// Buttons are passive widgets: they own a [key.Binding] and an [Action], but
// they do not consume Bubble Tea messages on their own. The hosting screen is
// responsible for routing key presses through the button's [Button.Matches]
// helper and dispatching the returned command. This keeps button uniqueness
// (one mnemonic per screen) the screen's concern, not the component's.
//
// Rendering is SGR-aware: View emits a single ANSI sequence per chunk
// (brackets / mnemonic / rest of label) where each transition fully
// re-states bold + underline + foreground. The button does NOT emit
// intermediate `\x1b[0m` resets that would terminate a parent style
// (e.g. a table's selected-row highlight) mid-content. A single reset
// is emitted at the very end so the button cannot leak its own colors
// into adjacent text.
package mnemonic

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// Action is the side-effect executed when a button fires. Returning a nil
// [tea.Cmd] is valid for actions that only mutate parent state.
type Action func() tea.Cmd

// Styles centralizes the appearance of a button (and the separator a
// [Set] uses to join buttons) so callers can theme everything in one
// place without touching the component internals.
//
// Each chunk style is an [ansi.Style] rather than a lipgloss.Style.
// Storing the raw SGR attribute list means [Button.View] can emit
// state-restoring transitions (one SGR sequence per chunk) without
// inserting bare `\x1b[0m` resets that would break a parent style
// wrapping the button (e.g. a table's selected-row highlight). Each
// style should set foreground + bold + underline explicitly so a
// transition fully overrides whatever the parent left enabled.
type Styles struct {
	// Accent renders the `[` and `]` framing the label.
	Accent ansi.Style
	// Mnemonic renders the single highlighted mnemonic character.
	Mnemonic ansi.Style
	// Text renders the non-mnemonic characters of the label.
	Text ansi.Style
	// Separator joins buttons in [Set.View]. Ignored by [Button.View].
	Separator string
}

// DefaultStyles returns the standard themed palette: accent on the
// brackets, mnemonic color (bold + underlined) on the shortcut letter,
// muted text color on the rest of the label. Pulls colors from
// [styles.ColorAccent], [styles.ColorMnemonic], and [styles.ColorText]
// so a theme change touches one file.
func DefaultStyles() Styles {
	return ThemedStyles(styles.ColorAccent, styles.ColorMnemonic, styles.ColorText)
}

// ThemedStyles builds [Styles] with the three foreground colors fully
// pinned: bold + underline are explicitly disabled on the accent and
// text chunks (so a parent's bold does not leak in) and explicitly
// enabled on the mnemonic chunk. Pass any [ansi.Color] (basic, indexed,
// or true-color) — lipgloss.Color values satisfy the interface and may
// be passed directly.
func ThemedStyles(accent, mnemonic, text ansi.Color) Styles {
	return Styles{
		Accent:    ansi.Style{}.Normal().Underline(false).ForegroundColor(accent),
		Mnemonic:  ansi.Style{}.Bold().Underline(true).ForegroundColor(mnemonic),
		Text:      ansi.Style{}.Normal().Underline(false).ForegroundColor(text),
		Separator: " ",
	}
}

// Button is a mnemonic-bound, label-rendered action trigger.
type Button struct {
	label      string
	mnemonic   rune
	action     Action
	binding    key.Binding
	bindingKey string   // set via [WithBindingKey]; empty falls back to the lowercased mnemonic
	extraKeys  []string // set via [WithExtraBindingKeys]; appended to the binding's key list
	styles     Styles
}

// Option configures a [Button] at construction time.
type Option func(*Button)

// WithStyles overrides the default rendering palette.
func WithStyles(s Styles) Option {
	return func(b *Button) { b.styles = s }
}

// WithBindingKey overrides the key string the button's [Button.Binding]
// listens for. By default the binding matches the lowercased mnemonic rune;
// pass this option when the activating key sequence differs from the
// displayed mnemonic — for example a modifier-prefixed shortcut like
// `ctrl+1` whose visible indicator is still `[1]`.
func WithBindingKey(key string) Option {
	return func(b *Button) { b.bindingKey = key }
}

// WithExtraBindingKeys appends extra trigger keys to the button's binding
// without changing the displayed mnemonic or the binding's help text. Use it
// when a button should fire on its mnemonic AND on a conventional fallback
// like `esc` for a Back button.
func WithExtraBindingKeys(keys ...string) Option {
	return func(b *Button) { b.extraKeys = append(b.extraKeys, keys...) }
}

// New constructs a Button. The mnemonic must be a single rune that appears in
// the label (case-insensitive match). action must be non-nil. Any violation
// panics at construction — these are programmer errors, not user errors.
func New(label string, mnemonic rune, action Action, opts ...Option) *Button {
	if action == nil {
		panic("mnemonic: nil action")
	}
	if label == "" {
		panic("mnemonic: empty label")
	}
	if !containsRuneFold(label, mnemonic) {
		panic("mnemonic: mnemonic rune not present in label")
	}
	keyLabel := string(unicode.ToLower(mnemonic))
	b := &Button{
		label:    label,
		mnemonic: mnemonic,
		action:   action,
		styles:   DefaultStyles(),
	}
	for _, opt := range opts {
		opt(b)
	}
	bindKey := b.bindingKey
	if bindKey == "" {
		bindKey = keyLabel
	}
	keys := append([]string{bindKey}, b.extraKeys...)
	b.binding = key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(bindKey, label),
	)
	return b
}

// Label returns the button label (without brackets).
func (b *Button) Label() string { return b.label }

// Mnemonic returns the shortcut rune (the original case as constructed).
func (b *Button) Mnemonic() rune { return b.mnemonic }

// Binding exposes the [key.Binding] so callers can include the button in a
// help bar or disable it via SetEnabled.
func (b *Button) Binding() key.Binding { return b.binding }

// Matches reports whether the given key press triggers this button.
func (b *Button) Matches(msg tea.KeyPressMsg) bool {
	return key.Matches(msg, b.binding)
}

// Trigger executes the action and returns its command. Callers should invoke
// this when [Button.Matches] returns true.
func (b *Button) Trigger() tea.Cmd {
	return b.action()
}

// chunk classifies a span of the rendered button so the render loop
// only emits an SGR transition when the target style actually changes.
type chunk int

const (
	chunkAccent chunk = iota
	chunkMnemonic
	chunkText
)

// View renders the button as `[Label]` with the mnemonic character styled
// distinctly. The first case-insensitive occurrence of the mnemonic is the
// one highlighted; subsequent occurrences fall under the text style.
//
// Internally, View emits exactly one SGR sequence per chunk transition
// (accent → mnemonic → text → accent) plus a single trailing reset.
// Bare `\x1b[0m` mid-string would terminate a wrapping parent style
// (e.g. a treetable selected-row highlight), so each transition fully
// re-states bold, underline, and foreground instead.
func (b *Button) View() string {
	var sb strings.Builder
	current := chunkAccent
	writeStyle(&sb, b.styles.Accent)
	sb.WriteString("[")
	highlighted := false
	for _, r := range b.label {
		next := chunkText
		if !highlighted && unicode.ToLower(r) == unicode.ToLower(b.mnemonic) {
			next = chunkMnemonic
			highlighted = true
		}
		if next != current {
			switch next {
			case chunkMnemonic:
				writeStyle(&sb, b.styles.Mnemonic)
			case chunkText:
				writeStyle(&sb, b.styles.Text)
			}
			current = next
		}
		sb.WriteRune(r)
	}
	if current != chunkAccent {
		writeStyle(&sb, b.styles.Accent)
	}
	sb.WriteString("]")
	sb.WriteString(ansi.ResetStyle)
	return sb.String()
}

// writeStyle emits the SGR sequence for st only when st carries
// attributes. An empty [ansi.Style] would otherwise stringify to a
// bare reset (`\x1b[m`) per [ansi.Style.String], which is exactly the
// SGR leak the SGR-aware render path is meant to avoid.
func writeStyle(sb *strings.Builder, st ansi.Style) {
	if len(st) == 0 {
		return
	}
	sb.WriteString(st.String())
}

func containsRuneFold(s string, r rune) bool {
	target := unicode.ToLower(r)
	for _, c := range s {
		if unicode.ToLower(c) == target {
			return true
		}
	}
	return false
}
