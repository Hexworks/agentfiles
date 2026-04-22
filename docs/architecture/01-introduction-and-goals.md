# 1. Introduction And Goals

`agentfiles` is a local-first workspace manager for LLM tooling. It lets a user
define reusable assets inside profile folders, select those assets per project,
and then materialize agent-specific files into target repositories in a
controlled way.

The project exists to replace ad hoc and link-heavy workspace setups with a
clear source-of-truth model. Instead of treating project files as the primary
representation, `agentfiles` treats profile content as authoritative and generated
project files as outputs that can be planned, inspected, and applied.

## Primary Goals

### Safety

Generated files must be predictable. The user should see drift and recognized
deletion candidates before applying changes.

### Traceability

The system should make it obvious where generated files came from, which
profile/project produced them, and what changed between runs.

### Maintainability

The domain model should stay explicit: registry, profiles, assets, projects,
rendering, and sync are separate concerns with clear ownership.

### Git-Friendly Storage

Profiles must live in normal folders with normal files so they can be committed
and reviewed with standard Git workflows.

## Stakeholders

| Stakeholder | Expectation |
| --- | --- |
| Workspace maintainer | Manage reusable LLM setup assets in one place |
| Project maintainer | Apply only the assets needed for a specific repository |
| Profile owner | Keep profile data isolated, inspectable, and versionable |

