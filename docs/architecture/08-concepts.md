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

Domain packages declare typed error structs (one per failure mode) that
satisfy `errs.DomainError` (each implements `Error()` and
`Severity() errs.Severity`). Accumulator-shape functions return
`[]errs.DomainError` directly so the TUI can list every problem in one
go; non-accumulator functions wrap the slice in `errs.Errors` and
return a single `error`. The TUI dispatches on `err.Severity()` rather
than enumerating concrete types, so a new typed error picks up
icon + color automatically. The detailed convention lives in
[`docs/guidelines/errors.md`](../guidelines/errors.md).

