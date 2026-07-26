# Use Json Schema review

The implementation is a careful, well-tested refactor. The DoD gate passed (all
acceptance criteria met; `make build && make test && make lint` green; no scope
creep). Seven parallel review agents (security, clean code, clean architecture,
SOLID, DDD, testing, Go) confirmed the core design is sound: the `Persisted[T]`
constraint keeps `utils` at the bottom of the import graph, validation is
compiler-enforced, the value-copy write pipeline and `WriteJSONAtomic` cleanup
are correct, and the `migrate.go` escape hatch is justified and documented.

No blocking defects were found. The issues below are refinements: one real
(if minor) file-permission gap, two contract/documentation-accuracy mismatches
between the boundary's promise and what it delivers, one genuine test-coverage
gap on the write side of decision C, two doc-drift fixes, and two style
uniformity nits. Pick one checkbox per issue.

## `registry.Save` cannot re-tighten permissions on an already-permissive `profiles.json`

> [!WARNING]
>
> - [security.md](../../../docs/guidelines/security.md) — "Protect the user's local configuration and credentials"

`registry.Store.Save` (`internal/registry/registry.go:124`) persists through the
**non-atomic** `WriteJSONMode` with `0o600`. But `WriteJSONMode`
(`internal/utils/fs.go:113`) writes via `os.WriteFile(path, data, fileMode)`,
and `os.WriteFile` applies the mode only when _creating_ the file — an existing
file's mode is left untouched. So a `profiles.json` that already exists at
`0o644` (written by an older `af`, or created by another tool) stays
world-readable after a rewrite even though the caller asked for `0o600`. The
file holds absolute profile paths (machine-identifying data). The two sibling
stores (`projectstore`, `settings`) avoid this because they use
`WriteJSONAtomic`, which does an explicit `tmp.Chmod(fileMode)`
(`internal/utils/fs.go:177`). This behavior is unchanged from `master` — the
generic refactor preserved it exactly — so it is not a regression, but this task
is the natural place to close it since it touches every write path.

```go
// internal/utils/fs.go — WriteJSONMode
if err := os.WriteFile(path, data, fileMode); err != nil { // mode ignored when path already exists
    return WriteJSONError{Path: path, Err: err}
}
```

Choose one:

- [x] Route `registry.Store.Save` through `WriteJSONAtomic` (matching
      `projectstore`/`settings`), which chmods the temp file explicitly and also
      gives the registry the same crash-atomicity the other two user-config files
      already have.
- [ ] Add an explicit `os.Chmod(path, fileMode)` after the write in
      `WriteJSONMode` so the requested mode is enforced on existing files too.
- [ ] Accept as-is (out of scope: pre-existing, not introduced by 0001) and file
      a follow-up.

## The persistence boundary's "loaded ⇒ safe" promise does not hold for `sync.ManagedState`

> [!WARNING]
>
> - [sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md) — managed-path safety
> - [solid.md](../../../docs/guidelines/solid.md) — Liskov Substitution

The whole selling point of the boundary (`internal/utils/persist.go:5-7`) is that
a value obtained through `ReadJSON` has been migrated **and** validated.
`asset.Manifest.Validate` honors this by checking domain shape. But
`ManagedState.Validate` (`internal/sync/sync.go:129-134`) checks _only_ the
version; the security-critical invariants — slash-key path safety, `..` escape
rejection, half-populated v3 provenance — live in `loadState`
(`internal/sync/sync.go:896-925`), which runs _after_ `ReadJSON` returns. So a
`ManagedState` obtained through `ReadJSON[ManagedState]` is **not** fully
validated, unlike every other persisted type. A future caller that trusts
"ReadJSON ⇒ safe" gets an under-validated value for the one type whose payload
can drive writes/deletes inside the target repo.

```go
// internal/sync/sync.go
func (s *ManagedState) Validate() errs.DomainError {
    if s.Version > SchemaVersion {
        return errs.NewerSchemaVersionError{Have: s.Version, Known: SchemaVersion}
    }
    return nil // path-key safety enforced later in loadState, NOT here
}
```

