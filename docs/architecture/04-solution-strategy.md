# 4. Solution Strategy

The architecture is intentionally simple: keep the domain model explicit, keep
storage file-based, and keep synchronization deterministic.

## Core Strategy

- Use profile folders as the authoritative source of truth
- Use a global registry file only for profile discovery and ownership lookup
- Express reusable content as typed assets
- Select assets per project with enabled-agent constraints
- Render canonical selections into known agent-specific file surfaces
- Plan before apply so the user can inspect changes

## Why This Shape

This strategy avoids the operational and conceptual complexity of a virtual
filesystem, live link management, or bidirectional synchronization. It also
fits the project's requirement that profiles should remain commit-friendly and
easy to reason about.

## Key Tradeoff

The current system prefers safety and transparency over live mutability.
Generated project files are easy to inspect and overwrite, but they are not yet
editable as first-class source artifacts.

