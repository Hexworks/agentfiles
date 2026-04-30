# 4. Solution Strategy

The architecture is intentionally simple: keep the domain model explicit, keep
storage file-based, and keep synchronization deterministic.

## Decomposition Pattern

The system is decomposed into small Go packages, one per domain concept,
arranged so dependencies flow from delivery (the TUI) inward to the domain
(profile, asset, project, registry). Packages near the bottom of the import
graph (`config`, `errs`, `surfaces`, `fsutil`) hold values and helpers used
by the rest. The pattern is captured in `docs/guidelines/clean_architecture.md`
and made visible in section 5.

## Quality Goal Mapping

Each fundamental decision serves at least one of the prioritized goals from
section 1.

| Quality goal         | Strategy                                                               | Reference                  |
| -------------------- | ---------------------------------------------------------------------- | -------------------------- |
| Safety               | Plan before apply; preview reports drift and delete candidates         | §6 Apply scenario, ADR 0007 |
| Safety               | Render refuses targets outside the managed surfaces fence              | §5 `surfaces` package       |
| Traceability         | `.agentfiles/state.json` records hashes of every managed file          | §8 Drift Detection          |
| Traceability         | Single profile owns each project path                                  | ADR 0004                    |
| Maintainability      | Profile folders are the source of truth; project files are outputs     | ADR 0001                    |
| Maintainability      | Domain packages stay separable; render is read-only, sync writes       | §5 building blocks          |
| Git-Friendly Storage | Profiles are plain folders with JSON manifests                         | §3 Technical Context        |

## Why This Shape

This strategy avoids the operational and conceptual complexity of a virtual
filesystem, live link management, or bidirectional synchronization. It also
fits the requirement that profiles should remain commit-friendly and easy to
reason about.

## Key Tradeoff

The system prefers safety and transparency over live mutability. Generated
project files are easy to inspect and overwrite, but they are not yet
editable as first-class source artifacts.
