package shell

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hexworks/agentfiles/internal/appapi"
	"github.com/hexworks/agentfiles/internal/errs"
	"github.com/hexworks/agentfiles/internal/tui/notifications"
)

// TestMergeCommitText pins the six-case matrix so future outcome
// variants cannot silently drift the composed text (task 0035 review
// issue #10).
func TestMergeCommitText(t *testing.T) {
	skip := appapi.Skipped{Reason: appapi.SkipDisabled}
	failed := appapi.Failed{Err: stubDomainErr{msg: "boom", sev: errs.SeverityWarning}}
	cases := []struct {
		name  string
		sync  appapi.CommitOutcome
		adopt appapi.CommitOutcome
		want  string
	}{
		{"both committed", appapi.Committed{SHA: "syncsha"}, appapi.Committed{SHA: "adopt99"}, "Project synced (committed syncsha; profile adopt99)"},
		{"sync only", appapi.Committed{SHA: "syncsha"}, skip, "Project synced (committed syncsha)"},
		{"adopt only", skip, appapi.Committed{SHA: "adopt99"}, "Project synced (profile adopt99)"},
		{"neither", skip, skip, "Project synced"},
		{"sync failed adopt skipped", failed, skip, "Project synced"},
		{"sync committed adopt failed", appapi.Committed{SHA: "syncsha"}, failed, "Project synced (committed syncsha)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeCommitText("Project synced", tc.sync, tc.adopt); got != tc.want {
				t.Errorf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSyncCommitOutcomeCmd_BatchesWarnsForFailedOutcomes pins that
// the returned tea.Cmd emits one info toast plus a warn for each
// Failed outcome, in sync-then-adopt order.
func TestSyncCommitOutcomeCmd_BatchesWarnsForFailedOutcomes(t *testing.T) {
	failedSync := appapi.Failed{Err: stubDomainErr{msg: "sync boom", sev: errs.SeverityWarning}}
	failedAdopt := appapi.Failed{Err: stubDomainErr{msg: "adopt boom", sev: errs.SeverityWarning}}
	cmd := syncCommitOutcomeCmd("Project synced", failedSync, failedAdopt)
	if cmd == nil {
		t.Fatalf("cmd = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want tea.BatchMsg", msg)
	}
	var infoText string
	var warns []string
	for _, c := range batch {
		if c == nil {
			continue
		}
		note, ok := c().(notifications.NotificationMsg)
		if !ok {
			continue
		}
		if note.Notification.Severity == errs.SeverityInfo {
			infoText = note.Notification.Text
			continue
		}
		warns = append(warns, note.Notification.Text)
	}
	if !strings.Contains(infoText, "Project synced") {
		t.Errorf("info text = %q, want to contain Project synced", infoText)
	}
	if len(warns) != 2 {
		t.Fatalf("warns = %v, want two", warns)
	}
	if warns[0] != "sync boom" || warns[1] != "adopt boom" {
		t.Errorf("warns = %v, want [sync boom adopt boom] (sync-then-adopt order)", warns)
	}
}

// TestSyncCommitOutcomeCmd_NoWarnsProducesSingleInfoCmd pins that the
// happy path returns a plain (non-batched) info cmd so consumers
// unwrapping tea.BatchMsg for the failure case still work.
func TestSyncCommitOutcomeCmd_NoWarnsProducesSingleInfoCmd(t *testing.T) {
	cmd := syncCommitOutcomeCmd("Project synced", appapi.Committed{SHA: "syncsha"}, appapi.Skipped{Reason: appapi.SkipDisabled})
	if cmd == nil {
		t.Fatalf("cmd = nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); ok {
		t.Fatalf("msg is tea.BatchMsg, want single NotificationMsg")
	}
	note, ok := msg.(notifications.NotificationMsg)
	if !ok {
		t.Fatalf("msg = %T, want NotificationMsg", msg)
	}
	if !strings.Contains(note.Notification.Text, "committed syncsha") {
		t.Errorf("info text = %q, want to contain committed syncsha", note.Notification.Text)
	}
}
