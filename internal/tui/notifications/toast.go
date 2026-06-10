package notifications

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// DefaultToastDuration is how long each toast stays visible before the
// next entry in the queue takes its place.
const DefaultToastDuration = 5 * time.Second

// PushMsg enqueues a notification on the toast. The shell sends one of
// these on receipt of NotificationMsg from the bridge.
type PushMsg struct {
	Notification Notification
}

// expireMsg fires after the visible toast's duration elapses. The seq
// field guards against a stale tick from a previous front-of-queue
// toast displacing the current one. A bare expireMsg whose seq does
// not match the toast's current seq is a no-op.
type expireMsg struct {
	seq uint64
}

// Toast is the FIFO queue of pending notifications. Only the front of
// the queue is visible at any moment; the rest wait for the current
// one's duration to elapse. The duration is injectable so tests can
// run with a long tick without ever firing real wall-clock expiry.
type Toast struct {
	duration time.Duration
	queue    []Notification
	seq      uint64
}

// NewToast constructs a Toast widget. Pass 0 to use
// DefaultToastDuration.
func NewToast(duration time.Duration) *Toast {
	if duration == 0 {
		duration = DefaultToastDuration
	}
	return &Toast{duration: duration}
}

// Init satisfies the bubbletea component contract. The toast has no
// startup work to do.
func (t *Toast) Init() tea.Cmd { return nil }

// Update handles PushMsg (enqueue) and expireMsg (advance the queue).
// Other messages are ignored.
func (t *Toast) Update(msg tea.Msg) (*Toast, tea.Cmd) {
	switch m := msg.(type) {
	case PushMsg:
		becameFront := len(t.queue) == 0
		t.queue = append(t.queue, m.Notification)
		if becameFront {
			return t, t.scheduleExpire()
		}
		return t, nil
	case expireMsg:
		if m.seq != t.seq || len(t.queue) == 0 {
			return t, nil
		}
		t.queue = t.queue[1:]
		if len(t.queue) == 0 {
			return t, nil
		}
		return t, t.scheduleExpire()
	}
	return t, nil
}

// View renders the front of the queue. Empty queue → empty string so
// the area can stay invisible.
func (t *Toast) View() string {
	if len(t.queue) == 0 {
		return ""
	}
	return renderNotification(t.queue[0])
}

// Empty reports whether the queue has no visible toast.
func (t *Toast) Empty() bool { return len(t.queue) == 0 }

// scheduleExpire bumps seq and returns a tick command that produces
// expireMsg{seq} after t.duration. The seq guard means any prior
// expireMsg from an earlier front-of-queue cycle that arrives late is
// a no-op.
func (t *Toast) scheduleExpire() tea.Cmd {
	t.seq++
	seq := t.seq
	d := t.duration
	return tea.Tick(d, func(time.Time) tea.Msg {
		return expireMsg{seq: seq}
	})
}
