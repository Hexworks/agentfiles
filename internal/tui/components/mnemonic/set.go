package mnemonic

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Set is a collection of [Button] values that enforces mnemonic uniqueness at
// registration time. Screens register buttons through a Set instead of
// constructing them ad-hoc; the host then queries the Set for rendering and
// key routing.
//
// Uniqueness is case-insensitive: a button registered with mnemonic `'S'`
// collides with one registered with `'s'`. Duplicate registration is a
// programmer error and panics at the [Set.Add] call site — per-screen unit
// tests catch this in CI, the registration-time panic catches it during
// development. The check is O(1) via an internal rune index keyed by the
// lowercased mnemonic.
type Set struct {
	buttons []*Button
	byRune  map[rune]*Button
	styles  Styles
}

// SetOption configures a [Set] at construction time.
type SetOption func(*Set)

// WithSetStyles overrides the [Styles] used by the Set. Only the Separator
// field is consulted; bracket/label/mnemonic styles are owned by the
// individual [Button] values registered with the Set.
func WithSetStyles(s Styles) SetOption { return func(set *Set) { set.styles = s } }

// NewSet returns an empty Set initialized with [DefaultStyles].
func NewSet(opts ...SetOption) *Set {
	s := &Set{
		byRune: make(map[rune]*Button),
		styles: DefaultStyles(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Add registers b. Panics if b is nil or if another button with the same
// mnemonic rune (case-insensitive) is already registered.
func (s *Set) Add(b *Button) {
	if b == nil {
		panic("mnemonic: nil button")
	}
	r := unicode.ToLower(b.Mnemonic())
	if existing, ok := s.byRune[r]; ok {
		panic(fmt.Sprintf("mnemonic: duplicate mnemonic %q for %q collides with %q (mnemonic %q)",
			b.Mnemonic(), b.Label(), existing.Label(), existing.Mnemonic()))
	}
	s.byRune[r] = b
	s.buttons = append(s.buttons, b)
}

// View renders every button via [Button.View] joined by the configured
// separator (defaults to a single space, see [Styles.Separator]). Order is
// the order in which buttons were registered.
func (s *Set) View() string {
	parts := make([]string, len(s.buttons))
	for i, b := range s.buttons {
		parts[i] = b.View()
	}
	return strings.Join(parts, s.styles.Separator)
}

// Match returns the first button whose binding matches kp, or nil when no
// registered button matches.
func (s *Set) Match(kp tea.KeyPressMsg) *Button {
	for _, b := range s.buttons {
		if b.Matches(kp) {
			return b
		}
	}
	return nil
}

// Buttons returns the registered buttons in insertion order.
//
// The insertion order is a part of the contract: callers (and tests)
// rely on it to predict the order [Set.View] renders and the order
// status-bar helpers iterate. Add the buttons in the order you want
// rendered.
//
// The returned slice header is a copy; reordering or replacing entries does
// not affect the Set. The pointed-at Buttons are shared — do not mutate them
// through the returned slice if you want the Set's view of them to stay
// consistent.
func (s *Set) Buttons() []*Button {
	return slices.Clone(s.buttons)
}
