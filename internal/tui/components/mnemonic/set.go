package mnemonic

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Set is a collection of [Button] values that enforces mnemonic uniqueness at
// registration. Screens register buttons through a Set instead of constructing
// them ad-hoc; the host then queries the Set for rendering and key routing.
//
// Uniqueness is case-insensitive: a button registered with mnemonic `'S'`
// collides with one registered with `'s'`. Duplicate registration is a
// programmer error and panics — per-screen unit tests catch this in CI, the
// runtime panic catches it at first render during development.
type Set struct {
	buttons []*Button
}

// NewSet returns an empty Set.
func NewSet() *Set { return &Set{} }

// Add registers b. Panics if another button with the same mnemonic rune
// (case-insensitive) is already registered.
func (s *Set) Add(b *Button) {
	r := unicode.ToLower(b.Mnemonic())
	for _, existing := range s.buttons {
		if unicode.ToLower(existing.Mnemonic()) == r {
			panic(fmt.Sprintf("mnemonic: duplicate mnemonic %q registered for %q (already used by %q)",
				b.Mnemonic(), b.Label(), existing.Label()))
		}
	}
	s.buttons = append(s.buttons, b)
}

// View renders every button via [Button.View] joined by sep. Order is the
// order in which buttons were registered.
func (s *Set) View(sep string) string {
	parts := make([]string, len(s.buttons))
	for i, b := range s.buttons {
		parts[i] = b.View()
	}
	return strings.Join(parts, sep)
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

// Buttons returns the registered buttons in insertion order. The returned
// slice is a copy; mutating it does not affect the Set.
func (s *Set) Buttons() []*Button {
	out := make([]*Button, len(s.buttons))
	copy(out, s.buttons)
	return out
}
