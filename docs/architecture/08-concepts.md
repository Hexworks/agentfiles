# 8. Concepts

This section records cross-cutting concepts that shape multiple parts of the
system.

## Source Of Truth

Profile folders are authoritative. They contain manifests and reusable asset
content. Project files are outputs generated from profile selections.

## Managed Surfaces

The renderer and sync engine only work within recognized LLM-tooling paths.
This keeps synchronization predictable and reduces the risk of accidental file
writes outside the intended area.

## Compatibility And Exclusivity

Assets may declare `compatible_agents` to limit where they can render and
`exclusive_group` to prevent mutually incompatible selections from being used
together.

## Drift Detection

The sync layer stores hashes of managed files in `.agentfiles/state.json`. If a
managed file changes after apply, the next preview reports drift instead of
silently overwriting without explanation. Drift defaults to *keep*; the user
must explicitly resolve a drift entry to `ResolveOverwrite` to let apply
replace the local edits. See ADR 0010.

## First-Apply Clean Slate

A project with no `.agentfiles/state.json` is treated as fresh: every desired
file is classified as `ChangeCreate` (overwriting whatever happens to exist
at that path), and stray files in managed surfaces are ignored. The first
successful apply writes the initial state; subsequent plans then distinguish
drift from unknown normally. See ADR 0010.

## Safety-First Deletion

Files inside managed surfaces split into two classifications. Files recorded
in the previous `ManagedState` but missing from the new desired output
become `ChangeDelete` and are removed on apply — the user already opted in
to managing them. Files inside a managed surface that the engine has never
tracked become `ChangeUnknown`; apply leaves them alone unless the user
resolves them to `ResolveDelete`. Both flow through the preview so no write
is implicit.

## Rendering Lives In The TUI

Domain packages return data — `sync.Preview`, `RenderedFile`,
typed error structs. The TUI is the only layer that produces user-facing
text. This keeps stable policy independent of presentation, and makes
output styling (icons, colors, severity) testable in one place. See ADR
0007.

## Theming And Palette

All TUI color lives in a single semantic `Palette` (roles such as `Text`,
`Muted`, `Highlight`, not raw shades) in `internal/tui/styles`. Styles are
rebuilt from the active palette through one `Apply(Palette)` call rather
than frozen at package init, so the look is swappable in one place. An
optional `theme.json` (default `$XDG_CONFIG_HOME/agentfiles/theme.json`,
overridable with `--theme`) merges over the defaults at startup; a missing
file is a no-op and a malformed file aborts startup with a typed error.
Theming is purely presentational — no domain package imports `tui/styles`.
See ADR 0012.

## Typed Errors With Accumulation

Domain packages declare typed error structs that satisfy `errs.DomainError`
(`Error()` + `Severity()`). Accumulator-shape functions return
`[]errs.DomainError`; non-accumulator functions wrap the slice in
`errs.Errors`. The TUI dispatches on severity rather than concrete type.
Detail in [`docs/guidelines/errors.md`](../guidelines/errors.md).

## Security Boundary

Two safety nets bound what the system can write to. The managed-surfaces
fence in `internal/surfaces` refuses any projection outside the recognized
LLM-tooling paths. The TUI stripping helper `safe()` cleans ANSI control
sequences from any string interpolated into rendered output, so a hostile
profile name cannot rewrite the user's terminal. See
[`docs/guidelines/security.md`](../guidelines/security.md) and ADR 0007.

## Concurrency Model

The application is single-process and single-threaded by design: the TUI
event loop calls into the domain layer synchronously and awaits the
result. Plan-before-apply ordering is the primary safety mechanism, not
locking; section 6 shows the gating sequence. See
[`docs/guidelines/sync_and_safety.md`](../guidelines/sync_and_safety.md).

## Domain Model

The domain split is itself a crosscutting concern. Registry, profile,
asset, project, render, and sync remain decoupled, with stable identifiers
and explicit manifests rather than ad-hoc maps. See
[`docs/guidelines/domain_model.md`](../guidelines/domain_model.md).

## External Tool Handoff

Every shell-out to an outside program lives in a narrow wrapper
package: `internal/tui/editor` for the interactive `$VISUAL`/`$EDITOR`,
and `internal/git` for the `git` binary that records optional
[Commit Triggers](../glossary.md#commit-trigger). Both wrappers own
their `exec.Command` construction, argument quoting, and typed error
translation so callers never touch `os/exec` directly. See
[`docs/guidelines/external_tools.md`](../guidelines/external_tools.md)
and ADRs 0009 / 0019.

## Testing Strategy

Tests live next to the code they exercise and use table-driven cases with
typed-error assertions rather than message-string matching. See
[`docs/guidelines/testing.md`](../guidelines/testing.md).
