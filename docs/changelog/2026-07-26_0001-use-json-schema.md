# 0001 changes

Turned `utils.ReadJSON` and the three write helpers into **type-parameterised**
functions constrained to a new `utils.Persisted[T]` interface, so every load
and save of an on-disk JSON document runs `Migrate()` then `Validate()` at a
single persistence boundary. Validation and legacy-version migration can no
longer be forgotten by a caller — a type that does not implement both methods
is a compile error at the call site.

Research concluded JSON Schema is **not** adopted: it validates shape at one
point in time and does not provide schema evolution (migration is always
application code), and the schemas never leave the `af` process, so a
JSON-Schema library would earn no dependency. The idiomatic Go path — stdlib
`encoding/json` + generics + a `version` envelope + app-owned
`Migrate()`/`Validate()` at one boundary — was implemented instead. `go.mod`
is unchanged.

Every persisted type now carries a schema `version` and implements
`Persisted`: `registry.Registry`, `projectstore.State`, `settings.Settings`,
`profile.Manifest`, `asset.Manifest` (version field newly added), and
`sync.ManagedState` (integer `version` added **alongside** its existing
`generator_version`, decision C). A missing `version` (0) is a legacy sentinel
migrated up; a version newer than the build knows is rejected with the shared
`errs.NewerSchemaVersionError`, filled in with the file path at the boundary.

## Decisions

- **Shared reject-newer error lives in `internal/errs`** (`NewerSchemaVersionError`) — **Why:** every persisted type's `Validate()` returns it and `errs` already sits at the bottom of the import graph as the common error vocabulary; one definition avoids six copies.
- **Path enrichment happens at the boundary, not in `Validate()`** — **Why:** a decoded value does not know which file it came from. `ReadJSON`/`WriteJSON*` set `NewerSchemaVersionError.Path` via a small `withPath` helper so the message is precise while `Validate()` stays path-agnostic.
- **Write validates before any filesystem side effect** — **Why:** an invalid value must never reach disk. `prepareJSON` runs migrate→validate→marshal before `EnsureDirMode`/temp-file creation, so no partial file, no created directory, and no `.tmp-*` residue on rejection.
- **Value-copy write semantics** (`WriteJSON*` take `v T` by value) — **Why:** `Migrate()` stamping the version must not mutate the caller's value.
- **`state.json` keeps both `generator_version` and a new integer `version`** (decision C) — **Why:** they mean different things (entry payload format vs. schema envelope); overloading or renaming would break existing files and conflate concerns.
- Rejected: an opt-in `ReadJSONValidated` alongside a plain `ReadJSON[T any]` — keeps the "forgot to validate" foot-gun, which the task exists to remove.

## Assumptions

- **Go constraint type inference resolves `P = *T`** so `ReadJSON[Manifest](path)` and `WriteJSON(path, value)` compile without the explicit pointer type argument — **Why:** the `Persisted[T]` constraint embeds core type `*T`; verified at implementation by a green build across all six persisted types and their call sites (fallback `ReadJSON[T, *T]` was not needed).
- **Plan step 7 misidentified `migrate.go:302` as reading `asset.Manifest`; it actually reads `project.Manifest`** — **Why (deviation):** `project.Manifest` is persisted **nested** inside `projectstore.State`, not as its own versioned document, and the plan deliberately keeps it out of `Persisted`. Forcing it to implement `Persisted` would have required adding a `version` field to it (out of scope, contradicts the plan). Resolved by reading those v1-layout standalone manifest files raw via `encoding/json` — the same escape-hatch precedent already used by `readV1Registry` in the same package — with the existing `Normalize()`/`Validate()` calls left in place. `migrate_test.go`'s v1 seed helper was updated to write those fixtures raw for the same reason.

## Other Notes

- **New ADR:** `docs/adr/0022-validated-versioned-persistence-boundary.md`.
- **New guideline section:** `docs/guidelines/go.md` → "Persist Only Validated, Versioned Data".
- **Glossary:** added "Persistence Boundary", "Schema Version", "Legacy Sentinel" to `docs/glossary.md`.
- **CLAUDE.md:** `utils` bullet now describes the validated/versioned JSON boundary and the `Persisted` constraint.
- **Tests (+18):** boundary round-trip / legacy-sentinel / reject-newer / malformed / write-refuses-invalid / atomic-no-residue in `internal/utils/persist_test.go`; real-file legacy + reject-newer tests per persisted type in `asset`, `registry`, `projectstore`, `profile`, `settings`, and `sync` (`loadState` preserves `generator_version`).
- The redundant standalone `manifest.Validate()` calls in `asset.Load` and `asset.SaveManifest` were removed — validation now runs inside `ReadJSON`/`WriteJSON`. The pre-flight `Validate()` in `scaffold`/`InitFromFolder` was left in place (it gates before directory creation, a distinct concern from load-time validation).
- Redundant manual `Version` stamping removed from `registry.Save`/`Load` and `settings.Load`/`Save`; `Migrate` in the pipeline is now the single source of stamping.

## Generic, validated `ReadJSON`/`WriteJSON`

```go
// before
func ReadJSON(path string, v any) errs.DomainError {
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadJSONError{Path: path, Err: err}
	}
	if err := json.Unmarshal(data, v); err != nil {
		return ReadJSONError{Path: path, Err: err}
	}
	return nil
}
```

```go
// after — decode → Migrate → Validate; caller cannot skip either step
func ReadJSON[T any, P Persisted[T]](path string) (T, errs.DomainError) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, ReadJSONError{Path: path, Err: err}
	}
	if err := json.Unmarshal(data, P(&v)); err != nil {
		return v, ReadJSONError{Path: path, Err: err}
	}
	if mErr := P(&v).Migrate(); mErr != nil {
		return v, withPath(mErr, path)
	}
	if vErr := P(&v).Validate(); vErr != nil {
		return v, withPath(vErr, path)
	}
	return v, nil
}
```

## The `Persisted` constraint

```go
// after — new file internal/utils/persist.go
type Persisted[T any] interface {
	*T
	Migrate() errs.DomainError  // stamps legacy version 0 up to current
	Validate() errs.DomainError // domain shape + reject-newer guard
}
```

## Per-type Migrate/Validate (asset.Manifest shown)

```go
// before — value receiver Validate, no version field
func (m Manifest) Validate() errs.DomainError {
	if m.ID == "" || m.Name == "" {
		return ErrAssetIDNameRequired
	}
	// ...
}
```

```go
// after — version field + Migrate + reject-newer in Validate
const Version = 1

func (m *Manifest) Migrate() errs.DomainError {
	if m.Version == 0 {
		m.Version = Version
	}
	return nil
}

func (m Manifest) Validate() errs.DomainError {
	if m.Version > Version {
		return errs.NewerSchemaVersionError{Have: m.Version, Known: Version}
	}
	if m.ID == "" || m.Name == "" {
		return ErrAssetIDNameRequired
	}
	// ...
}
```
