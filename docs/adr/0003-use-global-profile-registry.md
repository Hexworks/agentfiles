# Use Global Profile Registry

## Status

accepted

## Context

`agentfiles` needs a way to discover many profiles from CLI and TUI entry points
without scanning arbitrary filesystems every time. The application also needs a
stable place to store profile metadata such as id, name, path, source, and
last-opened time.

## Decision

Store profile references in a global registry file at `~/.agentprofiles.json`.
Commands resolve profiles by id, name, or path through this registry.

## Consequences

Profile discovery becomes fast and explicit. The TUI can present a stable list
of known profiles. The tradeoff is one more piece of global state that must stay
consistent with the actual profile folders.

