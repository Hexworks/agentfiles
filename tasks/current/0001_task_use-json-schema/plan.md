# Plan — 0001 Use Json Schema (→ validated, versioned persistence boundary)

Cross-links: [description.md](./description.md) · new ADR `docs/adr/0022-validated-versioned-persistence-boundary.md` · new section in `docs/guidelines/go.md` · follow-up task `tasks/backlog/0043_task_schema-migration-guardrails` (`depends_on: 0001`).

## Research outcome (decided)

JSON Schema is **not** adopted. It validates shape at a point in time and does **not** provide schema evolution; migration is always application code. The schemas never leave the `af` process, so an external-contract library earns no dependency. The idiomatic Go path — and the one the codebase already half-uses — is: stdlib `encoding/json` + generics + a `version` envelope on every persisted type + app-owned `Migrate()` + `Validate()`, all enforced at a single load/save boundary so validation and migration cannot be forgotten.

## Goal

Turn `utils.ReadJSON`/`WriteJSON` into **type-parameterised** functions constrained to a `Persisted[T]` interface so that **every** load and save automatically runs `Migrate()` then `Validate()`. Give every persisted structure a `version` field. Add `Validate()`/`Migrate()` to the five types that lack them. Forward-compat: reject a file whose version is newer than this binary knows. Backward-compat: a missing `version` (0) is a legacy sentinel migrated up to current. Codify the rule as a Go guideline and an ADR.

## Persisted types in scope

| File | Go type | Has `version` today | Has `Validate()` today |
|---|---|---|---|
| `~/.agentfiles/profiles.json` | `registry.Index` | `version int` ✓ | ✗ |
| `~/.agentfiles/projects.json` | `projectstore.Store` | `version int` ✓ | ✗ |
| `~/.agentfiles/settings.json` | `settings.Settings` | `version int` ✓ | ✗ |
| `<profile>/profile.json` | `profile.Profile` | `version int` ✓ | ✗ |
| `<profile>/assets/<type>/<id>/asset.json` | `asset.Manifest` | ✗ **add** | ✓ (value receiver) |
| `<repo>/.agentfiles/state.json` | `sync.ManagedState` | `generator_version string` — **add `version int`** (decision C) | ✗ |

## Design

### The constraint (in `internal/utils/persist.go`, new file)

```go
// Persisted is the constraint every on-disk JSON document type satisfies.
// P is the pointer type of T; both Migrate and Validate have pointer
// receivers because Migrate stamps the version field in place.
type Persisted[T any] interface {
	*T
	// Migrate upgrades an older/legacy in-memory value to the current
	// schema version. A missing version (0) is the pre-versioning legacy
	// sentinel and is stamped to the current version. It never downgrades.
	Migrate() errs.DomainError
	// Validate checks domain shape AND rejects a version newer than this
	// binary knows (forward-compat guard).
	Validate() errs.DomainError
}
```

### Generic read/write (rewrite of the four funcs in `internal/utils/fs.go`)

Pipeline is identical on both sides: **decode/prepare → `Migrate()` → `Validate()` → (write only) marshal+persist.** Running `Migrate` before `Validate` on write means a freshly-constructed `version:0` value is stamped current before the newer-than-known check.

```go
func ReadJSON[T any, P Persisted[T]](path string) (T, errs.DomainError)
func WriteJSON[T any, P Persisted[T]](path string, v T) errs.DomainError
func WriteJSONMode[T any, P Persisted[T]](path string, v T, dirMode, fileMode fs.FileMode) errs.DomainError
func WriteJSONAtomic[T any, P Persisted[T]](path string, v T, dirMode, fileMode fs.FileMode) errs.DomainError
```

- `ReadJSON` returns the value (task requirement: "deserialize JSON into in-memory data based on a type"). Malformed JSON → `ReadJSONError` (unchanged type); newer version → new `NewerSchemaVersionError`.
- Call site `ReadJSON[asset.Manifest](path)` relies on Go 1.21+ core-type inference to derive `P = *asset.Manifest`. **Assumption to verify at implementation** (grounding table); fallback is the explicit two-arg form `ReadJSON[asset.Manifest, *asset.Manifest](path)`.
- `WriteJSON(path, manifest)` infers both `T` and `P` from the argument — no extra type args at call sites.
- `WriteJSON*` operate on a **copy** (`v T` passed by value), so `Migrate` stamping the version never mutates the caller's value.
- The write path must **not** persist the file when `Validate` fails (assert file absent/unchanged in tests).

