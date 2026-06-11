package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWelcomeScreen_InitialCursorOnProfiles(t *testing.T) {
	s := newWelcomeScreen()
	if s.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", s.cursor)
	}
	if s.items[s.cursor].label != "Profiles" {
		t.Fatalf("selected = %q, want Profiles", s.items[s.cursor].label)
	}
}

func TestWelcomeScreen_DownMovesCursor(t *testing.T) {
	s := newWelcomeScreen()
	for _, kp := range []tea.KeyPressMsg{
		{Code: tea.KeyDown},
		{Code: 'j', Text: "j"},
	} {
		s.cursor = 0
		_, _ = s.Update(kp)
		if s.cursor != 1 {
			t.Errorf("after %v cursor = %d, want 1", kp, s.cursor)
		}
	}
}

func TestWelcomeScreen_UpWrapsFromTop(t *testing.T) {
	s := newWelcomeScreen()
	for _, kp := range []tea.KeyPressMsg{
		{Code: tea.KeyUp},
		{Code: 'k', Text: "k"},
	} {
		s.cursor = 0
		_, _ = s.Update(kp)
		if s.cursor != len(s.items)-1 {
			t.Errorf("after %v cursor = %d, want %d (wrap)", kp, s.cursor, len(s.items)-1)
		}
	}
}

func TestWelcomeScreen_DownWrapsFromBottom(t *testing.T) {
	s := newWelcomeScreen()
	s.cursor = len(s.items) - 1
	_, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (wrap)", s.cursor)
	}
}

func TestWelcomeScreen_EnterOnProfilesPushesProfilesStub(t *testing.T) {
	s := newWelcomeScreen()
	s.cursor = 0
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("enter on Profiles produced nil cmd")
	}
	push, ok := cmd().(PushScreenMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PushScreenMsg", cmd())
	}
	if _, ok := push.Screen.(*profilesStub); !ok {
		t.Fatalf("pushed screen = %T, want *profilesStub", push.Screen)
	}
}

func TestWelcomeScreen_VOnSettingsPushesSettingsScreen(t *testing.T) {
	s := newWelcomeScreen()
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
	s := newWelcomeScreen()
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
	s := newWelcomeScreen()
	s.cursor = 1
	_, cmd := s.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if cmd != nil {
		t.Errorf("unrelated key produced cmd = %v, want nil", cmd)
	}
	if s.cursor != 1 {
		t.Errorf("unrelated key moved cursor to %d, want 1", s.cursor)
	}
}

func TestWelcomeScreen_TitleAndBody(t *testing.T) {
	s := newWelcomeScreen()
	if got := s.Title(); got != "Agentfiles" {
		t.Errorf("Title() = %q, want Agentfiles", got)
	}
	s.cursor = 1
	body := s.Body(80, 10)
	for _, want := range []string{"Choose a task", "Profiles", "Settings", "Quit"} {
		if !strings.Contains(body, want) {
			t.Errorf("Body missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, "> Settings") {
		t.Errorf("Body missing selected-row marker on Settings\n%s", body)
	}
	if strings.Contains(body, "> Profiles") {
		t.Errorf("Body has selection marker on unselected row\n%s", body)
	}
}
