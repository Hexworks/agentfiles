package notificationsmodal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// fakeLog is a fixed-content LogReader for deterministic tests. Real
// notifications.Log is also safe to use, but a stub avoids importing
// the ring-buffer's full surface and makes the input list explicit.
type fakeLog struct {
	entries []notifications.Notification
}

func (f *fakeLog) Entries() []notifications.Notification { return f.entries }

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	got, err := time.Parse("15:04:05", value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return got
}

// Three mixed-severity entries supplied newest-first (matching what the
// real Log returns from Entries). The modal must preserve that order in
// the rendered table: the oldest row is the last visible row.
func TestView_RendersEntriesNewestFirst(t *testing.T) {
	log := &fakeLog{entries: []notifications.Notification{
		{Severity: errs.SeverityError, Text: "boom", CreatedAt: mustTime(t, "13:05:15")},
		{Severity: errs.SeverityWarning, Text: "careful", CreatedAt: mustTime(t, "13:04:50")},
		{Severity: errs.SeverityInfo, Text: "hello", CreatedAt: mustTime(t, "13:04:42")},
	}}

	c := newContent(log, 80, 20)
	view := c.View()

	for _, want := range []string{"boom", "careful", "hello", "13:05:15", "13:04:50", "13:04:42"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}

	// Newest-first across all three rows: boom < careful < hello.
	var last int
	for i, want := range []string{"boom", "careful", "hello"} {
		idx := strings.Index(view, want)
		if idx == -1 {
			t.Fatalf("view missing %q", want)
		}
		if i > 0 && idx <= last {
			t.Errorf("entry %q at %d is not strictly after previous at %d\n%s",
				want, idx, last, view)
		}
		last = idx
	}
}

// An empty log must surface the empty-state message rather than render
// a header-only table or crash on a zero-row bubbles/table.
func TestView_EmptyLogShowsPlaceholder(t *testing.T) {
	const wantEmpty = "No notifications yet"
	c := newContent(&fakeLog{}, 80, 20)
	if !c.empty {
		t.Fatalf("content.empty = false on zero-entry log")
	}
	view := c.View()
	if !strings.Contains(view, wantEmpty) {
		t.Errorf("view = %q, want substring %q", view, wantEmpty)
	}
}

// esc and q both transition the content to Cancelled. The host modal
// then emits a ResolvedMsg with Confirmed=false.
func TestUpdate_CloseKeysCancelLifecycle(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"esc", tea.KeyPressMsg{Code: tea.KeyEsc}},
		{"q", tea.KeyPressMsg{Code: 'q', Text: "q"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newContent(&fakeLog{}, 80, 20)
			if state, _ := c.Lifecycle(); state != modal.Active {
				t.Fatalf("initial state = %v, want Active", state)
			}
			_, _ = c.Update(tc.msg)
			state, value := c.Lifecycle()
			if state != modal.Cancelled {
				t.Errorf("state = %v, want Cancelled", state)
			}
			if value != nil {
				t.Errorf("value = %v, want nil on cancel", value)
			}
		})
	}
}

// A non-close key must reach the underlying table without flipping the
// lifecycle. Asserts both: lifecycle stays Active *and* the table
// cursor moves, which is only possible if the message was forwarded.
func TestUpdate_NonCloseKeyKeepsActive(t *testing.T) {
	log := &fakeLog{entries: []notifications.Notification{
		{Severity: errs.SeverityInfo, Text: "row1", CreatedAt: mustTime(t, "10:00:00")},
		{Severity: errs.SeverityInfo, Text: "row2", CreatedAt: mustTime(t, "10:00:01")},
	}}
	c := newContent(log, 80, 20)
	before := c.table.Cursor()

	_, _ = c.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("state = %v, want Active after scroll key", state)
	}
	if c.table.Cursor() == before {
		t.Errorf("table cursor did not move after %q press (still at %d) — key not forwarded", "j", before)
	}
}

// New must return a *modal.Modal whose ID matches the requested value,
// even when the log is empty.
func TestNew_BuildsModalWithID(t *testing.T) {
	m := New("notifications-x", &fakeLog{}, 60, 20)
	if m == nil {
		t.Fatal("New returned nil modal")
	}
	if m.ID() != "notifications-x" {
		t.Errorf("ID = %q, want %q", m.ID(), "notifications-x")
	}
}

// renderSeverity wraps the canonical severity label from the styles
// package in the matching style. The vocabulary itself is locked by
// styles.SeverityLabel's own test; this test asserts the modal applies
// some styling on top.
func TestRenderSeverity_AppliesStyle(t *testing.T) {
	cases := []errs.Severity{errs.SeverityInfo, errs.SeverityWarning, errs.SeverityError}
	for _, s := range cases {
		label := styles.SeverityLabel(s)
		rendered := renderSeverity(s)
		if !strings.Contains(rendered, label) {
			t.Errorf("renderSeverity(%v) = %q, want substring %q", s, rendered, label)
		}
		if len(rendered) <= len(label) {
			t.Errorf("renderSeverity(%v) = %q (len=%d), expected styling to add ANSI escapes around %q (len=%d)",
				s, rendered, len(rendered), label, len(label))
		}
	}
}
