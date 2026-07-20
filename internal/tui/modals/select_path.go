package modals

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/components/modal"
	"github.com/hexworks/agentfiles/internal/tui/modals/pathselector"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// NewSelectPath opens the reusable path-picker modal defined by
// [pathselector.Content]. A validation failure (unreadable constraint / start
// folder, start outside constraint) is returned as a typed [errs.DomainError]
// so the caller can surface it through the standard notification path.
//
// Runtime errors emitted by the modal (constraint violation, folder-read
// failure) leave the pathselector package as its own message types
// ([pathselector.ConstraintViolationMsg] / [pathselector.ReadDirErrorMsg])
// so the modal stays host-agnostic. This wrapper translates them into
// [notifications.NotificationMsg] on the way out — the shell only sees
// the notification shape.
//
// The returned [modal.Modal] is passed to the shell exactly like any other
// modal. Read the selection with [pathselector.ResultFromMsg] on the
// [modal.ResolvedMsg] the runtime dispatches when the user confirms.
func NewSelectPath(opts pathselector.Options) (*modal.Modal, errs.DomainError) {
	content, err := pathselector.New(opts)
	if err != nil {
		return nil, err
	}
	caption := opts.Caption
	if caption == "" {
		caption = "Select path"
	}
	return modal.New("select-path", &notificationTranslator{inner: content}, modal.WithCaption(caption)), nil
}

// notificationTranslator wraps a [pathselector.Content] and rewrites the
// pathselector package's local message types into
// [notifications.NotificationMsg] before they reach the shell. Keeping
// the translation in the wrapper (rather than baking notifications into
// pathselector itself) lets the modal be embedded in any host that
// surfaces errors differently.
type notificationTranslator struct {
	inner *pathselector.Content
}

func (t *notificationTranslator) Init() tea.Cmd {
	return wrapCmd(t.inner.Init())
}

func (t *notificationTranslator) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
	_, cmd := t.inner.Update(msg)
	return t, wrapCmd(cmd)
}

func (t *notificationTranslator) View() string { return t.inner.View() }

func (t *notificationTranslator) Lifecycle() (modal.LifecycleState, any) {
	return t.inner.Lifecycle()
}

// SetSize forwards resize events so the wrapped Content still satisfies
// [modal.Resizable] through the translator.
func (t *notificationTranslator) SetSize(width, height int) {
	t.inner.SetSize(width, height)
}

// wrapCmd wraps a tea.Cmd so its returned message is translated on
// execution. Nil cmds stay nil. Batch commands are unwrapped and each
// sub-command re-wrapped so the translation reaches every leaf message.
func wrapCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		return translateMsg(cmd())
	}
}

// translateMsg rewrites the pathselector-local error messages into
// [notifications.NotificationMsg]. Other messages (including tea's own
// framing types) pass through untouched; batch messages are recursively
// re-wrapped so the translation is total.
func translateMsg(msg tea.Msg) tea.Msg {
	switch m := msg.(type) {
	case pathselector.ConstraintViolationMsg:
		return notifications.NotificationMsg{
			Notification: notifications.Notification{
				Severity:  errs.SeverityError,
				Text:      "Cannot leave " + m.Constraint,
				CreatedAt: time.Now(),
			},
		}
	case pathselector.ReadDirErrorMsg:
		return notifications.NotificationMsg{
			Notification: notifications.Notification{
				Severity:  errs.SeverityError,
				Text:      fmt.Sprintf("cannot read directory %q: %s", m.Path, m.Err),
				CreatedAt: time.Now(),
			},
		}
	case tea.BatchMsg:
		out := make(tea.BatchMsg, len(m))
		for i, sub := range m {
			out[i] = wrapCmd(sub)
		}
		return out
	default:
		return msg
	}
}
