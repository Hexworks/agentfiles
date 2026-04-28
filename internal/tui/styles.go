package tui

import "github.com/charmbracelet/lipgloss"

// ANSI palette indices used across the render helpers. Keeping them
// named means a palette change touches one constant instead of every
// style declaration.
const (
	colorMuted   lipgloss.Color = "8"  // dim grey
	colorRed     lipgloss.Color = "9"  // red
	colorGreen   lipgloss.Color = "10" // bright green
	colorYellow  lipgloss.Color = "11" // yellow
	colorMagenta lipgloss.Color = "13" // magenta
	colorCyan    lipgloss.Color = "14" // cyan
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
)

// safe strips ASCII control characters from a manifest-sourced string
// before it reaches the terminal. Without this, a manifest field
// containing raw escape sequences (e.g. \x1b[2J or OSC title changes)
// could hijack terminal state when rendered through lipgloss.
//
// Printable characters and newlines/tabs are preserved.
func safe(s string) string {
	if s == "" {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\t':
			b = append(b, c)
		case c < 0x20 || c == 0x7f:
			// drop ASCII control byte
		default:
			b = append(b, c)
		}
	}
	return string(b)
}
