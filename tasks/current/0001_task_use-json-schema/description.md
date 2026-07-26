---
id: 0001
type: task
status: in-review
topics: research
---

# Use Json Schema

Currently we load JSON as-is and don't validate whether the file has the appropriate structure.

We need to migrate this to use JSON Schema, or some other schema utility whenever when serialize/deserialize JSON.

The related functions in `fsutil.go` also need to be refactored and they need to be generic functions.

All data structures that we load into the app need to have their own model type in go to keep type-safety.

As part of this task we also need to find the appropriate library. The goal is to have a mechanism that can

- Serialize in-memory data based on a type into a JSON file (the `WriteJSON` function)
- Deserialize JSON into in-memory data based on a type from a JSON file (the `ReadJSON` function)
- Signal an error in the TUI if the file is malformed

**Note that** it is possible that we don't need JSON Schema for this. If Go already has built-in functionality
that we can re-use then JSON Schema won't be necessary, but it needs to be an utility that allows for
**schema evolution**. **Research this!**

## Clarification

### Question

Research found JSON Schema does **not** provide schema evolution — it only validates shape at
one point in time; migration is always application-owned code. The version-envelope +
migration pattern the codebase already uses (version fields, `migrate` pkg) is the idiomatic
Go path. JSON Schema (via `santhosh-tekuri/jsonschema` + `invopop/jsonschema`) is only worth
the dependency when the schema must **leave the Go process** (published for external tooling,
editor validation of hand-edited files, cross-language contract). Is there such a need?

### Answer

No — the schemas don't leave `af`, so we don't need JSON Schema. We do need validation. Are we
doing that already? (Answer: only partially — `asset.Manifest` and `project.Manifest` have
`Validate()` wired into their load path; `profile.Profile`, `registry.Registry`,
`projectstore.Store`, `settings.Settings`, and `sync.ManagedState` have no validation on load,
because `utils.ReadJSON` is a bare `json.Unmarshal` and validation is a separate call each
caller must remember.)

### Question

Generic signature fork: (A) `ReadJSON[T Validator]` where every persisted type **must**
implement `Validate()` — compiler-enforced, validation auto-runs on load; or (B) plain
`ReadJSON[T any]` + separate `ReadJSONValidated` — opt-in, keeps the "forgot to validate"
foot-gun. Which?

### Answer

A. We must not allow forgetting validation. Also: all persisted data must carry a `version` for
easy migration (`asset.json` has none; `state.json` has `generator_version`). And create a Go
guideline postulating that all persisted data must be automatically validated and versioned.

### Question

Version-field representation fork: (A) normalize everything to `version int` — rename
`generator_version`→`version` on `state.json`, breaks existing files, needs read-time
migration; (B) add `version int` to `asset.Manifest` only, tolerate two conventions; (C) add
`version int` to both `asset.Manifest` and `ManagedState`, keeping `generator_version` for its
existing meaning. Which?

### Answer

C.

### Question

(a) Should `WriteJSON` also validate **before** writing (refuse to persist invalid data), across
all three write variants? (b) Existing `asset.json` files lack a `version` key → `encoding/json`
zero-fills to `0`. Should `0` be a legacy sentinel (= treat as v1, migrate up, preserve existing
files) or invalid (Validate rejects, breaks current profiles on first load)?

### Answer

(a) yes. (b) legacy sentinel.

### Question

(a) Is actual migration logic in scope for 0001, or just the infrastructure? (b) Should
`Validate()` reject a version **newer** than the code knows (forward-compat guard against an old
binary mangling a newer file)?

### Answer