The code comment rationalizes the split ("loadState needs the file path for its
corruption errors"), but `withPath` (`internal/utils/fs.go:141`) already exists
precisely to inject the path into a path-less boundary error, so that reason does
not fully hold. Defensible today because `loadState` is the only sanctioned
reader — but the boundary contract is silently weaker for this type.

Choose one:

- [x] Move the path-key / provenance safety checks from `loadState` into
      `ManagedState.Validate` (returning a path-less `StateCorruptError` that
      `withPath` enriches at the boundary), so "loaded ⇒ safe" holds uniformly.
- [ ] Keep the split but document on `Persisted.Validate` (and in ADR 0022) that
      it is not the complete safety gate for every type, naming `ManagedState` as
      the explicit exception — making the weaker contract visible instead of
      silent.
- [ ] Accept as-is: `loadState` is the sole reader and already enforces the
      checks; no other code path can obtain an unvalidated `ManagedState`.

## `Persisted.Validate`'s documented "domain shape" contract is met by only one of six types, and the version guard is copy-pasted six times

> [!WARNING]
>
> - [solid.md](../../../docs/guidelines/solid.md) — Single Responsibility / Liskov
> - [go.md](../../../docs/guidelines/go.md) — "Persist Only Validated, Versioned Data"

`internal/utils/persist.go:23-27` documents `Validate` as "checks the domain
shape **and** rejects a version newer than this build understands." Only
`asset.Manifest.Validate` actually checks domain shape (id/name/type). The other
five (`profile.go:46`, `registry.go:57`, `projectstore/store.go:51`,
`settings.go:54`, `sync.go:129`) check _only_ the version and return `nil` for any
shape. Separately, the identical reject-newer guard
(`if x.Version > Version { return errs.NewerSchemaVersionError{...} }`) is a
single cross-cutting persistence-boundary responsibility that has been fanned out
verbatim into all six `Validate` methods. Both observations point at the same
root cause and the same fix.

```go
// internal/registry/registry.go — a "Validate" that validates no domain shape,
// only the envelope version the boundary already owns
func (r *Registry) Validate() errs.DomainError {
    if r.Version > Version { // <- identical guard duplicated in 6 files
        return errs.NewerSchemaVersionError{Have: r.Version, Known: Version}
    }
    return nil // ProfileRefs never shape-checked here
}
```

Choose one:

- [x] Hoist the version guard into the boundary: add a `SchemaVersion() (have,
    known int)` method to `Persisted[T]`, let `ReadJSON`/`prepareJSON` run the
      reject-newer check once, and narrow each `Validate` to domain-shape-only.
      Removes the six-fold duplication and makes the guard uniform by
      construction.
- [ ] Keep per-type `Validate` but narrow the `persist.go` docstring to the
      contract actually guaranteed: "rejects a version newer than this build
      understands; **may** additionally check domain shape." Optionally extract a
      shared `errs.RejectNewer(have, known)` helper to kill the duplicated literal
      without moving the check.
- [ ] Accept as-is: each type's shape rules genuinely differ, and
      registry/projectstore enforce shape on mutation elsewhere; only the docstring
      over-promises.

## Decision C's `generator_version` preservation is asserted only on load, never on the re-write path

> [!WARNING]
>
> - [testing.md](../../../docs/guidelines/testing.md) — assert against ground truth (bytes on disk), test the risky path

The acceptance criterion says the legacy `state.json` load "stamps `version:1`
while preserving `generator_version`." `internal/sync/schema_test.go:28` seeds
`generator_version:"2.0.0"` and asserts it survives `loadState` — but the risky
path is `Apply`'s re-write (`internal/sync/sync.go:529`), which hardcodes
`GeneratorVersion: GeneratorVersion` (the `"2.0.0"` constant) rather than
propagating the loaded value. The test passes only because the seed value equals
the current constant — the "expected value is a copy of the constant the code
also produced" anti-pattern. No test decodes a **re-written** `state.json` off
disk to check `generator_version` survived serialization (contrast
`asset/schema_test.go:41-49`, which correctly unmarshals the file to verify
`version`). So the write side of the "keep both fields" invariant is unverified.

```go
// internal/sync/schema_test.go — seed == current constant, so preservation is untested
seed := `{"generator_version":"2.0.0", ...}` // equals GeneratorVersion; can't catch a clobber
// after load: assert state.GeneratorVersion == "2.0.0"  <- passes even if Apply overwrites with the constant
```

Choose one:

- [x] Add a real-file test that seeds a **distinct** `generator_version` (e.g.
      `"1.5.0"`), runs the state re-write path, reads `state.json` back off disk,
      and asserts both `version:1` and the original `generator_version` are present
      — verifying the on-disk bytes, not the in-memory struct.
- [ ] If the intended behavior is "always stamp the current constant" (not
      preserve), rename the test and add a comment so a seed equal to the constant
      is not mistaken for round-trip preservation, and correct the acceptance
      criterion wording. (Also consider asserting the asset on-disk test against
      the literal `1` rather than `== Version`, so a mistaken const bump can't move
      the goalpost.)

## Task docs claim `Store` validation "cascades" to `project.Manifest`, but the boundary never reaches it

> [!WARNING]
>
> - [domain_model.md](../../../docs/guidelines/domain_model.md) — document current reality; ubiquitous language

`description.md:143` (and the framing in `docs/adr/0022-*.md`) state that
`project.Manifest` is validated because it nests inside `projectstore.State` and
"`Store` validation cascades to it." But `State.Validate()`
(`internal/projectstore/store.go:51-56`) only checks the version envelope;
per-project `Normalize()`/`Validate()` runs in the **separate**
`validateState`/`readAndCheck` pass, not at the `Persisted` boundary. The
aggregate boundary itself is sound (`project.Manifest` correctly is _not_
`Persisted` — it owns no file), but the stated reason is inaccurate: the boundary
does not reach project manifests at all. The `State.Migrate` docstring
(`store.go:41-45`) is already honest about this, so the drift is only in the
narrative docs.

```go
// internal/projectstore/store.go — the Persisted hook does NOT fan out to projects
func (s *State) Validate() errs.DomainError {
    if s.Version > Version { return errs.NewerSchemaVersionError{...} }
    return nil // no project.Manifest.Validate() call
}
```

Choose one:

- [ ] Correct `description.md` and ADR 0022 to say project manifests are
      validated by the store's own `validateState`/`readAndCheck` pass (a domain
      seam), not by the `Persisted` boundary — the boundary validates the envelope,
      the store validates its contents.
