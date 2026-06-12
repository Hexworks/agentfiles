package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestInfoScreen_Title(t *testing.T) {
	m := newTestShell(t)
	s := m.newInfoScreen()
	if got := s.Title(); got != "Help" {
		t.Errorf("Title() = %q, want Help", got)
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

// Happy-path: with a real overview.md under docs/manual/, the info modal
// resolves the topic, loads the file, and renders the body. Guards
// against the path-resolution regression (review finding #1) where the
// shell handed help.New a full repo path and got a NotFoundError back.
func TestInfoScreen_RendersOverviewBody(t *testing.T) {
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
	m.width = 80
	m.height = 24
	s := m.newInfoScreen()

	rendered := s.Body(80)
	if !strings.Contains(rendered, "Global keys") {
		t.Errorf("rendered body missing expected manual content; got:\n%s", rendered)
	}
	if strings.Contains(rendered, "not found") || strings.Contains(rendered, "Failed to load manual") {
		t.Errorf("rendered body surfaced a load error; got:\n%s", rendered)
	}
}
