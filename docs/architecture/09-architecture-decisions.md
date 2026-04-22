# 9. Architecture Decisions

The current implementation already reflects several significant decisions. Their
durable rationale is recorded as ADRs in [`../adr/`](../adr/README.md).

## Initial Decision Set

- `0001`: use profile-based source of truth
- `0002`: materialize agent files into projects
- `0003`: use a global profile registry
- `0004`: enforce single-profile ownership per project path
- `0005`: use the TUI as the only user interface (superseded by `0006`)
- `0006`: remove CLI subcommands in favor of pure TUI

## Usage

This chapter is a guide to the ADR set, not a replacement for it. When a new
architectural decision is made, add a new ADR and update this page only if the
summary set of important decisions has changed.

