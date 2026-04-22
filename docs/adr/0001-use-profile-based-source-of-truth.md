# Use Profile-Based Source Of Truth

## Status

accepted

## Context

`agentfiles` manages reusable LLM workspace assets and synchronizes them into
target repositories. The system needs one authoritative place to store those
assets and their project selections.

Using generated project files as the primary source would make ownership,
composition, and reuse harder. It would also blur the boundary between user
authored source content and synchronized outputs.

## Decision

Use profile folders as the source of truth. A profile contains `profile.json`,
asset directories, and project manifests. Project files are treated as rendered
artifacts derived from those profile assets.

## Consequences

Profile state stays Git-friendly and reusable across projects. Rendering and
synchronization become easier to reason about. The tradeoff is that project-side
edits are treated as drift and are not imported back automatically.

