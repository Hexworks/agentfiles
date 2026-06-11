package styles

import (
	"testing"

	"github.com/hexworks/agentfiles/internal/errs"
)

// SeverityLabel is the single source of truth for the short uppercase
// severity vocabulary every severity-aware view shares. Lock the labels
// so an accidental rename in one place does not drift away from the
// rest of the TUI.
func TestSeverityLabel_Vocabulary(t *testing.T) {
	cases := []struct {
		severity errs.Severity
		want     string
	}{
		{errs.SeverityInfo, "INFO"},
		{errs.SeverityWarning, "WARN"},
		{errs.SeverityError, "ERROR"},
	}
	for _, tc := range cases {
		got := SeverityLabel(tc.severity)
		if got != tc.want {
			t.Errorf("SeverityLabel(%v) = %q, want %q", tc.severity, got, tc.want)
		}
	}
}
