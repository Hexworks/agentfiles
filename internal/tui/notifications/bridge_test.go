package notifications_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

type fakeErr struct {
	severity errs.Severity
	text     string
}

func (e fakeErr) Error() string           { return e.text }
func (e fakeErr) Severity() errs.Severity { return e.severity }

func TestFrom_SuccessProducesInfoNotificationMsg(t *testing.T) {
	cmd := notifications.From(func() (int, errs.DomainError) {
		return 42, nil
	}, "created")

	msg := cmd().(notifications.NotificationMsg)
	if msg.Notification.Level != notifications.LevelInfo {
		t.Fatalf("expected LevelInfo, got %q", msg.Notification.Level)
	}
	if msg.Notification.Text != "created" {
		t.Fatalf("expected text 'created', got %q", msg.Notification.Text)
	}
}

func TestFrom_ErrorProducesErrorNotificationMsg(t *testing.T) {
	cmd := notifications.From(func() (struct{}, errs.DomainError) {
		return struct{}{}, fakeErr{severity: errs.SeverityError, text: "boom"}
	}, "created")

	msg := cmd().(notifications.NotificationMsg)
	if msg.Notification.Level != notifications.LevelError {
		t.Fatalf("expected LevelError, got %q", msg.Notification.Level)
	}
	if msg.Notification.Text != "boom" {
		t.Fatalf("expected text 'boom', got %q", msg.Notification.Text)
	}
}

func TestFrom_TimestampPopulated(t *testing.T) {
	before := time.Now()
	cmd := notifications.From(func() (int, errs.DomainError) {
		return 0, nil
	}, "ok")

	msg := cmd().(notifications.NotificationMsg)
	after := time.Now()

	if msg.Notification.CreatedAt.Before(before) || msg.Notification.CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v outside [%v, %v]", msg.Notification.CreatedAt, before, after)
	}
}

func TestFrom_ErrsErrorsRenderedViaErrorMethod(t *testing.T) {
	cmd := notifications.From(func() (struct{}, errs.DomainError) {
		return struct{}{}, errs.Errors{
			fakeErr{severity: errs.SeverityError, text: "first"},
			fakeErr{severity: errs.SeverityWarning, text: "second"},
		}
	}, "")

	msg := cmd().(notifications.NotificationMsg)
	if msg.Notification.Level != notifications.LevelError {
		t.Fatalf("expected LevelError, got %q", msg.Notification.Level)
	}
	if !strings.Contains(msg.Notification.Text, "first") ||
		!strings.Contains(msg.Notification.Text, "second") {
		t.Fatalf("expected both errors in text, got %q", msg.Notification.Text)
	}
}
