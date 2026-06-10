package notifications_test

import (
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

func push(text string) notifications.PushMsg {
	return notifications.PushMsg{Notification: notifications.Notification{
		Level: notifications.LevelInfo,
		Text:  text,
	}}
}

func TestToast_EmptyRendersEmptyString(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	if v := toast.View(); v != "" {
		t.Fatalf("expected empty view, got %q", v)
	}
	if !toast.Empty() {
		t.Fatal("expected Empty() true")
	}
}

func TestToast_FirstPushSchedulesExpire(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)

	next, cmd := toast.Update(push("hello"))
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd for first push")
	}
	if next.Empty() {
		t.Fatal("expected queue non-empty after push")
	}
}

func TestToast_SecondPushDoesNotResetTimer(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	toast, _ = toast.Update(push("first"))

	_, cmd := toast.Update(push("second"))
	if cmd != nil {
		t.Fatalf("expected no new tea.Cmd while a toast is already visible, got %v", cmd)
	}
}

func TestToast_QueueOrderIsFIFO(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	toast, _ = toast.Update(push("first"))
	toast, _ = toast.Update(push("second"))
	toast, _ = toast.Update(push("third"))

	// Initial view = first.
	if !containsText(toast.View(), "first") {
		t.Fatalf("expected first visible, got %q", toast.View())
	}
	// Expire first → second.
	toast, _ = toast.Update(toast.ExpireNowForTest())
	if !containsText(toast.View(), "second") {
		t.Fatalf("expected second visible, got %q", toast.View())
	}
	// Expire second → third.
	toast, _ = toast.Update(toast.ExpireNowForTest())
	if !containsText(toast.View(), "third") {
		t.Fatalf("expected third visible, got %q", toast.View())
	}
	// Expire third → empty.
	toast, _ = toast.Update(toast.ExpireNowForTest())
	if !toast.Empty() {
		t.Fatalf("expected empty after final expire, got %q", toast.View())
	}
}

func TestToast_StaleExpireIgnored(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	toast, _ = toast.Update(push("first"))
	stale := toast.StaleExpireForTest() // seq for #1 -1; older still
	_ = stale
	// Properly: expire #1 to advance, then push #2 and replay a stale
	// expireMsg generated *before* push #1 — must not displace #2.
	toast, _ = toast.Update(toast.ExpireNowForTest()) // #1 popped, queue empty
	toast, _ = toast.Update(push("second"))           // bumps seq again

	preStale := toast.StaleExpireForTest() // seq = current-1
	toast, _ = toast.Update(preStale)

	if !containsText(toast.View(), "second") {
		t.Fatalf("stale expire displaced current toast, got %q", toast.View())
	}
}

func TestToast_DefaultDurationUsedWhenZero(t *testing.T) {
	toast := notifications.NewToast(0)
	if toast.Duration() != notifications.DefaultToastDuration {
		t.Fatalf("expected default %v, got %v", notifications.DefaultToastDuration, toast.Duration())
	}
}

func containsText(rendered, needle string) bool {
	for i := 0; i+len(needle) <= len(rendered); i++ {
		if rendered[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
