package notificationsmodal

import (
	"charm.land/bubbles/v2/table"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// Column widths are tuned for the level + time slots; everything else
// goes to the content column. Widths include the per-cell padding that
// bubbles/table draws inside the column, so they must be wide enough to
// hold the header label and the longest data value combined with the
// padding — otherwise the header truncates to "severi…". The minimum
// content width keeps the table readable when the host shrinks the modal.
const (
	levelColW   = 9
	timeColW    = 10
	minContentW = 10
	// modalHFrame reserves space for the modal's captioned-panel frame:
	// 2 cols for the panel border + 4 cols for the Padding(1, 2) the
	// modal applies to content when [modal.WithCaption] is used.
	modalHFrame = 6
	// modalVFrame reserves space for the panel border (2 rows) plus the
	// modal's Padding(1, 2) vertical inset (2 rows).
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

func buildTable(entries []notifications.Notification, innerW int) ([]table.Column, []table.Row) {
	contentW := innerW - levelColW - timeColW - 3*cellPadding
	if contentW < minContentW {
		contentW = minContentW
	}
	cols := []table.Column{
		{Title: "Level", Width: levelColW},
		{Title: "Message", Width: contentW},
		{Title: "Time", Width: timeColW},
	}
	rows := make([]table.Row, len(entries))
	for i, e := range entries {
		rows[i] = table.Row{
			renderSeverity(e.Severity),
			styles.Safe(e.Text),
			e.CreatedAt.Format("15:04:05"),
		}
	}
	return cols, rows
}

func buildTableModel(entries []notifications.Notification, outerW, outerH int) table.Model {
	innerW, innerH := innerSize(outerW, outerH)
	cols, rows := buildTable(entries, innerW)
	return table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(innerH),
		table.WithWidth(innerW),
		table.WithStyles(styles.TableStyles()),
	)
}

// renderSeverity applies the shared severity style to the canonical
// label so the column matches the rest of the TUI's severity palette.
func renderSeverity(s errs.Severity) string {
	_, style := styles.SeverityStyle(s)
	return style.Render(styles.SeverityLabel(s))
}
