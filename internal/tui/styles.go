package tui

import "github.com/hexworks/agentfiles/internal/tui/styles"

// Local re-aliases keep this package's render helpers reading like the
// pre-extraction code while the canonical definitions live in
// internal/tui/styles.
var (
	headerStyle = styles.HeaderStyle
	cleanStyle  = styles.CleanStyle
	mutedStyle  = styles.MutedStyle

	createStyle = styles.CreateStyle
	updateStyle = styles.UpdateStyle
	driftStyle  = styles.DriftStyle
	deleteStyle = styles.DeleteStyle

	errorStyle = styles.ErrorStyle
	warnStyle  = styles.WarnStyle
	infoStyle  = styles.InfoStyle
)

func safe(s string) string { return styles.Safe(s) }
