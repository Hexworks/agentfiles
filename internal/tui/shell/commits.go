package shell

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/errs"
)

// notificationText composes the merged save-plus-commit toast text a
// git-aware screen surfaces after a successful mutation.
//
// The discriminated CommitOutcome resolves to one of three branches:
//   - appapi.Committed → base becomes "<base> (committed <sha>)" so
//     the existing save toast picks up the commit signal without a
//     second notification.
//   - appapi.Failed → base stays untouched and the second return
//     carries the warn toast the caller batches alongside it.
//   - appapi.Skipped (any reason) or nil → base returned unchanged
//     and the second value is nil (silent skip per ADR 0019).
func notificationText(base string, outcome appapi.CommitOutcome) (string, tea.Cmd) {
	switch v := outcome.(type) {
	case appapi.Committed:
		return base + " (committed " + v.SHA + ")", nil
	case appapi.Failed:
		return base, notificationCmd(v.Err.Severity(), v.Err.Error())
	default: // appapi.Skipped or nil
		return base, nil
	}
}

// commitOutcomeCmd is a convenience wrapper for post-mutation screens
// that want to fire the info toast and, if the commit failed, an extra
// warn toast in one tea.Batch. severity is always errs.SeverityInfo for
// the info toast because the save itself succeeded — the commit
// failure rides on the second toast.
func commitOutcomeCmd(base string, outcome appapi.CommitOutcome) tea.Cmd {
	text, warn := notificationText(base, outcome)
	info := notificationCmd(errs.SeverityInfo, text)
	if warn == nil {
		return info
	}
	return tea.Batch(info, warn)
}

// syncCommitOutcomeCmd merges the primary sync commit and the ADR 0020
// Adopt commit outcomes into one info toast plus any warning toasts
// their Failed variants demand. Success text follows the sync/adopt
// pattern:
//   - both Committed → "Project synced (committed <sync>; profile <adopt>)"
//   - sync Committed only → "Project synced (committed <sync>)"
//   - adopt Committed only → "Project synced (profile <adopt>)"
//   - neither Committed → base unchanged.
func syncCommitOutcomeCmd(base string, sync, adopt appapi.CommitOutcome) tea.Cmd {
	text := base
	var warns []tea.Cmd
	if syncCommitted, ok := sync.(appapi.Committed); ok {
		text = base + " (committed " + syncCommitted.SHA + ")"
	} else if failed, ok := sync.(appapi.Failed); ok {
		warns = append(warns, notificationCmd(failed.Err.Severity(), failed.Err.Error()))
	}
	if adoptCommitted, ok := adopt.(appapi.Committed); ok {
		if text == base {
			text = base + " (profile " + adoptCommitted.SHA + ")"
		} else {
			text = text[:len(text)-1] + "; profile " + adoptCommitted.SHA + ")"
		}
	} else if failed, ok := adopt.(appapi.Failed); ok {
		warns = append(warns, notificationCmd(failed.Err.Severity(), failed.Err.Error()))
	}
	info := notificationCmd(errs.SeverityInfo, text)
	if len(warns) == 0 {
		return info
	}
	return tea.Batch(append([]tea.Cmd{info}, warns...)...)
}
