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
package mnemonic

import (
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Action is the side-effect executed when a button fires. Returning a nil
// [tea.Cmd] is valid for actions that only mutate parent state.
type Action func() tea.Cmd

// Styles centralizes the appearance of a button (and the separator a
// [Set] uses to join buttons) so callers can theme everything in one
// place without touching the component internals.
type Styles struct {
	// Bracket renders the `[` and `]` framing the label.
	Bracket lipgloss.Style
	// Label renders the non-mnemonic characters of the label.
	Label lipgloss.Style
	// Mnemonic renders the single highlighted mnemonic character.
	Mnemonic lipgloss.Style
	// Separator joins buttons in [Set.View]. Ignored by [Button.View].
	Separator string
}

// DefaultStyles returns palette-neutral styles. Callers that want themed
// buttons should pass [WithStyles].
func DefaultStyles() Styles {
	return Styles{
		Bracket:   lipgloss.NewStyle(),
		Label:     lipgloss.NewStyle(),
		Mnemonic:  lipgloss.NewStyle().Bold(true).Underline(true),
		Separator: " ",
	}
}

// Button is a mnemonic-bound, label-rendered action trigger.
type Button struct {
	label      string
	mnemonic   rune
	action     Action
	binding    key.Binding
	bindingKey string // set via [WithBindingKey]; empty falls back to the lowercased mnemonic
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
	b.binding = key.NewBinding(
		key.WithKeys(bindKey),
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

// View renders the button as `[Label]` with the mnemonic character styled
// distinctly. The first case-insensitive occurrence of the mnemonic is the one
// highlighted; subsequent occurrences are rendered with the normal label
// style.
func (b *Button) View() string {
	var sb strings.Builder
	sb.WriteString(b.styles.Bracket.Render("["))
	highlighted := false
	for _, r := range b.label {
		if !highlighted && unicode.ToLower(r) == unicode.ToLower(b.mnemonic) {
			sb.WriteString(b.styles.Mnemonic.Render(string(r)))
			highlighted = true
			continue
		}
		sb.WriteString(b.styles.Label.Render(string(r)))
	}
	sb.WriteString(b.styles.Bracket.Render("]"))
	return sb.String()
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
