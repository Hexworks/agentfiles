package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEditProfileStub_BTriggersPop(t *testing.T) {
	s := newEditProfileStub("alpha")
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil {
		t.Fatalf("b produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditProfileStub_EscTriggersPop(t *testing.T) {
	s := newEditProfileStub("alpha")
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestEditProfileStub_UnrelatedKeyIsNoop(t *testing.T) {
	s := newEditProfileStub("alpha")
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
	}
}

func TestEditProfileStub_TitleAndStatusKeysExposeBack(t *testing.T) {
	s := newEditProfileStub("alpha")

	if got := s.Title(); got != "Edit Profile" {
		t.Errorf("Title() = %q, want Edit Profile", got)
	}
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

func TestEditProfileStub_BodyIncludesProfileID(t *testing.T) {
	s := newEditProfileStub("alpha-123")
	body := s.Body(80, 10)
	if !strings.Contains(body, "alpha-123") {
		t.Errorf("Body missing profile id 'alpha-123'\n%s", body)
	}
}
