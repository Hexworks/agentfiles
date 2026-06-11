package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWelcomeStub_TitleAndBody(t *testing.T) {
	s := newWelcomeStub()
	if got := s.Title(); got != "Welcome" {
		t.Errorf("Title() = %q, want %q", got, "Welcome")
	}
	if body := s.Body(80, 10); !strings.Contains(body, "agentfiles") {
		t.Errorf("Body missing 'agentfiles':\n%s", body)
	}
}

func TestStubs_EscapeEmitsPopCmd(t *testing.T) {
	stubs := []Screen{newNotificationsStub(), newSettingsStub(), newInfoStub()}
	for _, s := range stubs {
		t.Run(s.Title(), func(t *testing.T) {
			_, cmd := s.Update(tea.KeyPressMsg{Code: 27, Text: ""}) // esc
			if cmd == nil {
				t.Fatalf("esc produced nil cmd")
			}
			if _, ok := cmd().(PopScreenMsg); !ok {
				t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
			}
		})
	}
}
