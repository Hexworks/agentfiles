// Package notifications hosts the Notifications modal: a bubbles/table
// widget mounted inside a [modal.Modal] that renders the in-memory
// notification ring buffer (newest first). It reads from a [LogReader]
// once on open — re-opening pulls fresh data — and closes on esc/q.
//
// The package is named "notifications" to match the user-facing label;
// it is imported alongside the [internal/tui/notifications] log package
// using an alias (`notlog`) to keep both namespaces visible.
package notifications

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	notlog "github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// EmptyMessage is the body shown when the log is empty. Exported so
// tests can assert the exact string.
const EmptyMessage = "No notifications yet"

// LogReader is the narrow read-side of the notifications log the modal
// depends on. Defined consumer-side so the modal package does not
// reach into the concrete log implementation.
type LogReader interface {
	Entries() []notlog.Notification
}

type keymap struct {
	Close key.Binding
}

func defaultKeymap() keymap {
	return keymap{Close: key.NewBinding(key.WithKeys("esc", "q"))}
}

// content implements [modal.Content] for the notifications dialog.
// When the log is empty, table is zero-valued and View renders
// [EmptyMessage] instead.
type content struct {
	table table.Model
	empty bool
	state modal.LifecycleState
	keys  keymap
}

// New constructs a Notifications [modal.Modal] sized for (width x height).
// The log is snapshotted at construction time; a closed-then-reopened
// modal will pull fresh entries.
func New(id string, log LogReader, width, height int) *modal.Modal {
	c := newContent(log, width, height)
	return modal.New(id, c, modal.WithStyle(styles.ModalStyle))
}

func newContent(log LogReader, width, height int) *content {
	entries := log.Entries()
	c := &content{
		keys:  defaultKeymap(),
		empty: len(entries) == 0,
	}
	if c.empty {
		return c
	}
	innerW, innerH := innerSize(width, height)
	cols, rows := buildTable(entries, innerW)
	c.table = table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(innerH),
		table.WithWidth(innerW),
	)
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
		return EmptyMessage
	}
	return c.table.View()
}

func (c *content) Lifecycle() (modal.LifecycleState, any) {
	return c.state, nil
}

// Column widths are tuned for the level + time slots; everything else
// goes to the content column. The minimum content width keeps the
// table readable when the host shrinks the modal.
const (
	levelColW   = 7
	timeColW    = 8
	minContentW = 10
	// modalHFrame reserves space for ModalStyle's rounded border (2
	// cols) plus padding(1, 2) (4 cols).
	modalHFrame = 6
	// modalVFrame reserves space for the rounded border (2 rows) plus
	// padding(1, 2) (2 rows).
	modalVFrame = 4
	// cellPadding is the per-column horizontal padding bubbles/table
	// adds via its Cell style (Padding(0, 1)). Subtracted so the row
	// fits inside the modal's inner width.
	cellPadding = 2
)

// innerSize returns the table's drawable rect after deducting the
// modal frame from the requested outer dimensions. Both axes clamp at
// 1 so a tiny terminal does not feed bubbles/table a non-positive
// dimension.
func innerSize(width, height int) (int, int) {
	w := width - modalHFrame
	if w < minContentW+levelColW+timeColW+3*cellPadding {
		w = minContentW + levelColW + timeColW + 3*cellPadding
	}
	h := height - modalVFrame
	if h < 1 {
		h = 1
	}
	return w, h
}

func buildTable(entries []notlog.Notification, innerW int) ([]table.Column, []table.Row) {
	contentW := innerW - levelColW - timeColW - 3*cellPadding
	if contentW < minContentW {
		contentW = minContentW
	}
	cols := []table.Column{
		{Title: "level", Width: levelColW},
		{Title: "content", Width: contentW},
		{Title: "time", Width: timeColW},
	}
	rows := make([]table.Row, len(entries))
	for i, e := range entries {
		rows[i] = table.Row{
			renderLevel(e.Severity),
			styles.Safe(e.Text),
			e.CreatedAt.Format("15:04:05"),
		}
	}
	return cols, rows
}

// levelLabel is the short form of a severity used in the table's level
// column. Kept private; tests rely on the rendered text via
// [styles.SeverityStyle].
func levelLabel(s errs.Severity) string {
	switch s {
	case errs.SeverityError:
		return "ERROR"
	case errs.SeverityWarning:
		return "WARN"
	}
	return "INFO"
}

func renderLevel(s errs.Severity) string {
	_, style := styles.SeverityStyle(s)
	return style.Render(levelLabel(s))
}
