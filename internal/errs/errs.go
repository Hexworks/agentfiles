// Package errs declares the cross-package error vocabulary used by the
// agentfiles domain layer. It intentionally exposes only types and
// constants — no helpers that depend on other internal packages — so it
// stays at the bottom of the import graph and never introduces a cycle.
package errs

// Severity classifies a domain failure. It is part of the domain because
// "is this a hard error or a recoverable warning" is a domain question
// about the kind of failure, not a presentation concern. The TUI consumes
// severity directly via DomainError.Severity() instead of dispatching on
// the concrete type.
type Severity int

const (
	// SeverityInfo marks an informational outcome. Reserved; no current
	// typed error returns it.
	SeverityInfo Severity = iota
	// SeverityWarning marks a recoverable condition that does not prevent
	// the user from continuing (e.g. a path is owned elsewhere — pick a
	// different one).
	SeverityWarning
	// SeverityError marks a hard failure: the operation cannot proceed
	// without resolving the cause.
	SeverityError
)

// DomainError is the contract every typed domain error implements. It is
// a regular error plus a severity classification.
type DomainError interface {
	error
	Severity() Severity
}

// Collect flattens an error value into its constituent DomainErrors.
// errors.Join values (Unwrap() []error) are walked recursively; single-
// wrapper values (Unwrap() error) are descended too. Non-DomainError
// leaves are dropped — callers that need them must inspect the original
// error value directly.
func Collect(err error) []DomainError {
	if err == nil {
		return nil
	}
	var out []DomainError
	collect(err, &out)
	return out
}

func collect(err error, out *[]DomainError) {
	if err == nil {
		return
	}
	type joined interface{ Unwrap() []error }
	if j, ok := err.(joined); ok {
		for _, e := range j.Unwrap() {
			collect(e, out)
		}
		return
	}
	if d, ok := err.(DomainError); ok {
		*out = append(*out, d)
		return
	}
	type wrapper interface{ Unwrap() error }
	if w, ok := err.(wrapper); ok {
		collect(w.Unwrap(), out)
	}
}

// Errors is a slice of DomainError values that itself satisfies the
// DomainError interface. It is the return shape for accumulator
// functions that want to expose every issue at once while still
// handing a single DomainError-typed value to callers that propagate
// errors through the standard `error` plumbing.
type Errors []DomainError

// Error joins each underlying message with a newline so the value still
// renders sensibly when printed without TUI introspection.
func (es Errors) Error() string {
	if len(es) == 0 {
		return ""
	}
	if len(es) == 1 {
		return es[0].Error()
	}
	out := es[0].Error()
	for _, e := range es[1:] {
		out += "\n" + e.Error()
	}
	return out
}

// Severity returns the highest severity present in the slice. An empty
// slice degrades to SeverityInfo because it represents "no failure".
// This makes Errors itself a DomainError so wrappers can keep returning
// a single domain value even when several leaves are involved.
func (es Errors) Severity() Severity {
	highest := SeverityInfo
	for _, e := range es {
		if s := e.Severity(); s > highest {
			highest = s
		}
	}
	return highest
}

// Unwrap exposes the underlying domain errors so errors.As / errors.Is
// can walk the slice the same way they walk a value produced by
// errors.Join.
func (es Errors) Unwrap() []error {
	out := make([]error, len(es))
	for i, e := range es {
		out[i] = e
	}
	return out
}
