package shell

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
// (2 cols per side) applied to every column in the package's tables.
// Lifted to a package-level constant so each screen's column-sizer reads
// the same value.
const tableCellPadding = 8
