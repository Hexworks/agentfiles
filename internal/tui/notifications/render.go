package notifications

import (
	"strconv"
	"unicode"

	"charm.land/lipgloss/v2"
)

// Style/icon vocabulary mirrors internal/tui/render_errors.go's
// severityStyle: INFO is cyan with the info glyph; ERROR is red bold
// with the cross glyph. Duplicated here instead of imported to avoid
// the tui→notifications→tui import cycle that task 0021 introduces
// when the shell mounts the notification area. Lift into a shared
// internal/tui/styles package after 0021 lands.
var (
	infoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func iconAndStyleFor(l Level) (string, lipgloss.Style) {
	if l == LevelError {
		return "✗", errorStyle
	}
	return "ℹ", infoStyle
}

func renderNotification(n Notification) string {
	icon, style := iconAndStyleFor(n.Level)
	return style.Render(icon + " " + safe(n.Text))
}

// safe is a local copy of internal/tui/styles.go::safe. Kept in sync
// manually until the styles helpers are lifted into a shared package.
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

func isTerminalSafeRune(r rune) bool {
	switch r {
	case '\n', '\t':
		return true
	}
	if r < 0x20 || r == 0x7f {
		return false
	}
	if r >= 0x80 && r <= 0x9f {
		return false
	}
	if (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
		return false
	}
	if r == 0xfeff || (r >= 0x200b && r <= 0x200d) {
		return false
	}
	if unicode.IsControl(r) {
		return false
	}
	return true
}
