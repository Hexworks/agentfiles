# 1. Introduction And Goals

`agentfiles` is a local-first workspace manager for LLM tooling. It lets a user
define reusable assets inside profile folders, select those assets per project,
and then materialize agent-specific files into target repositories in a
controlled way.

The project exists to replace ad hoc and link-heavy workspace setups with a
clear source-of-truth model. Instead of treating project files as the primary
representation, `agentfiles` treats profile content as authoritative and generated
project files as outputs that can be planned, inspected, and applied.

## Quality Goals

The four most important quality attributes are listed in priority order. Each
maps to a measurable scenario in [section 10](./10-quality-requirements.md).

| Priority | Goal                  | Concrete expectation                                                                                          |
| -------- | --------------------- | ------------------------------------------------------------------------------------------------------------- |
| 1        | Safety                | Drift and recognized deletion candidates appear in the preview before any overwrite happens.                  |
| 2        | Traceability          | After every apply, managed state records which profile and project produced each managed file.                |
| 3        | Maintainability       | Registry, profile, asset, project, render, and sync remain separable; new asset types fit without collapsing them. |
| 4        | Git-Friendly Storage  | Profiles are plain folders with JSON manifests; standard Git workflows can review and version them.           |

## Stakeholders

| Stakeholder | Expectation |
| --- | --- |
| Workspace maintainer | Manage reusable LLM setup assets in one place |
| Project maintainer | Apply only the assets needed for a specific repository |
| Profile owner | Keep profile data isolated, inspectable, and versionable |
| Agent operator | Consume the rendered AGENTS.md, `.claude/`, `.cursor/`, `.codex/`, and `.opencode/` outputs in their tool of choice |
