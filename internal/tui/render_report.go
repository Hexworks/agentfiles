package tui

import (
	"fmt"
	"strings"

	"github.com/addamsson/agentfiles/internal/doctor"
	"github.com/charmbracelet/lipgloss"
)

// RenderReport returns the user-facing string representation of a doctor
// Report. It mirrors the previous string output of doctor.CheckProfile but
// adds icons and color, since the doctor package no longer formats anything.
func RenderReport(report *doctor.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", headerStyle.Render("Profile:"), safe(report.ProfileName))
	for _, status := range report.Projects {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "%s\n", headerStyle.Render("["+safe(status.Name)+"]"))
		if status.IsClean() {
			b.WriteString(cleanStyle.Render("clean"))
			b.WriteByte('\n')
			continue
		}
		for _, change := range status.Changes {
			icon, style := projectChangeStyle(change.Kind)
			line := fmt.Sprintf("%s %s %s", icon, change.Kind, safe(change.Path))
			b.WriteString(style.Render(line))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// projectChangeStyle picks the icon and lipgloss style for a doctor
// ProjectChange kind. It mirrors changeStyle's mapping but consumes
// doctor's vocabulary so the report renderer does not need to import
// sync.
func projectChangeStyle(kind doctor.ChangeKind) (string, lipgloss.Style) {
	switch kind {
	case doctor.ChangeCreate:
		return "+", createStyle
	case doctor.ChangeUpdate:
		return "~", updateStyle
	case doctor.ChangeDrift:
		return "!", driftStyle
	case doctor.ChangeDelete:
		return "-", deleteStyle
	}
	return "?", mutedStyle
}
