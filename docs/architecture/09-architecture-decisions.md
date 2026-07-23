# 9. Architecture Decisions

The current implementation already reflects several significant decisions. Their
durable rationale is recorded as ADRs in [`../adr/`](../adr/README.md).

## Initial Decision Set

- `0001`: use profile-based source of truth
- `0002`: materialize agent files into projects
- `0003`: use a global profile registry
- `0004`: enforce single-profile ownership per project path
- `0005`: use the TUI as the only user interface — superseded by `0006`
- `0006`: remove CLI subcommands in favor of pure TUI
- `0007`: rendering belongs to the TUI
- `0010`: sync resolution model and first-apply clean slate
- `0012`: palette-driven theming with external configuration
- `0013`: help and notifications as shell-owned modals
- `0014`: project scripting with single-file Go via gorun
- `0015`: drift-keep preserves the baseline; Adopt is the explicit promote
- `0016`: Definition-of-Done gate for the task workflow
- `0017`: split projects out of the profile folder
- `0018`: license under AGPL-3.0 with copyright-assignment CLA
- `0019`: optional git-aware auto-commits for profile and target repos

## Usage

This chapter is a guide to the ADR set, not a replacement for it. When a new
architectural decision is made, add a new ADR and update this page only if the
summary set of important decisions has changed.
