package notifications

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	notlog "github.com/hexworks/agentfiles/internal/tui/notifications"
)

// fakeLog is a fixed-content LogReader for deterministic tests. Real
// notifications.Log is also safe to use, but a stub avoids importing
// the ring-buffer's full surface and makes the input list explicit.
type fakeLog struct {
	entries []notlog.Notification
}

func (f *fakeLog) Entries() []notlog.Notification { return f.entries }

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
	log := &fakeLog{entries: []notlog.Notification{
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

	boomIdx := strings.Index(view, "boom")
	helloIdx := strings.Index(view, "hello")
	if boomIdx == -1 || helloIdx == -1 || boomIdx >= helloIdx {
		t.Errorf("entries out of order: boom@%d, hello@%d (want boom before hello)\n%s", boomIdx, helloIdx, view)
	}
}

// An empty log must surface the EmptyMessage rather than render a
// header-only table or crash on a zero-row bubbles/table.
func TestView_EmptyLogShowsPlaceholder(t *testing.T) {
	c := newContent(&fakeLog{}, 80, 20)
	view := c.View()
	if !strings.Contains(view, EmptyMessage) {
		t.Errorf("view = %q, want substring %q", view, EmptyMessage)
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
// lifecycle; otherwise scrolling closes the modal.
func TestUpdate_NonCloseKeyKeepsActive(t *testing.T) {
	log := &fakeLog{entries: []notlog.Notification{
		{Severity: errs.SeverityInfo, Text: "row1", CreatedAt: mustTime(t, "10:00:00")},
		{Severity: errs.SeverityInfo, Text: "row2", CreatedAt: mustTime(t, "10:00:01")},
	}}
	c := newContent(log, 80, 20)
	_, _ = c.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if state, _ := c.Lifecycle(); state != modal.Active {
		t.Fatalf("state = %v, want Active after scroll key", state)
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

// The level label is what the host can grep for in the rendered string;
// styles only add ANSI escape codes around it. Locking the label set
// here protects the visible vocabulary from accidental rename.
func TestLevelLabel_Vocabulary(t *testing.T) {
	cases := []struct {
		severity errs.Severity
		want     string
	}{
		{errs.SeverityInfo, "INFO"},
		{errs.SeverityWarning, "WARN"},
		{errs.SeverityError, "ERROR"},
	}
	for _, tc := range cases {
		got := levelLabel(tc.severity)
		if got != tc.want {
			t.Errorf("levelLabel(%v) = %q, want %q", tc.severity, got, tc.want)
		}
	}
}
