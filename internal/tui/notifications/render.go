package notifications

import (
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// renderNotification formats a notification using the shared severity
// icon/style vocabulary. INFO uses cyan + ℹ, WARNING uses yellow + ⚠,
// ERROR uses red bold + ✗ — identical to the rest of the TUI's error
// rendering so a warning notification and a warning error look the same.
func renderNotification(n Notification) string {
	icon, style := styles.SeverityStyle(n.Severity)
	return style.Render(icon + " " + styles.Safe(n.Text))
}
