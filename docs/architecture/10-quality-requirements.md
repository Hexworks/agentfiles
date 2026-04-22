# 10. Quality Requirements

The most important non-functional requirements in the current system are safety,
clarity, and extensibility.

## Safety Scenario

When a managed repository contains locally changed generated files, the system
should show those files as drift in the preview before any overwrite occurs.

## Traceability Scenario

When a user applies changes to a project, the system should persist enough state
to identify which profile and project generated the current managed files.

## Extensibility Scenario

When a new asset type or new agent-specific projection rule is introduced, the
change should fit into the existing domain model without collapsing the
separation between registry, profiles, assets, rendering, and sync.

## Usability Scenario

When a user wants to inspect pending changes quickly, the project plan/apply
TUI flows should show a concise preview of creates, updates, drift, and
delete candidates immediately after the project is selected.

