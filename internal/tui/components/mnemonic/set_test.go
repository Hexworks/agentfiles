package mnemonic

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func noopAction() Action { return func() tea.Cmd { return nil } }

func TestSetAddPanicsOnNilButton(t *testing.T) {
	t.Parallel()

	s := NewSet()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil button, got none")
		}
	}()
	s.Add(nil)
}

func TestSetAddPanicsOnDuplicateMnemonicSameCase(t *testing.T) {
	t.Parallel()

	s := NewSet()
	s.Add(New("Save", 'S', noopAction()))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate mnemonic, got none")
		}
	}()
	s.Add(New("Send", 'S', noopAction()))
}

func TestSetAddPanicsOnDuplicateMnemonicCaseInsensitive(t *testing.T) {
	t.Parallel()

	s := NewSet()
	s.Add(New("Save", 'S', noopAction()))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on case-insensitive duplicate mnemonic, got none")
		}
	}()
	s.Add(New("send", 's', noopAction()))
}

func TestSetMatchReturnsFirstMatchingButton(t *testing.T) {
	t.Parallel()

	save := New("Save", 'S', noopAction())
	cancel := New("Cancel", 'C', noopAction())

	s := NewSet()
	s.Add(save)
	s.Add(cancel)

	got := s.Match(tea.KeyPressMsg{Code: 's', Text: "s"})
	if got != save {
		t.Fatalf("Match('s') = %v, want save", got)
	}

	got = s.Match(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if got != cancel {
		t.Fatalf("Match('c') = %v, want cancel", got)
	}
}

func TestSetMatchReturnsNilForUnknownKey(t *testing.T) {
	t.Parallel()

	s := NewSet()
	s.Add(New("Save", 'S', noopAction()))

	if got := s.Match(tea.KeyPressMsg{Code: 'z', Text: "z"}); got != nil {
		t.Fatalf("Match('z') = %v, want nil", got)
	}
}

func TestSetMatchOnEmptySetReturnsNil(t *testing.T) {
	t.Parallel()

	s := NewSet()
	if got := s.Match(tea.KeyPressMsg{Code: 's', Text: "s"}); got != nil {
		t.Fatalf("Match on empty Set = %v, want nil", got)
	}
}

func TestSetViewPreservesInsertionOrder(t *testing.T) {
	t.Parallel()

	a := New("Alpha", 'A', noopAction())
	b := New("Bravo", 'B', noopAction())
	c := New("Charlie", 'C', noopAction())

	s := NewSet()
	s.Add(a)
	s.Add(b)
	s.Add(c)

	got := s.View()
	// View renders each button with ANSI styling around the mnemonic rune,
	// so search for each button's own rendered string (which embeds those
	// codes identically) rather than the raw label text.
	prev := -1
	for _, btn := range []*Button{a, b, c} {
		idx := strings.Index(got, btn.View())
		if idx <= prev {
			t.Fatalf("button %q out of order at idx %d (prev %d) in %q", btn.Label(), idx, prev, got)
		}
		prev = idx
	}
}

func TestSetViewUsesConfiguredSeparator(t *testing.T) {
	t.Parallel()

	styles := DefaultStyles()
	styles.Separator = " | "

	s := NewSet(WithSetStyles(styles))
	s.Add(New("Alpha", 'A', noopAction()))
	s.Add(New("Bravo", 'B', noopAction()))
	s.Add(New("Charlie", 'C', noopAction()))

	got := s.View()
	if strings.Count(got, " | ") != 2 {
		t.Fatalf("separator count = %d, want 2 in %q", strings.Count(got, " | "), got)
	}
}

func TestSetViewOnEmptySetIsEmpty(t *testing.T) {
	t.Parallel()

	if got := NewSet().View(); got != "" {
		t.Fatalf("empty Set View = %q, want \"\"", got)
	}
}

func TestSetButtonsReturnsCopy(t *testing.T) {
	t.Parallel()

	a := New("Alpha", 'A', noopAction())
	b := New("Bravo", 'B', noopAction())

	s := NewSet()
	s.Add(a)
	s.Add(b)

	got := s.Buttons()
	if len(got) != 2 {
		t.Fatalf("len(Buttons) = %d, want 2", len(got))
	}
	got[0] = nil

	again := s.Buttons()
	if again[0] != a {
		t.Fatal("Buttons returned slice aliases internal storage")
	}
}

func TestSetButtonsOnEmptySetReturnsEmpty(t *testing.T) {
	t.Parallel()

	got := NewSet().Buttons()
	if len(got) != 0 {
		t.Fatalf("len(Buttons) on empty Set = %d, want 0", len(got))
	}
}

func TestSetAddDuplicatePanicMessageNamesConflictingLabels(t *testing.T) {
	t.Parallel()

	s := NewSet()
	s.Add(New("Save", 'S', noopAction()))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		// The message should name both labels so a maintainer can spot
		// the collision without reading the registration order from code.
		if !strings.Contains(msg, "Save") || !strings.Contains(msg, "Send") {
			t.Fatalf("panic %q does not name both conflicting labels", msg)
		}
	}()
	s.Add(New("Send", 'S', noopAction()))
}
