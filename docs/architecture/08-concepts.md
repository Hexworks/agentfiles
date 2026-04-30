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
silently overwriting without explanation.

## Safety-First Deletion

Recognized but currently undesired LLM files are surfaced as delete candidates.
Deletion is explicit and opt-in rather than automatic.

## Rendering Lives In The TUI

Domain packages return data — `sync.Preview`, `doctor.Report`, `RenderedFile`,
typed error structs. The TUI is the only layer that produces user-facing
text. This keeps stable policy independent of presentation, and makes
output styling (icons, colors, severity) testable in one place. See ADR
0007.

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

## Testing Strategy

Tests live next to the code they exercise and use table-driven cases with
typed-error assertions rather than message-string matching. See
[`docs/guidelines/testing.md`](../guidelines/testing.md).
