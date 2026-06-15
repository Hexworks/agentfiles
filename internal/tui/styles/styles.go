// Package styles holds the shared TUI presentation primitives — color
// constants, lipgloss styles, the severity icon/style switch, and the
// terminal-safe string sanitizer. Both internal/tui and
// internal/tui/notifications depend on this leaf package, so the styles
// vocabulary stays in one place and no import cycle appears when the
// shell mounts notification components.
package styles

import (
	"strconv"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/hexworks/agentfiles/internal/errs"
)

// ANSI palette indices used across the render helpers. Naming them
// means a palette change touches one constant instead of every style
// declaration.
var (
	ColorMuted   = lipgloss.Color("8")  // dim grey
	ColorRed     = lipgloss.Color("9")  // red
	ColorGreen   = lipgloss.Color("10") // bright green
	ColorYellow  = lipgloss.Color("11") // yellow
	ColorMagenta = lipgloss.Color("13") // magenta
	ColorCyan    = lipgloss.Color("14") // cyan
)

// Mnemonic-button palette. Distinct from the severity palette so the
// activating-key hint reads as a control glyph rather than a status.
// ColorAccent paints the surrounding `[`/`]` brackets; ColorMnemonic
// paints the highlighted shortcut letter (rendered bold + underlined);
// ColorText paints the rest of the label.
var (
	ColorAccent   = ColorRed
	ColorMnemonic = ColorGreen
	ColorText     = ColorMuted
)

// Lipgloss styles consumed by the render helpers and by the
// notifications subsystem. Centralizing them keeps future palette
// changes local; callers only reference these vars.
var (
	HeaderStyle = lipgloss.NewStyle().Bold(true)
	CleanStyle  = lipgloss.NewStyle().Foreground(ColorGreen)
	MutedStyle  = lipgloss.NewStyle().Foreground(ColorMuted)

	CreateStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	UpdateStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	DriftStyle  = lipgloss.NewStyle().Foreground(ColorMagenta)
	DeleteStyle = lipgloss.NewStyle().Foreground(ColorRed)

	ErrorStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	WarnStyle  = lipgloss.NewStyle().Foreground(ColorYellow)
	InfoStyle  = lipgloss.NewStyle().Foreground(ColorCyan)

	// ModalStyle is the themed border for components/modal callers.
	// Pass it via modal.WithStyle so the modal package itself stays
	// palette-free.
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(1, 2)
)

// SeverityStyle picks the icon and lipgloss style for a domain severity.
// The notifications subsystem and the error renderer share this switch
// so a warning notification and a warning error look identical.
func SeverityStyle(s errs.Severity) (string, lipgloss.Style) {
	switch s {
	case errs.SeverityError:
		return "✗", ErrorStyle
	case errs.SeverityWarning:
		return "⚠", WarnStyle
	}
	return "ℹ", InfoStyle
}

// SeverityLabel returns the short uppercase label for a severity used
// by table-shaped renderers (e.g. the notifications modal). Centralized
// so every severity-aware view picks the same vocabulary.
func SeverityLabel(s errs.Severity) string {
	switch s {
	case errs.SeverityError:
		return "ERROR"
	case errs.SeverityWarning:
		return "WARN"
	}
	return "INFO"
}

// Safe returns s rendered as a terminal-safe string. Without this, a
// manifest field or attacker-controlled filename containing raw escape
// sequences (e.g. \x1b[2J or OSC title changes), C1 controls, bidi
// overrides, or zero-width formatters could hijack terminal state or
// hide path segments behind a flipped glyph.
//
// Strategy: if every rune in s is "obviously safe" (printable; newline
// or tab) the string is returned unchanged. As soon as a hostile rune
// is detected the entire string is returned via strconv.Quote so the
// user sees the literal escaped form (`"foo‮dlrow.txt"`) rather than
// the rendered glyph. Coarser than per-rune stripping, but deterministic
// and impossible to misread.
func Safe(s string) string {
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
