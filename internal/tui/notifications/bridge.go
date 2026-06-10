package notifications

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
)

// NotificationMsg is the message produced by From. The shell routes it
// to both *Log.Add and *Toast.Update(PushMsg{...}).
type NotificationMsg struct {
	Notification Notification
}

// From runs action, then returns a tea.Cmd whose message is a
// NotificationMsg. On success the message carries SeverityInfo with
// successText; on failure it carries err.Severity() (the typed domain
// severity) with err.Error() text. The action's T return value is
// discarded — screens that need both the value and a notification call
// the action directly and dispatch both messages themselves.
//
// The notification's CreatedAt is captured at command-execution time
// (when the tea runtime runs the cmd), not at From's construction time.
func From[T any](action func() (T, errs.DomainError), successText string) tea.Cmd {
	return func() tea.Msg {
		_, err := action()
		now := time.Now()
		if err != nil {
			return NotificationMsg{Notification: Notification{
				Severity:  err.Severity(),
				Text:      err.Error(),
				CreatedAt: now,
			}}
		}
		return NotificationMsg{Notification: Notification{
			Severity:  errs.SeverityInfo,
			Text:      successText,
			CreatedAt: now,
		}}
	}
}
