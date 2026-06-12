package shell

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// Manual SGR underline so the surrounding bubbles table cursor-row highlight
// is not terminated by lipgloss's full-reset `\x1b[0m`. Lifted here so every
// row-action cell renders the same trick.
const (
	underlineOn  = "\x1b[4m"
	underlineOff = "\x1b[24m"
)

// underline wraps s in SGR underline-on / underline-off codes.
func underline(s string) string { return underlineOn + s + underlineOff }

// tableCellPadding is the bubbles/v2 default Cell-style padding
// (2 cols total) applied across the package's four-column tables.
const tableCellPadding = 8

// naturalColumns sizes each column to the maximum visible width of its
// title and any row's content for that column. Columns are content-fit
// so the table never expands to fill the terminal.
func naturalColumns(titles []string, rows []table.Row) []table.Column {
	widths := make([]int, len(titles))
	for i, t := range titles {
		widths[i] = lipgloss.Width(t)
	}
	for _, r := range rows {
		for i, cell := range r {
			if i >= len(widths) {
				break
			}
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	cols := make([]table.Column, len(titles))
	for i, t := range titles {
		cols[i] = table.Column{Title: t, Width: widths[i]}
	}
	return cols
}

// tableNaturalWidth returns the SetWidth value needed for cols to render
// at their natural content width, accounting for bubbles cell padding.
func tableNaturalWidth(cols []table.Column) int {
	sum := 0
	for _, c := range cols {
		sum += c.Width
	}
	return sum + tableCellPadding
}

// sanitizeCursor pulls a stale cursor back into the [0, rowCount) range
// before the screen reads it to decide which row gets the action cell.
//
// bubbles' [table.Model.SetRows] clamps cursor down when len(rows) drops
// (cursor=0 with a nil rows slice becomes cursor=-1), but it never
// raises the cursor back up when rows reappear. A Body() rendered before
// the load command completes therefore hands the table a nil row set,
// permanently parking cursor at -1 unless we put it back.
func sanitizeCursor(t *table.Model, rowCount int) {
	if rowCount == 0 {
		return
	}
	c := t.Cursor()
	if c < 0 || c >= rowCount {
		t.SetCursor(0)
	}
}