(a) infra + a `Migrate()` hook in the read/write pipeline (defined seam for future bumps, even
though today's only migration is the v0→v1 legacy-sentinel stamp). (b) yes — reject newer.

## Acceptance Criteria

- [ ] `utils.ReadJSON` and all three write funcs (`WriteJSON`, `WriteJSONMode`, `WriteJSONAtomic`)
      are generic over a `Persisted[T]` constraint that requires `Migrate()` + `Validate()`; a
      non-`Persisted` type is a **compile error** — validation cannot be bypassed.
- [ ] `TestReadJSON_RoundTrip` (real `t.TempDir()` file): `WriteJSON` then `ReadJSON` returns an
      equal value with `version` stamped to current.
- [ ] `TestReadJSON_LegacySentinel` (real on-disk file with no `version` key): load succeeds, the
      returned value's `version == current` (Migrate ran).
- [ ] `TestReadJSON_RejectsNewerVersion` (real file, `version` > current): returns
      `errs.NewerSchemaVersionError`.
- [ ] `TestReadJSON_Malformed` (real garbage file): returns `ReadJSONError`.
- [ ] `TestWriteJSON_RefusesInvalid` (real `t.TempDir()`): `WriteJSON` of a value failing
      `Validate` returns an error **and the target file does not exist afterward**.
- [ ] `TestWriteJSONAtomic_RefusesInvalid`: same validate-before-write guarantee and leaves no
      `.tmp-*` residue.
- [ ] `asset.Manifest` gains `version int` + `const Version`; a real legacy `asset.json` (no
      `version`) in `t.TempDir()` loads and, on re-save, has on-disk `version == 1`; a
      `version: 2` manifest is rejected with `NewerSchemaVersionError`.
- [ ] `sync.ManagedState` gains `version int` **alongside** `generator_version`; a real legacy
      `state.json` (has `generator_version`, no `version`) stamps `version:1` while preserving
      `generator_version`; the existing `ManagedFileEntry` bare-hash back-compat test still passes.
- [ ] `registry.Registry`, `projectstore.Store`, `settings.Settings`, `profile.Profile` each gain
      `Migrate()` + `Validate()`: a real legacy file (missing/zero `version`) loads and stamps
      current, and a newer-version file is rejected (one real-file test per type).
- [ ] All seven `ReadJSON`/`WriteJSON` call sites compile against the generic API; the redundant
      standalone `manifest.Validate()` calls in `asset.go` are removed (validation now runs inside
      `ReadJSON`); `migrate.go`'s raw legacy-DTO unmarshal is left intact.
- [ ] `docs/guidelines/go.md` contains a "Persist Only Validated, Versioned Data" section
      (`grep -n "Persist Only Validated" docs/guidelines/go.md` matches).
- [ ] `docs/adr/0022-validated-versioned-persistence-boundary.md` exists and records the
      JSON-Schema rejection, decision C for `state.json`, the legacy-sentinel and reject-newer
      rules, and the `Migrate` seam.
- [ ] `docs/glossary.md` gains "persistence boundary", "schema version", "legacy sentinel"
      entries; the `CLAUDE.md` utils bullet mentions the validated/versioned JSON boundary.
- [ ] `go.mod` has no new dependency (JSON Schema libraries rejected).
- [ ] `make build && make test && make lint` all pass.

## Out of scope

- Adopting a JSON Schema library (`santhosh-tekuri/jsonschema`, `invopop/jsonschema`) — research
  rejected it; schemas never leave the `af` process.
- Any real schema-evolution migration **beyond** the v0→v1 legacy-sentinel stamp. Future
  version-bump migrations are deferred to follow-up task `0043_task_schema-migration-guardrails`
  (`depends_on: 0001`). Only the `Migrate()` seam is delivered now.
- Renaming or removing `generator_version` on `state.json` (decision C keeps it for its existing
  `ManagedFileEntry`-format meaning; a separate `version int` is added beside it).
- Giving `project.Manifest` its own `Persisted`/`Migrate` treatment — it persists **inside**
  `projectstore.Store`, not as its own file; `Store` validation cascades to it.
- Migrating `migrate.go`'s raw legacy-registry DTO unmarshal to the generic path — transient
  pre-versioning DTOs stay raw (sanctioned escape hatch).

## Verification

1. `make build && make test && make lint` — all green.
2. `go test ./internal/utils -run 'TestReadJSON|TestWriteJSON'` — round-trip, legacy sentinel,
   reject-newer, malformed, and refuse-invalid (both plain and atomic) cases pass.
3. `go test ./internal/asset ./internal/sync ./internal/registry ./internal/projectstore ./internal/settings ./internal/profile`
   — each package's legacy-load + reject-newer real-file tests pass.
4. `grep -n "Persist Only Validated" docs/guidelines/go.md` matches.
5. `ls docs/adr/0022-validated-versioned-persistence-boundary.md` exists.
6. `grep -nE "persistence boundary|schema version|legacy sentinel" docs/glossary.md` matches all three.
7. `git diff HEAD -- go.mod` is empty (no new dependency).
