package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSelectProjectAssetsStub_BTriggersPop(t *testing.T) {
	s := newSelectProjectAssetsStub("alpha", "proj-1")
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if cmd == nil {
		t.Fatalf("b produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestSelectProjectAssetsStub_EscTriggersPop(t *testing.T) {
	s := newSelectProjectAssetsStub("alpha", "proj-1")
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestSelectProjectAssetsStub_UnrelatedKeyIsNoop(t *testing.T) {
	s := newSelectProjectAssetsStub("alpha", "proj-1")
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
	}
}

func TestSelectProjectAssetsStub_TitleAndStatusKeysExposeBack(t *testing.T) {
	s := newSelectProjectAssetsStub("alpha", "proj-1")

	if got := s.Title(); got != "Select Project Assets" {
		t.Errorf("Title() = %q, want Select Project Assets", got)
	}
	keys := s.StatusKeys()
	if len(keys) != 1 {
		t.Fatalf("StatusKeys length = %d, want 1", len(keys))
	}
	h := keys[0].Help()
	if h.Key != "b" || h.Desc != "Back" {
		t.Errorf("Help() = (%q, %q), want (b, Back)", h.Key, h.Desc)
	}
}

func TestSelectProjectAssetsStub_BodyIncludesProfileAndProjectIDs(t *testing.T) {
	s := newSelectProjectAssetsStub("alpha-123", "proj-xyz")
	body := s.Body(80, 10)
	for _, want := range []string{"alpha-123", "proj-xyz"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

var _ Screen = (*selectProjectAssetsStub)(nil)
