package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsScreen_BTriggersPop(t *testing.T) {
	s := newSettingsScreen()
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil {
		t.Fatalf("b produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestSettingsScreen_EscTriggersPop(t *testing.T) {
	s := newSettingsScreen()
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestSettingsScreen_UnrelatedKeyIsNoop(t *testing.T) {
	s := newSettingsScreen()
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
	}
}

func TestSettingsScreen_StatusKeysExposeBack(t *testing.T) {
	s := newSettingsScreen()
	keys := s.StatusKeys()
	if len(keys) != 1 {
		t.Fatalf("StatusKeys length = %d, want 1", len(keys))
	}
	h := keys[0].Help()
	if h.Key != "b" {
		t.Errorf("Help().Key = %q, want b", h.Key)
	}
	if h.Desc != "Back" {
		t.Errorf("Help().Desc = %q, want Back", h.Desc)
	}
}

func TestSettingsScreen_TitleAndBody(t *testing.T) {
	s := newSettingsScreen()
	if got := s.Title(); got != "Settings" {
		t.Errorf("Title() = %q, want Settings", got)
	}
	body := s.Body(80, 10)
	if !strings.Contains(body, "Coming soon") {
		t.Errorf("Body missing 'Coming soon'\n%s", body)
	}
	if !strings.Contains(body, "[") || !strings.Contains(body, "ack]") {
		t.Errorf("Body missing [Back] button render\n%s", body)
	}
}
