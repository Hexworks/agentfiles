package utils

import "github.com/hexworks/agentfiles/internal/errs"

// Persisted is the constraint every on-disk JSON document type satisfies so
// that ReadJSON/WriteJSON can run migration, the version guard, and domain
// validation at the single persistence boundary — the caller cannot forget
// any of them.
//
// T is the document struct; P is its pointer type. The methods take pointer
// receivers: Migrate stamps the version field in place, and pinning P to *T
// lets ReadJSON decode into a fresh value and still invoke them.
type Persisted[T any] interface {
	*T
	// Migrate upgrades an older or legacy in-memory value to the current
	// schema version. A missing version (the zero value 0) is the
	// pre-versioning legacy sentinel and is stamped up to the current
	// version; Migrate never downgrades. It runs first on both read and
	// write.
	Migrate() errs.DomainError
	// SchemaVersion reports the value's own schema version (have) and the
	// highest version this build understands (known). The boundary runs the
	// reject-newer forward-compatibility guard once from this pair (returning
	// errs.NewerSchemaVersionError when have > known), so no per-type Validate
	// repeats it. Called after Migrate, so a stamped legacy value reports the
	// current version.
	SchemaVersion() (have, known int)
	// Validate checks the domain shape of the value — its type-specific
	// invariants only. The version guard is owned by the boundary (see
	// SchemaVersion), so Validate is a no-op for types whose on-disk shape is
	// just a version envelope.
	Validate() errs.DomainError
}
