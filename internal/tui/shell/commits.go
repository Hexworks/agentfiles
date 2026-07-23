package shell

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/app"
	"github.com/hexworks/agentfiles/internal/errs"
)

// notificationText composes the merged save-plus-commit toast text a
// git-aware screen surfaces after a successful mutation. When the
// commit succeeded, base becomes "<base> (committed <sha>)" so the
// existing save toast picks up the commit signal without a second
// notification. When the commit failed, base stays untouched and the
// second return carries the warn toast the caller batches alongside
// it. When no commit was attempted (feature disabled or dir not a
// repo) base is returned unchanged and the second value is nil.
func notificationText(base string, outcome app.CommitOutcome) (string, tea.Cmd) {
	if outcome.SHA != "" {
		return base + " (committed " + outcome.SHA + ")", nil
	}
	if outcome.Err != nil {
		return base, notificationCmd(outcome.Err.Severity(), outcome.Err.Error())
	}
	return base, nil
}

// commitOutcomeCmd is a convenience wrapper for post-mutation screens
// that want to fire the info toast and, if the commit failed, an extra
// warn toast in one tea.Batch. severity is always errs.SeverityInfo for
// the info toast because the save itself succeeded — the commit
// failure rides on the second toast.
func commitOutcomeCmd(base string, outcome app.CommitOutcome) tea.Cmd {
	text, warn := notificationText(base, outcome)
	info := notificationCmd(errs.SeverityInfo, text)
	if warn == nil {
		return info
	}
	return tea.Batch(info, warn)
}
