# Rendering Belongs To The TUI

## Status

accepted

## Context

Domain packages (`asset`, `profile`, `render`, `sync`, `doctor`, `app`) had
grown two presentation responsibilities that should not have lived there:

1. They produced human-facing strings directly. `sync.FormatPreview` built a
   text summary with `fmt.Fprintf`. `doctor.CheckProfile` returned
   `(string, error)` and assembled the report with `fmt.Fprintf`.
2. Their errors were created with `fmt.Errorf("...%s...", value)`. Callers had
   only the rendered string to work with — no `Path`, no `ProfileName`, no
   asset id. The TUI could not pick a color, an icon, or a severity from a
   plain string without parsing it.

Several of those error sites also short-circuited inside loops, returning the
first failure and discarding the rest. Users saw one missing asset id at a
time even when several were missing.

This blurred the dependency direction laid out in the existing clean
architecture and domain-model guidelines: stable policy was importing
`fmt` for presentation, and the TUI ended up printing whatever string the
domain layer happened to produce.

## Decision

The TUI is the only place that turns domain values into user-facing text.

Concretely:

- Domain packages return typed errors that satisfy `errs.DomainError`.
  Each package owns an `errors.go` file with one struct per failure mode
  (for example `render.AssetNotFoundError{AssetID string}`,
  `app.ProjectPathOwnedError{Path, ProfileName, ProjectName string}`).
  Every type implements both `Error()` (so the value still satisfies
  `error`) and `Severity() errs.Severity`. Severity is part of the
  domain because it answers "is this a hard error or a recoverable
  warning" — a question about the kind of failure, not how it is
  rendered.
- Accumulator-shape functions (`render.Build`, `render.resolveAssets`,
  `app.Service.AddProject`, `app.Service.ensureProjectPathAvailable`,
  `doctor.CheckProfile`) return `[]errs.DomainError` directly so the
  caller sees every issue at once without unwrap dance.
  Non-accumulator functions that consume an accumulator (sync.Plan,
  app.Plan, app.Apply) wrap the slice in `errs.Errors` and return a
  single `error`; the TUI calls `errs.Collect` to flatten the typed
  leaves before rendering.
- Pure formatting helpers are moved into `internal/tui/`.
  `sync.FormatPreview` is replaced by `tui.RenderPreview`;
  `doctor.CheckProfile` returns a `*doctor.Report` struct that the TUI
  formats via `tui.RenderReport`. No domain package imports `fmt` for
  output anymore — only for `Error()` string composition.
- The TUI dispatches presentation off `err.Severity()` instead of
  enumerating every concrete typed error in a switch. New typed errors
  pick up icon + color automatically by implementing the interface.
- Lipgloss styles live in one place (`internal/tui/styles.go`) so future
  styling changes stay local.
- TUI rendering applies a `safe()` helper that strips ASCII control
  bytes from manifest-sourced strings before they reach the terminal,
  so a hostile or accidentally-edited manifest cannot inject ANSI
  escape sequences.

## Consequences

Render and sync stay free of presentation; they keep producing data the same
way. `doctor` stops being a string-builder and exposes a structured report,
which makes it easier to extend with new project-status fields later (drift
counts, last-applied timestamp, etc.) without touching its callers.

Error rendering becomes consistent. The TUI receives either a single typed
error or a `Join`ed bundle, and renders both shapes with the same
icon-plus-color treatment. The "first error wins" surprises in render and
app go away.

The cost is one new file per domain package (`errors.go`) and a small extra
indirection at TUI call sites — they now pass the error to `RenderError`
instead of letting `fmt.Printf("error: %v\n", err)` handle it. That cost
buys testability: the TUI rendering layer is unit-tested with stripped
ANSI styling, and domain tests assert on typed errors with `errors.As`
rather than on string substrings.

## Revision history

- 2026-04-28 — initial decision; severity assigned by a TUI-side switch
  that introspected each concrete typed error; a `Severity()` method on
  domain errors was rejected as "presentation leaking back into the
  domain".
- 2026-04-28 (post-review) — reversed: severity is now a domain
  concern. Each typed error implements `Severity() errs.Severity`. The
  switch is gone; the TUI consumes the method directly. The previous
  framing conflated "presentation" with "presentation-relevant
  metadata"; severity is the latter, and centralizing it in the TUI
  meant every new typed error had to be registered there or it would
  silently render with the wrong icon. Domain ownership eliminates that
  failure mode.