- [x] (Stronger) Make `State.Validate()` actually fan out to each
      `project.Manifest.Validate()` so the aggregate-consistency invariant lives at
      one boundary and the docs become true as written; leave `validateState`'s
      orphan/Normalize concerns where they are.

## Task docs and `CLAUDE.md` name the registry type `registry.Index`, but the code ships `registry.Registry`

> [!WARNING]
>
> - [documentation.md](../../../docs/guidelines/documentation.md) — document current reality; searchable names

`plan.md:17`, `description.md:117`, and the `CLAUDE.md` utils/registry references
call the persisted registry type `registry.Index`, but the actual type is
`registry.Registry` (`internal/registry/registry.go:39`). A reader who greps for
`registry.Index` finds nothing. This is a doc/code naming mismatch, not a source
smell.

```text
docs say:  registry.Index      (does not exist)
code is:   registry.Registry   (internal/registry/registry.go:39)
```

Choose one:

- [x] Update `description.md`, `plan.md`, and `CLAUDE.md` to say
      `registry.Registry`.
- [ ] Accept as-is (task docs are historical; the changelog and ADR already say
      `registry.Registry`).

## `asset.Manifest.Validate` uses a value receiver while every other persisted type uses a pointer receiver

> [!WARNING]
>
> - [go.md](../../../docs/guidelines/go.md) — similar concepts should look similar

`asset.Manifest.Validate` is a **value** receiver
(`internal/asset/asset.go:148`), while its own `Migrate` and all five other
types' `Validate` are **pointer** receivers. `*Manifest` still satisfies
`Persisted[Manifest]` because a value-receiver method is promoted into the
pointer's method set, so it compiles and works — and a value receiver is arguably
safer for `Validate` (it cannot mutate). But it is the one exception to the
pattern, so a future edit that copies this type as a template, or that needs
`Validate` to touch state, will behave differently here.

```go
func (m *Manifest) Migrate() errs.DomainError { ... }  // pointer
func (m Manifest)  Validate() errs.DomainError { ... } // value — the odd one out
```

Choose one:

- [x] Change to `func (m *Manifest) Validate()` for uniformity across all six
      persisted types, so the pattern is copy-safe.
- [ ] Leave as-is (documented in the plan; compiles; value receiver is the safer
      default for a read-only check).

## The `Migrate`/`Validate` doc comments are near-duplicated across all six persisted types

> [!WARNING]
>
> - [clean_code.md](../../../docs/guidelines/clean_code.md) — don't restate what the structure/contract already says

The `Migrate()`/`Validate()` doc comments are copy-pasted (with minor wording
drift) across `asset.go:134`, `profile.go:33`, `registry.go:44`,
`projectstore/store.go:37`, `settings.go:41`, `sync.go:114`. Each restates the
same three facts the `Persisted` constraint doc (`internal/utils/persist.go:15-28`)
already explains authoritatively (legacy sentinel = 0, pointer receiver,
reject-newer). The method bodies are trivial and identical, so the per-type
comments add little beyond locality.

```go
// registry.go — near-identical prose to 5 other files
// Migrate stamps a legacy (version 0) registry up to the current schema
// version. Pointer receiver so ReadJSON/WriteJSON own version stamping ...
func (r *Registry) Migrate() errs.DomainError { if r.Version == 0 { r.Version = Version }; return nil }
```

Choose one:

- [x] Reduce each to a one-line comment (e.g.
      `// Migrate stamps the legacy sentinel to Version; see utils.Persisted.`) and
      let `persist.go` own the full rationale.
- [ ] Accept as intentional per-type boilerplate (locality over DRY) and leave
      as-is.
