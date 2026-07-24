package diffview

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
)

// New().View() must show the title tab and the body so the diff (or an error
// rendered into the body) is actually visible — not a blank pane.
func TestNew_ViewShowsTitleAndBody(t *testing.T) {
	m := New("diff-x", ".claude/skills/foo/SKILL.md", "+added line", 80, 24)
	if m == nil {
		t.Fatal("New returned nil modal")
	}
	if m.ID() != "diff-x" {
		t.Errorf("ID = %q, want %q", m.ID(), "diff-x")
	}
	out := m.View()
	if !strings.Contains(out, "Diff: .claude/skills/foo/SKILL.md") {
		t.Errorf("view missing title tab:\n%s", out)
	}
	if !strings.Contains(out, "added line") {
		t.Errorf("view missing body text:\n%s", out)
	}
}

// ErrorText renders a diff-load failure into the body so Acceptance Criterion 7
// (a typed error shown inside the modal, not a blank pane) holds: both the
// preamble and the underlying error text must reach View().
func TestErrorText_RendersInsideModalView(t *testing.T) {
	m := New("diff-err", "p", ErrorText(errors.New("boom")), 80, 24)
	out := m.View()
	if !strings.Contains(out, "Failed to produce diff") {
		t.Errorf("view missing error preamble:\n%s", out)
	}
	if !strings.Contains(out, "boom") {
		t.Errorf("view missing underlying error text:\n%s", out)
	}
}

// Esc transitions the content to Cancelled so the hosting modal emits a
// ResolvedMsg (Confirmed=false) — the shell then clears s.modal and returns to
// the tree (Acceptance Criterion 5). `q` is reserved for the global quit
// binding and must not close the dialog.
func TestContent_EscCancelsQKeepsActive(t *testing.T) {
	c := &content{keys: defaultKeymap()}
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("initial state = %v, want Active", state)
	}
	_, _ = c.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Errorf("state after q = %v, want Active (q must not close the diff dialog)", state)
	}
	_, _ = c.Update(tea.KeyPressMsg{Code: 27})
	state, value := c.Lifecycle()
	if state != modal.Cancelled {
		t.Errorf("state after esc = %v, want Cancelled", state)
	}
	if value != nil {
		t.Errorf("value = %v, want nil on cancel", value)
	}
}

// SetSize on a degenerate 1x1 terminal must not panic: ScrollableInner clamps
// both axes at 1 so the viewport stays valid.
func TestSetSize_TinyTerminalDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSize panicked on tiny terminal: %v", r)
		}
	}()
	New("diff-tiny", "p", "body", 80, 24).SetSize(1, 1)
}
