package errs

import (
	"errors"
	"fmt"
	"testing"
)

type fakeErr struct {
	msg string
	sev Severity
}

func (f fakeErr) Error() string      { return f.msg }
func (f fakeErr) Severity() Severity { return f.sev }

func TestCollect_FlattensJoinedErrors(t *testing.T) {
	a := fakeErr{msg: "a", sev: SeverityError}
	b := fakeErr{msg: "b", sev: SeverityWarning}
	joined := errors.Join(a, b)

	got := Collect(joined)

	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Severity() != SeverityError || got[1].Severity() != SeverityWarning {
		t.Fatalf("unexpected severities: %+v", got)
	}
}

func TestCollect_DescendsSingleUnwrap(t *testing.T) {
	leaf := fakeErr{msg: "leaf", sev: SeverityError}
	wrapped := fmt.Errorf("outer: %w", leaf)

	got := Collect(wrapped)

	if len(got) != 1 {
		t.Fatalf("expected 1 leaf, got %d", len(got))
	}
	if got[0].Severity() != SeverityError {
		t.Fatalf("expected severity preserved")
	}
}

func TestCollect_DropsNonDomainLeaves(t *testing.T) {
	got := Collect(errors.New("plain"))
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestErrors_ImplementsErrorAndUnwrap(t *testing.T) {
	es := Errors{
		fakeErr{msg: "a", sev: SeverityError},
		fakeErr{msg: "b", sev: SeverityWarning},
	}

	if got := es.Error(); got != "a\nb" {
		t.Fatalf("unexpected joined message: %q", got)
	}
	if len(es.Unwrap()) != 2 {
		t.Fatalf("expected 2 unwrapped, got %d", len(es.Unwrap()))
	}
}
