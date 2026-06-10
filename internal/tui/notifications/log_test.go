package notifications_test

import (
	"testing"
	"time"

	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

func mkNotif(i int) notifications.Notification {
	return notifications.Notification{
		Level:     notifications.LevelInfo,
		Text:      "n" + itoa(i),
		CreatedAt: time.Unix(int64(i), 0),
	}
}

// itoa avoids strconv import just for tiny test labels.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestLog_NewLogEmpty(t *testing.T) {
	log := notifications.NewLog()
	if got := log.Entries(); len(got) != 0 {
		t.Fatalf("expected empty log, got %d entries", len(got))
	}
}

func TestLog_AddBelowCapReturnsNewestFirst(t *testing.T) {
	log := notifications.NewLog()
	for i := 0; i < 3; i++ {
		log.Add(mkNotif(i))
	}

	entries := log.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Text != "n2" || entries[2].Text != "n0" {
		t.Fatalf("expected newest first, got %v / %v", entries[0].Text, entries[2].Text)
	}
}

func TestLog_AddAtCapDropsOldest(t *testing.T) {
	log := notifications.NewLog()
	for i := 0; i < notifications.LogCap; i++ {
		log.Add(mkNotif(i))
	}
	log.Add(mkNotif(notifications.LogCap)) // 501st entry

	entries := log.Entries()
	if len(entries) != notifications.LogCap {
		t.Fatalf("expected %d entries, got %d", notifications.LogCap, len(entries))
	}
	if entries[0].Text != "n"+itoa(notifications.LogCap) {
		t.Fatalf("expected newest text n%d, got %q", notifications.LogCap, entries[0].Text)
	}
	if entries[len(entries)-1].Text != "n1" {
		t.Fatalf("expected oldest survivor n1, got %q", entries[len(entries)-1].Text)
	}
}

func TestLog_AddPastCapEvictionOrder(t *testing.T) {
	log := notifications.NewLog()
	const extra = 50
	for i := 0; i < notifications.LogCap+extra; i++ {
		log.Add(mkNotif(i))
	}

	entries := log.Entries()
	if len(entries) != notifications.LogCap {
		t.Fatalf("expected cap %d entries, got %d", notifications.LogCap, len(entries))
	}
	// Newest = LogCap+extra-1; oldest = extra (first `extra` entries evicted).
	if want, got := "n"+itoa(notifications.LogCap+extra-1), entries[0].Text; want != got {
		t.Fatalf("newest: want %q got %q", want, got)
	}
	if want, got := "n"+itoa(extra), entries[len(entries)-1].Text; want != got {
		t.Fatalf("oldest: want %q got %q", want, got)
	}
}

func TestLog_EntriesIsCopySafe(t *testing.T) {
	log := notifications.NewLog()
	log.Add(mkNotif(0))

	entries := log.Entries()
	entries[0].Text = "mutated"

	again := log.Entries()
	if again[0].Text != "n0" {
		t.Fatalf("Entries() mutation leaked: %q", again[0].Text)
	}
}
