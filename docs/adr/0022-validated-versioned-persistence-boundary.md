# Validated, Versioned Persistence Boundary

## Status

accepted

## Context

Every on-disk document `af` owns — the profile registry, the project
store, user settings, each profile manifest, each asset manifest, and each
target repo's managed state — is loaded through `utils.ReadJSON` and saved
through `utils.WriteJSON` / `WriteJSONMode` / `WriteJSONAtomic`. Before this
decision those functions were `func(path string, v any)`: a bare
`encoding/json` round-trip with **no** validation. Validation, where it
existed, was a separate `Validate()` call each caller had to remember.
`asset.Manifest` and `project.Manifest` had it wired into their load path;
`registry.Registry`, `projectstore.State`, `settings.Settings`,
`profile.Manifest`, and `sync.ManagedState` did not. A malformed or
hand-edited file could load silently and fail somewhere downstream.

Two questions were researched:

1. **Do we need JSON Schema?** No. JSON Schema validates a document's shape
   at one point in time; it does **not** provide schema evolution —
   migration between versions is always application-owned code. The schemas
   never leave the `af` process (no external tooling, no editor validation
   of hand-edited files, no cross-language contract), so a JSON-Schema
   library (`santhosh-tekuri/jsonschema` + `invopop/jsonschema`) would earn
   its dependency only if the contract had to leave Go. It does not.

2. **What is the idiomatic Go path?** The one the codebase already
   half-used: stdlib `encoding/json` + generics + a `version` envelope on
   every persisted type + an app-owned `Migrate()` + `Validate()`, all
   enforced at a single load/save boundary so neither step can be
   forgotten.

## Decision

`utils.ReadJSON` and the three write variants are **type-parameterised**
over a `utils.Persisted[T]` constraint:

```go
type Persisted[T any] interface {
	*T
	Migrate() errs.DomainError
	SchemaVersion() (have, known int)
	Validate() errs.DomainError
}
```

The pipeline is identical on both sides: **decode/prepare → `Migrate()` →
reject-newer (`SchemaVersion()`) → `Validate()` → (write only) marshal +
persist.** Because `T` is constrained to `Persisted`, a caller cannot read or
write a persisted document without migration, the version guard, and
validation running. A type that does not implement all three methods is a
**compile error** at the call site — validation cannot be bypassed.

The version guard is a single cross-cutting concern, so it lives in the
boundary rather than being fanned out into every type's `Validate()`: the
boundary compares the `(have, known)` pair `SchemaVersion()` reports and
returns `errs.NewerSchemaVersionError` when `have > known`. `Validate()` is
then free to check only domain shape — for envelope-only types
(`registry.Registry`, `profile.Manifest`, `settings.Settings`) it is a
no-op; `asset.Manifest` checks id/name/type, `projectstore.State` fans out to
each nested `project.Manifest.Validate()`, and `sync.ManagedState` enforces
its managed-key path safety.

Rules baked into the boundary:

- **Legacy sentinel.** A missing `version` key decodes to `0`. `Migrate()`
  treats `0` as the pre-versioning legacy value and stamps it up to the
  type's current `Version`. Existing files keep loading; they gain the
  version on next save.
- **Reject newer.** The boundary reads the `(have, known)` pair from
  `SchemaVersion()` and returns `errs.NewerSchemaVersionError` when `have >
  known` — the document's version is greater than the `Version` this build
  knows. An old binary refuses to load — and therefore cannot mangle — a
  file a newer binary wrote. The boundary fills the file path into the error
  (a value does not know which file it came from); a path-less boundary error
  that implements `errs.PathSettable` (e.g. `sync.StateCorruptError`) is
  enriched the same way.
- **Migrate before Validate on write too**, so a freshly-constructed
  `version:0` value is stamped current before the reject-newer check.
- **Validate before any filesystem side effect on write.** An invalid value
  is never persisted, the target directory is not even created, and
  `WriteJSONAtomic` leaves no `.tmp-*` residue.
- **Value-copy write semantics.** The write functions take `v T` by value,
  so version stamping never mutates the caller's value.

### `state.json` version representation (decision C)

`sync.ManagedState` already carried `generator_version` (a semver string
tracking the `ManagedFileEntry` payload format, see ADR 0020). Rather than
overload or rename it, `ManagedState` gains a separate integer `version`
(`const SchemaVersion = 1`) **alongside** `generator_version`. The two mean
different things: `generator_version` is the entry-format marker;
`version` is the schema envelope the persistence boundary migrates and
validates. No existing file is broken; legacy state files (no `version`)
migrate up while `generator_version` is preserved untouched.

### Escape hatch for pre-versioning DTOs

`migrate` reads two legacy shapes raw via `encoding/json` rather than
through the generic boundary: the v1 registry (`readV1Registry`) and the
v1-layout standalone project manifest files (`harvestProjectsDir`). These
are transient migration-internal reads of pre-versioning data;
`project.Manifest` is persisted **nested** inside `projectstore.State`, not
as its own versioned document, so it is deliberately **not** a `Persisted`
type. The migrate reads validate through the explicit `Normalize()` /
`Validate()` calls that already follow them.

## Consequences

- One boundary owns migration + validation; the "forgot to validate"
  foot-gun is gone, enforced by the compiler.
- Every persisted document now carries a `version`, giving a defined seam
  (`Migrate()`) for future schema bumps even though today's only migration
  is the v0→v1 legacy-sentinel stamp.
- Forward-compatibility: a downgraded binary fails loudly instead of
  silently rewriting a newer file into an older shape.
- No new dependency; `go.mod` is unchanged.
- Follow-up work (real multi-version migrations) builds on the `Migrate()`
  seam without touching the boundary again.

## Alternatives Considered

- **JSON Schema library.** Rejected: no schema evolution, and the contract
  never leaves the process — a dependency with no payoff here.
- **Opt-in `ReadJSONValidated` alongside a plain `ReadJSON[T any]`.**
  Rejected: keeps the forget-to-validate foot-gun. The whole point is that
  validation cannot be skipped.
- **Normalize every type to a single `version int`, renaming
  `generator_version`.** Rejected: breaks existing `state.json` files and
  conflates two distinct concerns. Decision C keeps both.
