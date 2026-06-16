// Package notificationsmodal hosts the Notifications modal: a
// bubbles/table widget mounted inside a [modal.Modal] that renders the
// in-memory notification ring buffer (newest first). It reads from a
// [LogReader] once on open — re-opening pulls fresh data — and closes
// on esc/q.
//
// The package is named after its role (the modal view) so importers
// can reach for it without aliasing it apart from the
// [internal/tui/notifications] log package.
package notificationsmodal

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// emptyMessage is the body shown when the log is empty. Tests redeclare
// the expected text locally so the modal's wording stays an internal
// contract.
const emptyMessage = "No notifications yet"

// modalCaption is the label embedded in the modal's top border via
// [modal.WithCaption]. Centralized so the constructor and any tests
// that assert the visible caption agree on the wording.
const modalCaption = "Notifications"

// LogReader is the narrow read-side of the notifications log the modal
// depends on. Defined consumer-side so the modal package does not
// reach into the concrete log implementation.
type LogReader interface {
	Entries() []notifications.Notification
}

type keymap struct {
	Close key.Binding
}

func defaultKeymap() keymap {
	return keymap{Close: key.NewBinding(key.WithKeys("esc", "q"))}
}

// content implements [modal.Content] for the notifications dialog.
// When the log is empty, table is zero-valued and View renders
// [emptyMessage] instead. The snapshot of log entries is taken once at
// construction; [SetSize] reuses that snapshot to rebuild the table on
// terminal resizes without re-reading from the log.
type content struct {
	entries []notifications.Notification
	table   table.Model
	empty   bool
	state   modal.LifecycleState
	keys    keymap
}

// New constructs a Notifications [modal.Modal] sized for (width x height).
// The log is snapshotted at construction time; a closed-then-reopened
// modal will pull fresh entries. The dialog is framed by a captioned
// panel via [modal.WithCaption] so the "Notifications" label sits in the
// top border the same way it does on every other table-shaped view.
func New(id string, log LogReader, width, height int) *modal.Modal {
	c := newContent(log, width, height)
	return modal.New(id, c, modal.WithCaption(modalCaption))
}

func newContent(log LogReader, width, height int) *content {
	entries := log.Entries()
	c := &content{
		entries: entries,
		keys:    defaultKeymap(),
		empty:   len(entries) == 0,
	}
	if c.empty {
		return c
	}
	c.table = buildTableModel(entries, width, height)
	return c
}

func (c *content) Init() tea.Cmd { return nil }

func (c *content) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kp, c.keys.Close) {
			c.state = modal.Cancelled
			return c, nil
		}
	}
	if c.empty {
		return c, nil
	}
	var cmd tea.Cmd
	c.table, cmd = c.table.Update(msg)
	return c, cmd
}

func (c *content) View() string {
	if c.empty {
		return emptyMessage
	}
	return c.table.View()
}

func (c *content) Lifecycle() (modal.LifecycleState, any) {
	return c.state, nil
}

// SetSize re-runs the table layout for new outer (modal) dimensions.
// Implements the resizable contract the [modal.Modal] wrapper checks on
// terminal resize events. Empty-log content is dimension-independent.
func (c *content) SetSize(width, height int) {
	if c.empty {
		return
	}
	innerW, innerH := innerSize(width, height)
	cols, _ := buildTable(c.entries, innerW)
	c.table.SetColumns(cols)
	c.table.SetWidth(innerW)
	c.table.SetHeight(innerH)
}
