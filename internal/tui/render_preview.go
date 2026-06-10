package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	llmsync "github.com/hexworks/agentfiles/internal/sync"
)

// RenderPreview returns the user-facing string representation of a sync
// preview. It replaces the previous sync.FormatPreview helper so all
// presentation lives in the TUI package.
func RenderPreview(preview *llmsync.Preview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", headerStyle.Render("Project:"), safe(preview.ProjectPath))
	if preview.FirstApply {
		b.WriteString(infoStyle.Render("First apply — every desired file will be created."))
		b.WriteByte('\n')
	}
	if len(preview.Changes) == 0 {
		b.WriteString(cleanStyle.Render("No changes."))
		b.WriteByte('\n')
		return b.String()
	}
	for _, change := range preview.Changes {
		icon, style := changeStyle(change.Kind)
		line := fmt.Sprintf("%s [%s] %s: %s", icon, change.Kind, safe(change.Path), reasonText(change.Reason))
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
	case llmsync.ChangeUnknown:
		return "?", driftStyle
	}
	return "?", mutedStyle
}

// reasonText translates a domain ReasonKind into user-facing prose.
// The sync engine emits ReasonKind constants only; all human-readable
// strings live here so a copy-edit (or future i18n pass) touches one
// file.
func reasonText(reason llmsync.ReasonKind) string {
	switch reason {
	case llmsync.ReasonFirstApply:
		return "first apply"
	case llmsync.ReasonFileMissing:
		return "file missing"
	case llmsync.ReasonContentDiffers:
		return "content differs"
	case llmsync.ReasonDriftDetected:
		return "managed file changed locally"
	case llmsync.ReasonStateRecordedDelete:
		return "recognized llm file not selected"
	case llmsync.ReasonUnknown:
		return "unknown file in managed surface"
	}
	return string(reason)
}
