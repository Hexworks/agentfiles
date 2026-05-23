package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// RenderPreview returns the user-facing string representation of a sync
// preview. It replaces the previous sync.FormatPreview helper so all
// presentation lives in the TUI package.
func RenderPreview(preview *llmsync.Preview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", headerStyle.Render("Project:"), safe(preview.ProjectPath))
	if len(preview.Changes) == 0 {
		b.WriteString(cleanStyle.Render("No changes."))
		b.WriteByte('\n')
		return b.String()
	}
	for _, change := range preview.Changes {
		icon, style := changeStyle(change.Kind)
		line := fmt.Sprintf("%s [%s] %s: %s", icon, change.Kind, safe(change.Path), safe(change.Reason))
		b.WriteString(style.Render(line))
		b.WriteByte('\n')
	}
	return b.String()
}

// changeStyle picks the icon and lipgloss style for a single change kind.
// Kept private because it is only meaningful as part of preview rendering.
func changeStyle(kind llmsync.ChangeKind) (string, lipgloss.Style) {
	switch kind {
	case llmsync.ChangeCreate:
		return "+", createStyle
	case llmsync.ChangeUpdate:
		return "~", updateStyle
	case llmsync.ChangeDrift:
		return "!", driftStyle
	case llmsync.ChangeDelete:
		return "-", deleteStyle
	}
	return "?", mutedStyle
}
