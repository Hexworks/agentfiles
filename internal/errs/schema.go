package errs

import "fmt"

// NewerSchemaVersionError reports that a persisted document on disk was
// written by a newer build of af than the one now reading it: its schema
// version is greater than the highest version this binary understands.
//
// It is the forward-compatibility guard returned by every persisted type's
// Validate(): an older binary refuses to load — and therefore cannot
// silently mangle — a file whose shape it does not fully understand. The
// path is filled in at the persistence boundary (utils.ReadJSON /
// WriteJSON), so a value's Validate() can report Have/Known without knowing
// which file it came from.
//
// It lives in errs (rather than any one domain package) because every
// persisted type across the domain returns it, and errs already sits at the
// bottom of the import graph as the shared error vocabulary.
type NewerSchemaVersionError struct {
	Path  string
	Have  int
	Known int
}

func (e NewerSchemaVersionError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf(
			"document was written by a newer version of af (schema v%d, this build knows v%d)",
			e.Have, e.Known)
	}
	return fmt.Sprintf(
		"%s was written by a newer version of af (schema v%d, this build knows v%d)",
		e.Path, e.Have, e.Known)
}

func (NewerSchemaVersionError) Severity() Severity {
	return SeverityError
}

// WithPath returns the error with Path filled in, but only when it was
// created without one — a value's Validate() reports Have/Known without
// knowing which file it came from. An error already carrying a path is
// returned unchanged. Satisfies PathSettable so the persistence boundary can
// enrich it generically.
func (e NewerSchemaVersionError) WithPath(path string) DomainError {
	if e.Path == "" {
		e.Path = path
	}
	return e
}

// PathSettable is implemented by boundary errors created without a file path —
// the value being validated does not know which file it was decoded from. The
// persistence boundary (utils.ReadJSON/WriteJSON) injects the path via WithPath
// so the message can name the offending file. Implementations must only set the
// path when they were created without one.
type PathSettable interface {
	DomainError
	WithPath(path string) DomainError
}
