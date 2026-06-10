package mnemonic

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func noopAction() Action { return func() tea.Cmd { return nil } }

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

func TestSetViewPreservesInsertionOrder(t *testing.T) {
	t.Parallel()

	a := New("Alpha", 'A', noopAction())
	b := New("Bravo", 'B', noopAction())
	c := New("Charlie", 'C', noopAction())

	s := NewSet()
	s.Add(a)
	s.Add(b)
	s.Add(c)

	got := s.View(" ")
	want := strings.Join([]string{a.View(), b.View(), c.View()}, " ")
	if got != want {
		t.Fatalf("View order mismatch\n got: %q\nwant: %q", got, want)
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
