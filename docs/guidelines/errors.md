# Error Guidelines

Errors in `agentfiles` are domain values, not strings. Callers (especially the
TUI) need enough metadata to pick a color, an icon, a severity, or to
introspect a specific failure with `errors.As`. Hard-coded `fmt.Errorf`
strings hide that metadata and are not usable for anything beyond printing.

Related: [Clean Architecture](./clean_architecture.md), [Go](./go.md),
ADR [0007](../adr/0007-rendering-belongs-to-tui.md),
ADR [0008](../adr/0008-domain-error-everywhere.md).

## The Rule

**Every function that can fail returns `errs.DomainError` or
`[]errs.DomainError`. Plain `error` is not allowed in domain or
application code.**

Use a single `errs.DomainError` for point operations (`Validate`,
`Save`, `Load`). Use `[]errs.DomainError` for accumulator-shape
functions that walk a collection and want to surface every failure at
once (`render.Build`, `app.AddProject`,
`sync.detectDeleteCandidates`).

The TUI (`internal/tui/`) is the only layer that may keep using the
standard `error` return type in its own helpers, because its values
feed `RenderError`, which already accepts `error` for back-compat with
`huh.ErrUserAborted`-style sentinels.

### Wrapping External Errors

When a failure originates in an imported package (`os`,
`encoding/json`, `path/filepath`, third-party libs), declare a typed
error in the package's `errors.go` with the appropriate
`Severity()` and an `Unwrap()` that preserves the underlying value.
Example: `utils.ReadJSONError{Path, Err}`.

### Propagating Domain Errors

If every failure inside a function originates from another helper that
already returns `errs.DomainError`, do not re-wrap it. Just propagate:

```go
func (m *Manifest) Normalize() errs.DomainError {
    abs, err := utils.ToAbsolute(m.Path)
    if err != nil {
        return err // already a DomainError
    }
    m.Path = abs
    ...
    return nil
}
```

Re-wrapping a domain error in another domain error loses the original
severity and adds a layer that `errs.Collect` has to walk for nothing.

## Use Typed Errors For Domain Failures

Each domain package owns an `errors.go` file with one struct per failure
mode. Every typed error satisfies `errs.DomainError`: it implements
`Error()` for the standard `error` interface and `Severity() errs.Severity`
so callers can render an icon, pick a color, or filter by severity
without enumerating concrete types.

```go
// internal/render/errors.go

import "github.com/addamsson/agentfiles/internal/errs"

// AssetNotFoundError reports a selected asset id that does not exist in
// the profile. It carries the offending id so the TUI can render a
// precise message without reparsing strings.
type AssetNotFoundError struct {
    AssetID string
}

func (e AssetNotFoundError) Error() string {
    return fmt.Sprintf("selected asset not found: %s", e.AssetID)
}

func (AssetNotFoundError) Severity() errs.Severity {
    return errs.SeverityError
}
```

The `Error()` text should match the previous `fmt.Errorf` string verbatim
when refactoring an existing site, so plain-text consumers keep working.

## Severity Is A Domain Concern

`errs.Severity` is one of `SeverityInfo`, `SeverityWarning`,
`SeverityError`. It classifies the *kind* of failure (hard vs.
recoverable), not its rendering. The TUI consumes the result directly;
adding a new typed error never requires editing the TUI.

| Pattern                              | Severity            |
| ------------------------------------ | ------------------- |
| Validation failure, missing entity   | `SeverityError`     |
| Conflict the user can resolve        | `SeverityWarning`   |
| Informational notice                 | `SeverityInfo`      |

## Accumulator Functions Return `[]errs.DomainError`

Loops over a collection should not short-circuit when one element fails.
The caller usually wants the full list (every missing id, every
conflicting group) to fix everything at once. Accumulator-shape
functions return the slice directly:

```go
func resolveAssets(p *profile.Profile, proj *project.Manifest) ([]*asset.Asset, []errs.DomainError) {
    var selected []*asset.Asset
    var domainErrs []errs.DomainError
    for _, id := range proj.SelectedAssetIDs {
        a := p.Assets[id]
        if a == nil {
            domainErrs = append(domainErrs, AssetNotFoundError{AssetID: id})
            continue
        }
        selected = append(selected, a)
    }
    return selected, domainErrs
}
```

When iterating maps, sort the keys before appending so the slice order
is deterministic across runs.

Two exceptions:

1. The loop body performs filesystem I/O on the same resource (an
   `os.ReadFile` that other iterations also depend on). A mid-loop
   failure already implies the whole result is unusable; fail fast.
2. The error is a programming bug (nil pointer, invalid type) rather
   than an input failure. Fail fast and let the caller fix the bug.

## Non-accumulator Functions Wrap In `errs.Errors`

Functions that consume an accumulator but return a single
`errs.DomainError` (`sync.Plan`, `app.Plan`, `app.Apply`) wrap the
slice in `errs.Errors`. `errs.Errors` is itself a `DomainError`: its
`Severity()` returns the highest severity of its members, and its
`Unwrap() []error` lets `errors.As` walk the leaves.

```go
rendered, renderErrs := render.Build(p, proj)
if len(renderErrs) > 0 {
    return nil, errs.Errors(renderErrs)
}
```

There is no escape hatch for plain `error`. Helpers like
`utils.HashFile` and `utils.ReadJSON` return `errs.DomainError`
directly, so the rest of the stack does not need to invent
"wrapInternal" shims to lift `os` errors back into the domain
vocabulary. Callers that want the typed leaves call `errs.Collect`,
which flattens `Unwrap() []error` and `Unwrap() error` chains
uniformly.

## Inspecting Errors

Callers (TUI, tests) inspect with `errors.As` for one specific type, or
with `errs.Collect` to walk every domain leaf:

```go
var notFound render.AssetNotFoundError
if errors.As(err, &notFound) {
    // notFound.AssetID is available
}

for _, leaf := range errs.Collect(err) {
    fmt.Println(leaf.Severity(), leaf.Error())
}
```

## Tests

Assert on typed errors, not on `err.Error()` substrings:

```go
_, buildErrs := render.Build(p, proj)
if len(buildErrs) == 0 {
    t.Fatal("expected error")
}
var conflict render.ExclusiveGroupConflictError
if !errors.As(buildErrs[0], &conflict) {
    t.Fatalf("expected ExclusiveGroupConflictError, got %T: %v", buildErrs[0], buildErrs[0])
}
```

This makes tests resilient to copy editing of the message string and
documents the contract the domain layer promises to its callers.
