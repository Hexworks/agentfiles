package shell

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
)

// mutationDoneMsg envelopes a finished Create / Update / Delete action.
// Screens react with tea.Batch(notification, reload) — using one shared
// envelope lets every screen reuse mutationCmd without forking a per-screen
// message type.
type mutationDoneMsg struct {
	text     string
	severity errs.Severity
}

// mutationCmd runs action synchronously inside the Cmd closure and returns
// a mutationDoneMsg regardless of outcome. The action is guaranteed
// complete before the message is dispatched.
func mutationCmd(action func() errs.DomainError, successText string) tea.Cmd {
	return func() tea.Msg {
		if err := action(); err != nil {
			return mutationDoneMsg{text: err.Error(), severity: err.Severity()}
		}
		return mutationDoneMsg{text: successText, severity: errs.SeverityInfo}
	}
}
