package notifications_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// safeDuration is large enough that an accidentally-executed tea.Tick
// cmd cannot fire during the test. The tests drive expiry manually via
// ExpireNow / StaleExpire helpers from export_test.go.
const safeDuration = time.Hour

func push(text string) notifications.PushMsg {
	return notifications.PushMsg{Notification: notifications.Notification{
		Severity: errs.SeverityInfo,
		Text:     text,
	}}
}

func TestToast_EmptyRendersEmptyString(t *testing.T) {
	toast := notifications.NewToast(safeDuration)
	if v := toast.View(); v != "" {
		t.Fatalf("expected empty view, got %q", v)
	}
	if !toast.Empty() {
		t.Fatal("expected Empty() true")
	}
}

func TestToast_FirstPushSchedulesExpire(t *testing.T) {
	toast := notifications.NewToast(safeDuration)

	next, cmd := toast.Update(push("hello"))
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd for first push")
	}
	if next.Empty() {
		t.Fatal("expected queue non-empty after push")
	}
}

func TestToast_SecondPushDoesNotResetTimer(t *testing.T) {
	toast := notifications.NewToast(safeDuration)
	toast, _ = toast.Update(push("first"))

	_, cmd := toast.Update(push("second"))
	if cmd != nil {
		t.Fatalf("expected no new tea.Cmd while a toast is already visible, got %v", cmd)
	}
}

func TestToast_QueueOrderIsFIFO(t *testing.T) {
	toast := notifications.NewToast(safeDuration)
	toast, _ = toast.Update(push("first"))
	toast, _ = toast.Update(push("second"))
	toast, _ = toast.Update(push("third"))

	if !strings.Contains(toast.View(), "first") {
		t.Fatalf("expected first visible, got %q", toast.View())
	}
	toast, _ = toast.Update(notifications.ExpireNow(toast))
	if !strings.Contains(toast.View(), "second") {
		t.Fatalf("expected second visible, got %q", toast.View())
	}
	toast, _ = toast.Update(notifications.ExpireNow(toast))
	if !strings.Contains(toast.View(), "third") {
		t.Fatalf("expected third visible, got %q", toast.View())
	}
	toast, _ = toast.Update(notifications.ExpireNow(toast))
	if !toast.Empty() {
		t.Fatalf("expected empty after final expire, got %q", toast.View())
	}
}

func TestToast_StaleExpireIgnored(t *testing.T) {
	toast := notifications.NewToast(safeDuration)
	toast, _ = toast.Update(push("first"))
	toast, _ = toast.Update(notifications.ExpireNow(toast)) // #1 popped, queue empty
	toast, _ = toast.Update(push("second"))                 // bumps seq again

	preStale := notifications.StaleExpire(toast) // seq = current-1
	toast, _ = toast.Update(preStale)

	if !strings.Contains(toast.View(), "second") {
		t.Fatalf("stale expire displaced current toast, got %q", toast.View())
	}
}

func TestToast_DefaultDurationUsedWhenZero(t *testing.T) {
	toast := notifications.NewToast(0)
	if got := notifications.ToastDuration(toast); got != notifications.DefaultToastDuration {
		t.Fatalf("expected default %v, got %v", notifications.DefaultToastDuration, got)
	}
}
