---
id: 0001
type: task
status: in-progress
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
`Validate()` wired into their load path; `profile.Profile`, `registry.Index`,
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
