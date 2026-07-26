package utils

import "github.com/hexworks/agentfiles/internal/errs"

// Persisted is the constraint every on-disk JSON document type satisfies so
// that ReadJSON/WriteJSON can run migration and validation at the single
// persistence boundary — the caller cannot forget either step.
//
// T is the document struct; P is its pointer type. Both Migrate and Validate
// need pointer receivers: Migrate stamps the version field in place, and
// pinning P to *T lets ReadJSON decode into a fresh value and still invoke
// the pointer-receiver methods. A value-receiver Validate is promoted into
// *T's method set, so a type may keep Validate on the value and only add
// Migrate on the pointer.
type Persisted[T any] interface {
	*T
	// Migrate upgrades an older or legacy in-memory value to the current
	// schema version. A missing version (the zero value 0) is the
	// pre-versioning legacy sentinel and is stamped up to the current
	// version; Migrate never downgrades. It runs before Validate on both
	// read and write.
	Migrate() errs.DomainError
	// Validate checks the domain shape and rejects a version newer than
	// this build understands (returns errs.NewerSchemaVersionError), the
	// forward-compatibility guard against an old binary rewriting a newer
	// file.
	Validate() errs.DomainError
}
