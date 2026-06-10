package notifications

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// ExpireNow produces an expireMsg for the current sequence number. The
// external _test package uses this to drive expiry without waiting on
// real wall-clock ticks. Lives in export_test.go so the helper exists
// only at test time and never reaches the production binary.
func ExpireNow(t *Toast) tea.Msg {
	return expireMsg{seq: t.seq}
}

// StaleExpire produces an expireMsg for the previous sequence number,
// simulating a tick from a now-displaced front-of-queue toast arriving
// after a new toast has taken its place.
func StaleExpire(t *Toast) tea.Msg {
	return expireMsg{seq: t.seq - 1}
}

// ToastDuration exposes the unexported duration field for the default-
// mapping test. Replaces the production-API Duration() getter that
// previously leaked test concerns onto *Toast.
func ToastDuration(t *Toast) time.Duration {
	return t.duration
}
