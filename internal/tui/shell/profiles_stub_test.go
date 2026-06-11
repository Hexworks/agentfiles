package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestProfilesStub_EscEmitsPopCmd(t *testing.T) {
	s := newProfilesStub()
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestProfilesStub_TitleAndBody(t *testing.T) {
	s := newProfilesStub()
	if got := s.Title(); got != "Profiles" {
		t.Errorf("Title() = %q, want Profiles", got)
	}
	if got := s.Body(80, 10); !strings.Contains(got, "task 0025") {
		t.Errorf("Body() = %q, want substring %q", got, "task 0025")
	}
}
