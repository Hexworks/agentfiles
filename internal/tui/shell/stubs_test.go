package shell

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPlaceholderStubs_EscEmitsPopCmd(t *testing.T) {
	stubs := []Screen{
		newNotificationsStub(),
		newSettingsStub(),
		newInfoStub(),
	}
	for _, s := range stubs {
		t.Run(s.Title(), func(t *testing.T) {
			_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if cmd == nil {
				t.Fatalf("esc produced nil cmd")
			}
			if _, ok := cmd().(PopScreenMsg); !ok {
				t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
			}
		})
	}
}

func TestPlaceholderStubs_TitlesAndBodies(t *testing.T) {
	cases := []struct {
		s         Screen
		wantTitle string
		bodyHas   string
	}{
		{newNotificationsStub(), "Notifications", "task 0023"},
		{newSettingsStub(), "Settings", "task 0024"},
		{newInfoStub(), "Info", "task 0023"},
	}
	for _, tc := range cases {
		t.Run(tc.wantTitle, func(t *testing.T) {
			if got := tc.s.Title(); got != tc.wantTitle {
				t.Errorf("Title() = %q, want %q", got, tc.wantTitle)
			}
			if got := tc.s.Body(80, 10); !contains(got, tc.bodyHas) {
				t.Errorf("Body() = %q, want substring %q", got, tc.bodyHas)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
