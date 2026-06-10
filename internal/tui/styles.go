package tui

import (
	"strconv"
	"unicode"

	"charm.land/lipgloss/v2"
)

// ANSI palette indices used across the render helpers. Keeping them
// named means a palette change touches one constant instead of every
// style declaration.
var (
	colorMuted   = lipgloss.Color("8")  // dim grey
	colorRed     = lipgloss.Color("9")  // red
	colorGreen   = lipgloss.Color("10") // bright green
	colorYellow  = lipgloss.Color("11") // yellow
	colorMagenta = lipgloss.Color("13") // magenta
	colorCyan    = lipgloss.Color("14") // cyan
)

// Lipgloss styles used by the render_* helpers. Centralizing them keeps
// future palette changes local; callers only reference these vars.
var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	cleanStyle  = lipgloss.NewStyle().Foreground(colorGreen)
	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)

	createStyle = lipgloss.NewStyle().Foreground(colorGreen)
	updateStyle = lipgloss.NewStyle().Foreground(colorYellow)
	driftStyle  = lipgloss.NewStyle().Foreground(colorMagenta)
	deleteStyle = lipgloss.NewStyle().Foreground(colorRed)

	errorStyle = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(colorYellow)
	infoStyle  = lipgloss.NewStyle().Foreground(colorCyan)

	// modalStyle is the themed border for components/modal callers. Pass it
	// via modal.WithStyle so the modal package itself stays palette-free.
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorCyan).
			Padding(1, 2)
)

// safe returns s rendered as a terminal-safe string. Without this, a
// manifest field or attacker-controlled filename containing raw escape
// sequences (e.g. \x1b[2J or OSC title changes), C1 controls, bidi
// overrides, or zero-width formatters could hijack terminal state or
// hide path segments behind a flipped glyph.
//
// Strategy: if every rune in s is "obviously safe" (printable; newline
// or tab) the string is returned unchanged. As soon as a hostile rune
// is detected the entire string is returned via strconv.Quote so the
// user sees the literal escaped form (`"foo‮dlrow.txt"`) rather
// than the rendered glyph. Coarser than per-rune stripping, but
// deterministic and impossible to misread.
func safe(s string) string {
	if s == "" {
		return s
	}
	for _, r := range s {
		if !isTerminalSafeRune(r) {
			return strconv.Quote(s)
		}
	}
	return s
}

// isTerminalSafeRune reports whether r is safe to render directly into
// a lipgloss-styled terminal string. Newline and tab are allowed; every
// other control rune, C1 control, bidi override, or zero-width
// formatter is rejected.
func isTerminalSafeRune(r rune) bool {
	switch r {
	case '\n', '\t':
		return true
	}
	if r < 0x20 || r == 0x7f {
		return false
	}
	// C1 controls (U+0080..U+009F).
	if r >= 0x80 && r <= 0x9f {
		return false
	}
	// Bidi overrides and isolates (U+202A..U+202E, U+2066..U+2069).
	if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
		return false
	}
	// Zero-width formatters and BOM (U+200B..U+200D, U+FEFF).
	if r == 0xfeff || (r >= 0x200b && r <= 0x200d) {
		return false
	}
	// Any remaining Unicode-class control rune (Cc).
	if unicode.IsControl(r) {
		return false
	}
	return true
}
