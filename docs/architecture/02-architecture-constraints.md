# 2. Architecture Constraints

The current implementation is shaped by a small set of explicit constraints.
These constraints are important because they limit design freedom and explain
why some simpler or more dynamic options were not chosen.

## Local-First Execution

`agentfiles` is a terminal UI application. It runs on a developer machine,
opens a full-screen TUI on launch, and operates directly on the local
filesystem. There is no non-interactive invocation path.

## Git-Friendly Persistence

Profiles are stored as plain folders with JSON manifests and asset files. There
is no database and no remote service in the current system.

## Known Managed Surfaces Only

Rendering is restricted to recognized agent file locations such as `AGENTS.md`,
`.claude/`, `.cursor/`, `.codex/`, `.opencode/`, and `.mcp.json`. Arbitrary
output paths are intentionally not part of the current safety model.

## Single Profile Ownership Per Project Path

A repository path may belong to only one profile. This avoids conflicting
ownership and ambiguous synchronization behavior.

## Generated Files Are Not The Source Of Truth

Project files are materialized outputs. The current implementation detects drift
but does not import project edits back into profile assets.

