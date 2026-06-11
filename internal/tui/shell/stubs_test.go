package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// The remaining placeholder stub (settings) still pops on esc — the
// real screen lands in task 0024.
func TestSettingsStub_EscEmitsPopCmd(t *testing.T) {
	s := newSettingsStub()
	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("esc produced nil cmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestSettingsStub_TitleAndBody(t *testing.T) {
	s := newSettingsStub()
	if got := s.Title(); got != "Settings" {
		t.Errorf("Title() = %q, want Settings", got)
	}
	if got := s.Body(80, 10); !strings.Contains(got, "task 0024") {
		t.Errorf("Body() = %q, want substring %q", got, "task 0024")
	}
}

// The notifications screen owns the Notifications modal. Title is the
// load-bearing observable the status bar and global-key test rely on.
func TestNotificationsScreen_Title(t *testing.T) {
	m := newTestShell(t)
	s := m.newNotificationsScreen()
	if got := s.Title(); got != "Notifications" {
		t.Errorf("Title() = %q, want Notifications", got)
	}
}

// A modal.ResolvedMsg arriving from the wrapped modal must close the
// screen (pop). Without this the user would be stuck on a modal that
// has already reported itself done.
func TestNotificationsScreen_ResolvedMsgPops(t *testing.T) {
	m := newTestShell(t)
	s := m.newNotificationsScreen()
	_, cmd := s.Update(modal.ResolvedMsg{ID: "notifications", Confirmed: false})
	if cmd == nil {
		t.Fatalf("ResolvedMsg produced nil cmd, want popCmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}

func TestInfoScreen_Title(t *testing.T) {
	m := newTestShell(t)
	s := m.newInfoScreen()
	if got := s.Title(); got != "Info" {
		t.Errorf("Title() = %q, want Info", got)
	}
}

func TestInfoScreen_ResolvedMsgPops(t *testing.T) {
	m := newTestShell(t)
	s := m.newInfoScreen()
	_, cmd := s.Update(modal.ResolvedMsg{ID: "info", Confirmed: false})
	if cmd == nil {
		t.Fatalf("ResolvedMsg produced nil cmd, want popCmd")
	}
	if _, ok := cmd().(PopScreenMsg); !ok {
		t.Fatalf("cmd produced %T, want PopScreenMsg", cmd())
	}
}
