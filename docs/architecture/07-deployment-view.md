# 7. Deployment View

The current system is deployed only as a local developer tool. There is no
server-side runtime and no distributed deployment topology.

## Runtime Environment

- User workstation
- Go binary: `agentfiles`
- Local profile storage
- Local target repositories

## Relevant Locations

- Global registry: `~/.agentprofiles.json`
- Profile root: user-selected local path
- Managed project state: `<repo>/.agentfiles/state.json`
- Rendered agent files:
  - `AGENTS.md`
  - `.claude/`
  - `.cursor/`
  - `.codex/`
  - `.opencode/`
  - `.mcp.json`

## Deployment Characteristics

The deployment model is intentionally simple. The main architectural concern is
safe file synchronization on a local filesystem rather than service orchestration.

