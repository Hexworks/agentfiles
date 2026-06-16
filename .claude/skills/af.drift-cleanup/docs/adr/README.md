# Architecture Decision Records

This folder contains Architecture Decision Records (ADRs) for `agentfiles`. ADRs
capture important architectural choices together with the context that made
those choices relevant and the consequences that follow from them.

## Naming

ADR files use a zero-padded numeric prefix followed by a lowercase kebab-case
slug, for example `0001-use-profile-based-source-of-truth.md`.

## Status Values

- `proposed`
- `accepted`
- `rejected`
- `deprecated`
- `superseded`

## Template

Each ADR uses a small Michael Nygard style structure:

- `# Title`
- `## Status`
- `## Context`
- `## Decision`
- `## Consequences`

## Index

- 0001 — Use profile-based source of truth
- 0002 — Materialize agent files into projects
- 0003 — Use global profile registry
- 0004 — Enforce single profile ownership per project path
- 0005 — TUI as the only user interface
- 0006 — Remove CLI subcommands in favor of pure TUI
- 0007 — Rendering belongs to the TUI
- 0008 — Domain error everywhere
- 0009 — Edit files via system editor
- 0010 — Sync resolutions and first apply
- 0011 — TUI screen router

