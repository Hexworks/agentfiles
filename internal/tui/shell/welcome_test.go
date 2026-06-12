package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/actions"
	"github.com/hexworks/agentfiles/internal/app"
)

func newTestWelcomeScreen(t *testing.T) *welcomeScreen {
	t.Helper()
	dir := t.TempDir()
	svc := app.New(dir + "/registry.json")
	return newWelcomeScreen(defaultGlobalKeyMap(), actions.New(svc))
}

func TestWelcomeScreen_InitialCursorOnProfiles(t *testing.T) {
	s := newTestWelcomeScreen(t)
	if s.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", s.cursor)
	}
	if s.items[s.cursor].label != "Profiles" {
		t.Fatalf("selected = %q, want Profiles", s.items[s.cursor].label)
	}
}

func TestWelcomeScreen_DownMovesCursor(t *testing.T) {
	cases := []struct {
		name string
		kp   tea.KeyPressMsg
	}{
		{"down arrow", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"j key", tea.KeyPressMsg{Code: 'j', Text: "j"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestWelcomeScreen(t)
			_, _ = s.Update(tc.kp)
			if s.cursor != 1 {
				t.Errorf("cursor = %d, want 1", s.cursor)
			}
		})
	}
}

func TestWelcomeScreen_UpWrapsFromTop(t *testing.T) {
	cases := []struct {
		name string
		kp   tea.KeyPressMsg
	}{
		{"up arrow", tea.KeyPressMsg{Code: tea.KeyUp}},
		{"k key", tea.KeyPressMsg{Code: 'k', Text: "k"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestWelcomeScreen(t)
			_, _ = s.Update(tc.kp)
			if s.cursor != len(s.items)-1 {
				t.Errorf("cursor = %d, want %d (wrap)", s.cursor, len(s.items)-1)
			}
		})
	}
}

func TestWelcomeScreen_DownWrapsFromBottom(t *testing.T) {
	cases := []struct {
		name string
		kp   tea.KeyPressMsg
	}{
		{"down arrow", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"j key", tea.KeyPressMsg{Code: 'j', Text: "j"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestWelcomeScreen(t)
			s.cursor = len(s.items) - 1
			_, _ = s.Update(tc.kp)
			if s.cursor != 0 {
				t.Errorf("cursor = %d, want 0 (wrap)", s.cursor)
			}
		})
	}
}

func TestWelcomeScreen_EnterOnProfilesPushesProfilesScreen(t *testing.T) {
	s := newTestWelcomeScreen(t)
	s.cursor = 0
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter on Profiles produced nil cmd")
	}
	push, ok := cmd().(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	if _, ok := push.Screen.(*profilesScreen); !ok {
		t.Fatalf("pushed screen = %T, want *profilesScreen", push.Screen)
	}
}

func TestWelcomeScreen_VOnSettingsPushesSettingsScreen(t *testing.T) {
	s := newTestWelcomeScreen(t)
	s.cursor = 1
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if cmd == nil {
		t.Fatalf("v on Settings produced nil cmd")
	}
	push, ok := cmd().(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	if _, ok := push.Screen.(*settingsScreen); !ok {
		t.Fatalf("pushed screen = %T, want *settingsScreen", push.Screen)
	}
}

func TestWelcomeScreen_EnterOnQuitEmitsQuitMsg(t *testing.T) {
	s := newTestWelcomeScreen(t)
	s.cursor = len(s.items) - 1
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter on Quit produced nil cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd produced %T, want tea.QuitMsg", cmd())
	}
}

func TestWelcomeScreen_UnrelatedKeyIsNoop(t *testing.T) {
	s := newTestWelcomeScreen(t)
	s.cursor = 1
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
	}
	if s.cursor != 1 {
		t.Errorf("unrelated key moved cursor to %d, want 1", s.cursor)
	}
}

func TestWelcomeScreen_TitleIsAgentfiles(t *testing.T) {
	s := newTestWelcomeScreen(t)
	if got := s.Title(); got != "Agentfiles" {
		t.Errorf("Title() = %q, want Agentfiles", got)
	}
}

func TestWelcomeScreen_BodyContainsAllItems(t *testing.T) {
	s := newTestWelcomeScreen(t)
	body := s.Body(80)
	for _, want := range []string{"Choose a task", "Profiles", "Settings", "Quit"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
}

func TestWelcomeScreen_BodyMarksSelectedRow(t *testing.T) {
	s := newTestWelcomeScreen(t)
	s.cursor = 1
	body := s.Body(80)
	if !strings.Contains(body, "> Settings") {
		t.Errorf("Body missing selected-row marker on Settings\n%s", body)
	}
	if strings.Contains(body, "> Profiles") {
		t.Errorf("Body has selection marker on unselected row\n%s", body)
	}
}

func TestWelcomeScreen_BodyClampsLongLabelsToWidth(t *testing.T) {
	s := newTestWelcomeScreen(t)
	// width 6: prefix 4 + label 2 → "Profiles" must truncate to "P…".
	body := s.Body(6)
	if strings.Contains(body, "Profiles") {
		t.Errorf("Body did not truncate Profiles on narrow width\n%s", body)
	}
	if !strings.Contains(body, "…") {
		t.Errorf("Body missing ellipsis on narrow width\n%s", body)
	}
}
