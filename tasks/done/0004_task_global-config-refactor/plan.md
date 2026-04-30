# Plan — task#0004: Centralize global configuration

Cross-links:

- Task: [./description.md](./description.md)
- Architecture (updated): [../../../docs/architecture/05-building-block-view.md](../../../docs/architecture/05-building-block-view.md)

## Context

Configuration values are scattered across `internal/registry`, `internal/profile`, `internal/asset`, `internal/render`, `internal/sync`, and `internal/app`. They include filenames (`profile.json`, `asset.json`, `.agentprofiles.json`), directory names (`assets`, `projects`), the managed-surface root list (duplicated between `render` and `sync`), default `ProfileRef` values (`"local"`, `"self"`), and asset-scaffolding starter filenames. Each duplicates intent in multiple files, so a behavior change requires hunting through unrelated packages. The codebase already has 13 `FIX: ... @see task#0004` markers that flag the affected sites.

This refactor introduces a single `internal/config` package that owns these values and rewrites the call sites to read from it. No behavior changes; this is a pure consolidation.

## Scope

In scope (all current `FIX: task#0004` markers):

| Site | What moves |
| --- | --- |
| `registry/registry.go:48` | Registry filename `.agentprofiles.json` |
| `profile/profile.go:54` | Asset-type subdir list + `projects` dir name |
| `profile/profile.go:68,83` | `profile.json` filename |
| `profile/profile.go:104,117` | `assets` dir, `asset.json` filename |
| `profile/profile.go:136` | `projects` dir name |
| `profile/profile.go:198` | Default profile slug fallback `"profile"` |
| `asset/asset.go:103-138` | `assets` dir, type subdir, `asset.json`, `SKILL.md`, `AGENTS.md`, `codex.toml`, `.keep` |
| `asset/asset.go:162` | `asset.json` |
| `render/render.go:39` | `allowedPrefixes` |
| `app/service.go:54,78` | `Source: "local"`, `ManagedBy: "self"` |
| `sync/sync.go:193` | Managed-surface root list (duplicate of render's `allowedPrefixes`) |
| `sync/sync.go:22` | `StatePath` (already a const, but moves to `config` so all paths live together) |

Out of scope (not flagged with task#0004):

- `render/render.go:127-135` settings agent → file mapping
- `render/render.go:222-247` skill output target paths
- `sync/sync.go:26` `GeneratorVersion` (lifecycle metadata, not configuration)
- All `task#0005` markers (separate ticket)

## Design

New package `internal/config` (no internal deps — sits at the bottom of the import graph):

```go
package config

const (
    RegistryFileName        = ".agentprofiles.json"
    ProfileManifestFileName = "profile.json"
    AssetManifestFileName   = "asset.json"
    StateFileRelPath        = ".agentfiles/state.json"
)

const (
    AssetsDirName   = "assets"
    ProjectsDirName = "projects"
)

const (
    DefaultProfileSource    = "local"
    DefaultProfileManagedBy = "self"
    DefaultProfileSlug      = "profile"
)

const (
    SkillStarterFileName     = "SKILL.md"
    AgentsDocStarterFileName = "AGENTS.md"
    SettingsStarterFileName  = "codex.toml"
    EmptyDirSentinel         = ".keep"
)

var AssetTypeDirs = []string{
    "skill", "agents_doc", "settings", "mcp", "rule", "hook",
}

var ManagedSurfaceRoots = []string{
    "AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json",
}
```

Notes:

- Single source for managed-surface roots — bare names (no trailing slash). `render.isAllowedTarget` adapts to use `target == root || strings.HasPrefix(target, root+"/")` which is equivalent to today's mixed `.claude/`-prefix form.
- `var` for slices (Go forbids const slices). Comment marks them as read-only.
- `AssetTypeDirs` overlaps semantically with `asset.Type` constants. Keeping them separate avoids a circular import. Comment flags the invariant.

## Step-by-step

1. Create `internal/config/config.go`.
2. Refactor `internal/registry/registry.go` to use config.
3. Refactor `internal/profile/profile.go` to use config.
4. Refactor `internal/asset/asset.go` to use config.
5. Refactor `internal/render/render.go` (rewrite `isAllowedTarget`).
6. Refactor `internal/sync/sync.go` (replace local `StatePath`, walk `config.ManagedSurfaceRoots`).
7. Refactor `internal/app/service.go`.
8. Update arc42 `docs/architecture/05-building-block-view.md` — add `config` block.
9. Update `CLAUDE.md` — drop task#0004 from "Open refactor markers".
10. Run `make fmt && make lint && make test`.
11. Set task status `in-review`.
12. Write `docs/changelog/{YYYY-MM-DD}_0004-global-config-refactor.md`.

## Verification

- `make lint` passes.
- `make test` passes — behavior unchanged.
- `grep -rn "task#0004" internal/` returns zero matches.
- `grep -rn '"profile.json"\|"asset.json"\|".agentprofiles.json"' internal/` returns matches only inside `internal/config/config.go`.

## Risks

- Test references to literal strings — switch them to `config.*` only if they reference the consolidated values.
- Import cycle — avoided; `config` has no internal deps.
- Duplicate root-list semantics — render's slash-prefix form merges with sync's bare form via the new matcher; behavior unchanged for unexported `isAllowedTarget`.

## Docs / ADRs / Guidelines

- Architecture: arc42 §5 building-block view updated.
- ADR: none. Code-organization refactor, not a durable architectural decision.
- Guidelines: no change.
- Changelog: `docs/changelog/{YYYY-MM-DD}_0004-global-config-refactor.md`.