### Escape hatch for pre-versioning DTOs

`migrate.go:277` already uses a raw `json.Unmarshal` for an isolated legacy registry shape that predates versioning. That stays raw — transient migration DTOs are **not** forced to implement `Persisted`. `migrate.go:302` reads `asset.Manifest` and moves to the generic form (legacy `asset.json` with no version → `Migrate` stamps v1 → passes).

### Per-type changes

- **`asset.Manifest`**: add `Version int \`json:"version"\`` (first field), add `const Version = 1`, add `Migrate()` (pointer receiver: `if m.Version == 0 { m.Version = Version }`), extend `Validate()` to also `if m.Version > Version { return NewerSchemaVersionError{...} }`. Keep existing shape checks. (Existing `Validate` value receiver is promoted into `*Manifest`'s method set, so it still satisfies the constraint alongside the pointer-receiver `Migrate`.)
- **`sync.ManagedState`**: add `Version int \`json:"version"\`` **alongside** existing `generator_version` (decision C), add `const SchemaVersion = 1` (distinct from the `GeneratorVersion` semver string, which keeps its `ManagedFileEntry`-format meaning), add `Migrate()` + `Validate()` (currently none) with the sentinel + reject-newer logic.
- **`registry.Index`, `projectstore.Store`, `settings.Settings`, `profile.Profile`**: they already have `version int` + `const Version = 1`; add `Migrate()` (sentinel 0→`Version`) and `Validate()` (reject-newer + minimal existing field checks). Remove now-redundant manual `reg.Version = Version` stamping in `registry.Save`/`Load` (lines 85/99) — `Migrate` in the write pipeline owns stamping, single source.
- **`project.Manifest`**: already has `Validate()`. It is persisted **inside** `projectstore.Store` (not its own file), so it does not need `Persisted`; `Store.Validate()` will fan out to each project's `Validate()`. Add a no-op-friendly `Migrate()` only if `Store.Migrate` needs to cascade (decide during impl; default: cascade validation only).

### New error type (`internal/utils/errors.go`)

```go
type NewerSchemaVersionError struct { Path string; Have, Known int }
// Error(): "…was written by a newer version of af (schema v%d, this build knows v%d): %s"
// Severity(): SeverityError
```
`Validate()` implementations on each type return this (they own their `Version` const). Because `Validate` lives in the domain package and this error lives in `utils`, either (i) put `NewerSchemaVersionError` in `internal/errs`, or (ii) give each package its own. **Decide:** place it in `internal/errs` (shared, single definition) — grounding: `errs` is already the common error-severity home imported everywhere.

## Execution plan

1. **`internal/errs`**: add `NewerSchemaVersionError` (shared, referenced by every persisted type's `Validate`).
2. **`internal/utils/persist.go`** (new): define `Persisted[T]` constraint.
3. **`internal/utils/fs.go`**: rewrite `ReadJSON`, `WriteJSON`, `WriteJSONMode`, `WriteJSONAtomic` as generic constrained funcs with the `Migrate→Validate` pipeline; drop the `@see task#0001` TODOs. Keep `WriteFile`/`CopyDir`/hashing untouched (not JSON-typed).
4. **`asset.Manifest`**: add version field + const + `Migrate` + reject-newer in `Validate`.
5. **`sync.ManagedState`**: add `version int` + `SchemaVersion` const + `Migrate` + `Validate`.
6. **`registry` / `projectstore` / `settings` / `profile`**: add `Migrate` + `Validate`; drop redundant registry stamping.
7. **Update all call sites** to the generic form: `asset.go` (Load + 3 other ReadJSON/Validate pairs — the separate `manifest.Validate()` calls become redundant and are removed), `profile.go:83`, `registry.go:81`, `projectstore/store.go:154`, `settings/store.go:47`, `sync/sync.go:854`, `migrate/migrate.go:302`. Leave `migrate.go:277` raw.
8. **Guideline**: add section to `docs/guidelines/go.md` (below "Keep I/O At The Edges") titled **"Persist Only Validated, Versioned Data"** — portable body (every persisted document carries a `version`; load/save runs migrate-then-validate at one boundary; missing version = legacy sentinel; reject newer-than-known) + a short repo appendix pointing at `utils.ReadJSON`/`Persisted`.
9. **ADR** `docs/adr/0022-validated-versioned-persistence-boundary.md`: decision, the C choice for `state.json`, legacy-sentinel + reject-newer rules, the `Migrate` seam, why JSON Schema was rejected.
10. **Docs**: update the `utils` bullet in `CLAUDE.md` (note the generic validated JSON boundary + `Persisted` constraint); add glossary entries to `docs/glossary.md` for **persistence boundary**, **schema version**, **legacy sentinel**.
11. **Tests** (below).
12. `make build && make test && make lint`.

## Tests

Boundary-crossing (filesystem) → real-stack with `t.TempDir()`, per DoD rule.

- **`internal/utils` (new `persist_test.go` / extend `fs_test.go`)** using a local fake `Persisted` type:
  - round-trip: `WriteJSON` then `ReadJSON` returns equal value with `version` stamped.
  - legacy sentinel: write a raw file with **no** `version` key → `ReadJSON` returns value with `version == current` (Migrate ran).
  - reject-newer: raw file with `version: 99` → `ReadJSON` returns `NewerSchemaVersionError`.
  - malformed JSON → `ReadJSONError`.
  - write refuses invalid: `WriteJSON` of a value whose `Validate` fails → error **and file does not exist** on disk.
  - `WriteJSONAtomic` variant: same validate-before-write guarantee; no `.tmp-*` residue left.
- **`asset`**: load a real legacy `asset.json` (no `version`) from `t.TempDir()` succeeds and re-save stamps `version:1`; `Validate` rejects `version:2`.
- **`sync`**: load a real legacy `state.json` (has `generator_version`, no `version`) → sentinel stamps `version:1`, `generator_version` preserved; reject-newer on `version` too high; existing `ManagedFileEntry` bare-hash back-compat test still passes.
- **`registry`/`projectstore`/`settings`/`profile`**: legacy file (no/zero `version`) loads and stamps current; newer version rejected.
- **`migrate`**: existing migration test still passes; a legacy `asset.json` migrated via `migrate.go:302` loads through the generic path.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| `ReadJSON`/`WriteJSON` are the two funcs the task targets and carry the `@see task#0001` TODO | `internal/utils/fs.go:59,76` | `// TODO: make this a generic function (@see task#0001)` |
| `ReadJSON` is currently a bare `json.Unmarshal` with no validation call | `internal/utils/fs.go:60-69` | `if err := json.Unmarshal(data, v); err != nil {` |
| `asset.Manifest.Validate` exists, value receiver, no version check | `internal/asset/asset.go:129` | `func (m Manifest) Validate() errs.DomainError {` |
| `asset.json` has no `version` field (fields are id/name/type/…/projections) | `internal/asset/asset.go:78-118` | `ID … json:"id"`; no `version` tag present |
| `state.json` carries `generator_version` semver string, not an int version | `internal/sync/sync.go:27,86` | `const GeneratorVersion = "2.0.0"` ; `GeneratorVersion string \`json:"generator_version"\`` |
| `ManagedState` has no `Validate()` (custom logic is `ManagedFileEntry.UnmarshalJSON` back-compat) | `internal/sync/sync.go:55-74` | `func (e *ManagedFileEntry) UnmarshalJSON(data []byte) error {` |
| `registry`/`projectstore`/`settings`/`profile` already have `const Version = 1` + `version int` | `registry.go:18`, `projectstore/store.go:24`, `settings/settings.go:9`, `profile.go:22` | `const Version = 1` |
| `registry.Save` already stamps `reg.Version = Version` (to be removed as redundant) | `internal/registry/registry.go:85,99` | `reg.Version = Version` |
| `migrate.go` uses a raw `json.Unmarshal` for a legacy DTO (escape hatch precedent) and `utils.ReadJSON` for the asset manifest | `internal/migrate/migrate.go:277,302` | `json.Unmarshal(data, &reg)` ; `utils.ReadJSON(manifestPath, manifest)` |
| Every `ReadJSON`/`WriteJSON` call site to migrate | `asset.go:160`, `profile.go:83`, `registry.go:81`, `projectstore/store.go:154`, `settings/store.go:47`, `sync/sync.go:854`, `migrate.go:302` | (listed grep hits) |
| Next ADR number is 0022 | `docs/adr/` (highest is `0021-render-reverse-strategy-table.md`) | — |
| Repo builds on Go 1.26.1 (generics + core-type inference available) | `go.mod:3` | `go 1.26.1` |
| stdlib `encoding/json` ignores unknown keys (forward-compat) and zero-fills missing (backward-compat) → `version` absent decodes to `0` | Go `encoding/json` docs (external) — relied upon for the legacy sentinel | — |
| Go 1.21+ infers the pointer type param `P` from constraint core type `*T` (so `ReadJSON[asset.Manifest]` compiles) | Go spec, type inference (external) — **verify at implementation**; fallback = explicit `ReadJSON[T, *T]` | — |

## ADRs / docs / guidelines summary

- **New ADR**: `docs/adr/0022-validated-versioned-persistence-boundary.md`.
- **New guideline section**: `docs/guidelines/go.md` → "Persist Only Validated, Versioned Data".
- **Docs updated**: `CLAUDE.md` (utils bullet), `docs/glossary.md` (persistence boundary / schema version / legacy sentinel).
- **No** new dependency (`go.mod` unchanged; JSON Schema libraries rejected).

## Acceptance Criteria

- [ ] `utils.ReadJSON` and all three write funcs are generic over `Persisted[T]`; a non-`Persisted` type is a **compile error** (verified by a `// want` / build-fail note or a documented negative example) — validation cannot be bypassed.
- [ ] `TestReadJSON_RoundTrip` (real `t.TempDir()` file): `WriteJSON` then `ReadJSON` returns an equal value with `version` stamped to current.
- [ ] `TestReadJSON_LegacySentinel` (real file with no `version` key on disk): load succeeds, in-memory `version == current`.
- [ ] `TestReadJSON_RejectsNewerVersion` (real file, `version` > current): returns `NewerSchemaVersionError`.
- [ ] `TestReadJSON_Malformed` (real garbage file): returns `ReadJSONError`.
- [ ] `TestWriteJSON_RefusesInvalid` (real `t.TempDir()`): `WriteJSON` of a value failing `Validate` returns an error **and the target file does not exist afterward**.
- [ ] `TestWriteJSONAtomic_RefusesInvalid` + leaves no `.tmp-*` residue.
- [ ] `asset.Manifest` gains `version int` + `const Version`; `TestAsset_LoadsLegacyManifestNoVersion` (real legacy `asset.json` in `t.TempDir()`) loads and, on re-save, on-disk `version == 1`; `TestAsset_RejectsNewerManifest` returns `NewerSchemaVersionError`.
- [ ] `sync.ManagedState` gains `version int` beside `generator_version`; `TestSync_LoadsLegacyStateNoVersion` (real legacy `state.json`) stamps `version:1` while preserving `generator_version`; existing `ManagedFileEntry` bare-hash back-compat test still passes.
- [ ] `registry.Index`, `projectstore.Store`, `settings.Settings`, `profile.Profile` each: a real legacy file (missing/zero `version`) loads and stamps current; a newer-version file is rejected (one real-file test per type).
- [ ] All seven `ReadJSON`/`WriteJSON` call sites compile against the generic API; the redundant standalone `manifest.Validate()` calls in `asset.go` are removed (validation now runs inside `ReadJSON`); `migrate.go:277` raw unmarshal is left intact.
- [ ] `docs/guidelines/go.md` contains a "Persist Only Validated, Versioned Data" section; `grep -n "Persist Only Validated" docs/guidelines/go.md` matches.
- [ ] `docs/adr/0022-validated-versioned-persistence-boundary.md` exists and records the JSON-Schema rejection, decision C, legacy-sentinel, reject-newer, and the `Migrate` seam.
- [ ] `docs/glossary.md` gains "persistence boundary", "schema version", "legacy sentinel"; `CLAUDE.md` utils bullet mentions the validated/versioned JSON boundary.
- [ ] `go.mod` has no new dependency.
- [ ] `make build && make test && make lint` all pass.
