package styles

import (
	"strconv"
	"strings"
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

// TestSafe_HostileRunesForceQuotedLiteral — any rune that can hijack the
// terminal (C0 controls, DEL, C1 controls, bidi overrides, zero-width
// formatters) must force the whole string into strconv.Quote form so the
// user sees the escaped literal, never the rendered control sequence.
// Expected is strconv.Quote(input), not a hand-copied literal, so the
// test cannot drift from the impl.
func TestSafe_HostileRunesForceQuotedLiteral(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"raw ESC", "~/re\x1bpo"},
		{"NUL", "a\x00b"},
		{"DEL", "a\x7fb"},
		{"bidi isolate", "a\u2066b"},
		{"BOM", "a\ufeffb"},
		{"C1 control", "ab"},
		{"bidi override", "a‮b"},
		{"zero-width formatter", "a​b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Safe(tc.input)
			if got == tc.input {
				t.Fatalf("Safe(%q) returned input unchanged; hostile rune must be quoted", tc.input)
			}
			if want := strconv.Quote(tc.input); got != want {
				t.Errorf("Safe(%q) = %q, want %q", tc.input, got, want)
			}
		})
	}
}

// TestSafe_CleanInputUnchanged — a mixed ASCII/UTF-8 path with no hostile
// runes is returned verbatim (no quoting, no stripping).
func TestSafe_CleanInputUnchanged(t *testing.T) {
	const path = "~/repos/münchen_project"
	if got := Safe(path); got != path {
		t.Errorf("Safe(%q) = %q, want unchanged", path, got)
	}
}

// TestSafe_PreservesTabAndNewline — `\t` and `\n` are in Safe's allow
// switch, so an otherwise-safe string keeps them and is returned
// unchanged (accepted residual, documented on pathDisplayNote).
func TestSafe_PreservesTabAndNewline(t *testing.T) {
	const s = "line1\tcol2\nline2"
	got := Safe(s)
	if got != s {
		t.Fatalf("Safe(%q) = %q, want unchanged", s, got)
	}
	if !strings.Contains(got, "\t") || !strings.Contains(got, "\n") {
		t.Errorf("Safe(%q) dropped a tab/newline: %q", s, got)
	}
}

// TestSafe_EmptyStringReturnsEmpty guards the s == "" fast path.
func TestSafe_EmptyStringReturnsEmpty(t *testing.T) {
	if got := Safe(""); got != "" {
		t.Errorf("Safe(\"\") = %q, want empty", got)
	}
}
