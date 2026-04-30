# 2. Architecture Constraints

The current implementation is shaped by a small set of explicit constraints.
These constraints limit design freedom and explain why some simpler or more
dynamic options were not chosen.

## Technical

### Local-First Execution

`agentfiles` is a terminal UI application that runs on a developer machine
and operates directly on the local filesystem. No remote service, no
background daemon. See section 7 for the deployment picture.

### Git-Friendly Persistence

Profiles are stored as plain folders with JSON manifests and asset files.
No database is used; section 3 covers the persistence layout.

### Known Managed Surfaces Only

Rendering is restricted to recognized agent file locations such as
`AGENTS.md`, `.claude/`, `.cursor/`, `.codex/`, `.opencode/`, and
`.mcp.json`. Arbitrary output paths are intentionally not part of the
current safety model.

## Organizational

### Single Profile Ownership Per Project Path

A repository path may belong to only one profile. This avoids conflicting
ownership and ambiguous synchronization behavior. The constraint is
enforced at project registration time.

### Generated Files Are Not The Source Of Truth

Project files are materialized outputs. The implementation detects drift
but does not import project edits back into profile assets.

## Conventions

### TUI-Only Entry Point

There is no non-interactive invocation path. Every command flows through
the TUI menu. The rationale and consequences are recorded in ADR 0006.
