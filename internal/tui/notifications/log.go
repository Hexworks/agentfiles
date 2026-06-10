// Package notifications hosts the in-memory notification ring buffer,
// the FIFO toast queue, and the action-to-notification bridge helper
// used by every TUI screen.
package notifications

import (
	"sync"
	"time"

	"github.com/hexworks/agentfiles/internal/errs"
)

// Notification is a single log entry and toast payload. Severity uses
// the canonical errs.Severity vocabulary so downstream consumers (log
// view, notifications modal) can re-style without inventing a parallel
// taxonomy.
type Notification struct {
	Severity  errs.Severity
	Text      string
	CreatedAt time.Time
}

// LogCap is the maximum number of entries retained in the ring buffer.
// New entries past the cap evict the oldest. 500 is sized for one busy
// TUI session worth of plan/apply/CRUD operations plus headroom; tuneable
// without changing behavior.
const LogCap = 500

// Log is a fixed-capacity ring buffer of Notification values, in-memory
// only. It is safe for concurrent Add/Entries because the bridge
// command may run on a goroutine other than the View loop.
type Log struct {
	mu      sync.Mutex
	entries []Notification
	next    int
	full    bool
}

// NewLog constructs an empty Log.
func NewLog() *Log {
	return &Log{entries: make([]Notification, 0, LogCap)}
}

// Add appends n. When the buffer is full the oldest entry is evicted
// in place.
func (l *Log) Add(n Notification) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.full {
		l.entries = append(l.entries, n)
		if len(l.entries) == LogCap {
			l.full = true
			l.next = 0
		}
		return
	}
	l.entries[l.next] = n
	l.next = (l.next + 1) % LogCap
}

// Entries returns the entries newest-first. The result is a defensive
// copy, so callers may sort or filter without locking.
func (l *Log) Entries() []Notification {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(l.entries)
	out := make([]Notification, n)
	if !l.full {
		for i := range n {
			out[i] = l.entries[n-1-i]
		}
		return out
	}
	for i := range n {
		idx := (l.next - 1 - i + LogCap) % LogCap
		out[i] = l.entries[idx]
	}
	return out
}
