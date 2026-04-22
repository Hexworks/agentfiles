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

