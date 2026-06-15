package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/help"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

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

// Pressing `?` while a screen is focused must emit a ShowHelpMsg
// carrying that screen's topic, instead of pushing a screen onto the
// stack. The shell owns the manual overlay so the global key handler
// can keep `q` mapped to quit while it is visible.
func TestGlobalKey_QuestionMarkEmitsShowHelpMsg(t *testing.T) {
	m := newTestShell(t)
	tm, cmd := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("? produced nil cmd, want ShowHelpMsg")
	}
	msg, ok := cmd().(ShowHelpMsg)
	if !ok {
		t.Fatalf("? produced %T, want ShowHelpMsg", cmd())
	}
	if msg.Topic != help.OverviewTopic {
		t.Errorf("topic = %+v, want %+v (welcome screen has no Topical)", msg.Topic, help.OverviewTopic)
	}
	if m.helpModal.Active() {
		t.Errorf("helpModal opened before ShowHelpMsg was dispatched")
	}
}

// ShowHelpMsg mounts the modal on the shell. A second `?` while the
// modal is open is a no-op so users do not stack identical overlays.
func TestShowHelpMsg_MountsModal(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)

	tm, _ = m.Update(ShowHelpMsg{Topic: help.OverviewTopic})
	m = tm.(Model)
	if !m.helpModal.Active() {
		t.Fatalf("helpModal not active after ShowHelpMsg")
	}
	first := m.helpModal

	tm, _ = m.Update(ShowHelpMsg{Topic: help.OverviewTopic})
	m = tm.(Model)
	if m.helpModal != first {
		t.Errorf("second ShowHelpMsg replaced live modal, want no-op")
	}
}

// While the help overlay is open, `q` must still quit the application:
// it is the universal exit binding, not a modal-close shortcut.
func TestHelpOverlay_QQuitsApplication(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)
	tm, _ = m.Update(ShowHelpMsg{Topic: help.OverviewTopic})
	m = tm.(Model)
	if !m.helpModal.Active() {
		t.Fatalf("precondition: help modal not active")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q produced nil cmd while help open, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q produced %T, want tea.QuitMsg", cmd())
	}
}

// Esc forwarded to the help modal cancels its lifecycle; the resulting
// ResolvedMsg fed back through Update must clear the shell's modal slot
// so the next `?` opens a fresh overlay.
func TestHelpOverlay_EscClosesModal(t *testing.T) {
	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)
	tm, _ = m.Update(ShowHelpMsg{Topic: help.OverviewTopic})
	m = tm.(Model)

	tm, cmd := m.Update(tea.KeyPressMsg{Code: 27})
	m = tm.(Model)
	if cmd == nil {
		t.Fatalf("Esc produced nil cmd, want ResolvedMsg")
	}
	resolved, ok := cmd().(modal.ResolvedMsg)
	if !ok {
		t.Fatalf("Esc cmd produced %T, want modal.ResolvedMsg", cmd())
	}
	tm, _ = m.Update(resolved)
	m = tm.(Model)
	if m.helpModal.Active() {
		t.Errorf("helpModal still active after ResolvedMsg, want cleared")
	}
}

// Happy-path: with a real overview.md under docs/manual/, opening the
// overlay loads the file and renders the body inside the shell view.
// Guards against the path-resolution regression where the shell handed
// help.New a full repo path and got a NotFoundError back.
func TestHelpOverlay_RendersOverviewBody(t *testing.T) {
	dir := t.TempDir()
	manualDir := filepath.Join(dir, "docs", "manual")
	if err := os.MkdirAll(manualDir, 0o755); err != nil {
		t.Fatalf("setup manual dir: %v", err)
	}
	body := "# Overview\n\nGlobal keys section.\n"
	if err := os.WriteFile(filepath.Join(manualDir, "overview.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write overview.md: %v", err)
	}
	t.Chdir(dir)

	m := newTestShell(t)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)
	tm, _ = m.Update(ShowHelpMsg{Topic: help.OverviewTopic})
	m = tm.(Model)

	rendered := m.View().Content
	if !strings.Contains(rendered, "Global keys") {
		t.Errorf("rendered view missing expected manual content; got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Failed to load manual") {
		t.Errorf("rendered view surfaced a load error; got:\n%s", rendered)
	}
}
