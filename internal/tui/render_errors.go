package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/styles"
)

// RenderError formats a single error value for display. errs.Collect
// flattens errors.Join chains and single-wrap errors, so both an
// errs.Errors value and a wrapper around one can be rendered uniformly.
// Errors that do not implement errs.DomainError fall back to a generic
// error severity so unexpected failures still render visibly.
func RenderError(err error) string {
	if err == nil {
		return ""
	}
	domainErrs := errs.Collect(err)
	if len(domainErrs) == 0 {
		// Fall back: no typed leaves, treat the raw message as an error.
		return errorStyle.Render("✗ "+safe(err.Error())) + "\n"
	}
	return RenderErrors(domainErrs)
}

// RenderErrors formats a slice of typed domain errors. Each element is
// rendered on its own line with an icon and color picked from its
// Severity().
func RenderErrors(domainErrs []errs.DomainError) string {
	if len(domainErrs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range domainErrs {
		b.WriteString(renderOneDomainError(e))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderOneDomainError(err errs.DomainError) string {
	icon, style := severityStyle(err.Severity())
	return style.Render(fmt.Sprintf("%s %s", icon, safe(err.Error())))
}

// severityStyle picks the icon and lipgloss style for a severity level.
// Delegates to the shared styles package so notifications and error
// rendering stay in sync.
func severityStyle(s errs.Severity) (string, lipgloss.Style) {
	return styles.SeverityStyle(s)
}
