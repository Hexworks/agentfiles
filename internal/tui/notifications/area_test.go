package notifications_test

import (
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

func TestArea_DelegatesPushToToast(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	area := notifications.NewArea(toast)

	area, _ = area.Update(notifications.PushMsg{Notification: notifications.Notification{
		Level: notifications.LevelInfo,
		Text:  "hi",
	}})

	if area.Empty() {
		t.Fatal("expected area non-empty after push")
	}
	if area.View() == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestArea_EmptyAfterExpire(t *testing.T) {
	toast := notifications.NewToast(time.Millisecond)
	area := notifications.NewArea(toast)
	area, _ = area.Update(notifications.PushMsg{Notification: notifications.Notification{
		Level: notifications.LevelInfo,
		Text:  "hi",
	}})

	area, _ = area.Update(toast.ExpireNowForTest())

	if !area.Empty() {
		t.Fatalf("expected empty after expire, got %q", area.View())
	}
	if area.View() != "" {
		t.Fatalf("expected empty view, got %q", area.View())
	}
}

func TestNewArea_NilToastPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil toast")
		}
	}()
	_ = notifications.NewArea(nil)
}
