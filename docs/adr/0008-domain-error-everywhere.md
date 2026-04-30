# DomainError Is The Only Error Shape

## Status

accepted

## Context

ADR [0007](./0007-rendering-belongs-to-tui.md) introduced
`errs.DomainError` and moved each domain package's error catalogue into
its own `errors.go` file. The contract was applied unevenly: render and
app produced typed errors, but several lower-level helpers
(`fsutil.EnsureDir`, `fsutil.WriteJSON`, `fsutil.HashFile`,
`fsutil.ToAbsolute`, `registry.Add`, `project.Validate`,
`profile.scanAssets`, `sync.Plan`, …) still returned plain `error`
values built with `fmt.Errorf` or `errors.New`.

That left holes in the rendering pipeline. The TUI calls
`RenderError`, which delegates to `errs.Collect` to walk the typed
leaves; a plain error has no severity, no fields, and no icon. The
fallback path renders such errors with the generic `✗` and the raw
message — usable, but it bypasses the domain vocabulary the rest of
the stack already speaks.

Mixed shapes also made callers awkward. Some functions returned
`error`, some returned `[]errs.DomainError`, and some returned
`(*Result, error)` while a sibling returned `(*Result, []errs.DomainError)`.
Conversions were ad-hoc and introduced wrappers like `app.InternalError`
whose only purpose was to lift a plain `error` back into the domain
vocabulary at a single call site.

## Decision

Every function in the codebase that can fail returns either:

1. a single `errs.DomainError`, or
2. a slice `[]errs.DomainError` when the function naturally accumulates
   independent failures (e.g. `render.Build`, `app.Service.AddProject`,
   `doctor.CheckProfile`, `sync.detectDeleteCandidates`).

Plain `error` returns are not allowed in domain or application code.
The TUI layer may continue to satisfy the standard `error` interface in
its own helpers (`runForm`, `selectProfile`) because those values feed
back into `RenderError`, which already accepts `error`.

Consequences of the rule:

- `errs.Errors` (the slice-typed wrapper) now also satisfies
  `errs.DomainError` so a function that internally collects multiple
  failures can wrap them as a single value without losing severity.
  `Severity()` returns the highest severity of the slice members.
- Helpers that previously returned plain `error`
  (`internal/fsutil/*`, `registry.*`, `project.Validate/Normalize/Save`,
  `profile.Init/Load/scan*`, `asset.Validate/Load/Init/RelativeFiles`,
  `sync.Plan/Apply/loadState/detectDeleteCandidates`) now declare
  `errs.DomainError` (or `[]errs.DomainError`) in their signatures.
- Each package owns one `errors.go` file. The previously inline
  `doctor.ProjectCheckError` was moved out of `doctor.go` for symmetry.
- When all failure modes inside a function come from imported helpers
  that already return `errs.DomainError`, the wrapping ceremony is
  dropped — callers just propagate the value. The
  `app.InternalError`/`wrapInternal` shim that existed for that purpose
  is removed.
- New typed errors are added in `internal/fsutil/errors.go`,
  `internal/registry/errors.go`, `internal/project/errors.go`,
  `internal/profile/errors.go`, and `internal/sync/errors.go` so the
  external libraries (`os`, `encoding/json`, `path/filepath`) that
  agentfiles wraps each get a domain-level severity and message.

## Consequences

The TUI rendering pipeline becomes total: every error reaching
`RenderError` carries a severity, an icon, and a structured message.
The "raw error fallback" branch in `RenderError` survives only as a
defensive net for tests that pass `errors.New(...)` directly.

Conversions and wrappers shrink. `app.InternalError` is gone. The
sync layer no longer mixes `errs.Errors`-wrapped accumulators with
naked I/O errors; both surface as `errs.DomainError`.

The rule is enforced by the type system: a function declared to return
`errs.DomainError` cannot silently leak a plain `error`, and code
review is no longer required to catch the mistake.

The cost is breadth — almost every package gained a few lines in its
`errors.go` and a few tweaks to its public signatures. That cost is
paid once; new code follows the same pattern by default.
