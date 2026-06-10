package notifications_test

import (
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
	if msg.Notification.Severity != errs.SeverityInfo {
		t.Fatalf("expected SeverityInfo, got %v", msg.Notification.Severity)
	}
	if msg.Notification.Text != "created" {
		t.Fatalf("expected text 'created', got %q", msg.Notification.Text)
	}
}

func TestFrom_ErrorProducesTypedSeverityNotificationMsg(t *testing.T) {
	cmd := notifications.From(func() (struct{}, errs.DomainError) {
		return struct{}{}, fakeErr{severity: errs.SeverityError, text: "boom"}
	}, "created")

	msg := cmd().(notifications.NotificationMsg)
	if msg.Notification.Severity != errs.SeverityError {
		t.Fatalf("expected SeverityError, got %v", msg.Notification.Severity)
	}
	if msg.Notification.Text != "boom" {
		t.Fatalf("expected text 'boom', got %q", msg.Notification.Text)
	}
}

func TestFrom_WarningSeverityPreserved(t *testing.T) {
	cmd := notifications.From(func() (struct{}, errs.DomainError) {
		return struct{}{}, fakeErr{severity: errs.SeverityWarning, text: "soft"}
	}, "")

	msg := cmd().(notifications.NotificationMsg)
	if msg.Notification.Severity != errs.SeverityWarning {
		t.Fatalf("expected SeverityWarning preserved (not collapsed to Error), got %v", msg.Notification.Severity)
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
	if msg.Notification.Severity != errs.SeverityError {
		t.Fatalf("expected SeverityError (highest in slice), got %v", msg.Notification.Severity)
	}
	if msg.Notification.Text != "first\nsecond" {
		t.Fatalf("expected exact joined text %q, got %q", "first\nsecond", msg.Notification.Text)
	}
}
