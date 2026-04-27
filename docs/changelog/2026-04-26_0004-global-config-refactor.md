# 0004 changes

Centralized hard-coded configuration values into a new `internal/config`
package and updated every domain package to read from it. The refactor
resolves all 13 `FIX: ... @see task#0004` markers without changing runtime
behavior.

The new package owns: profile/registry filenames (`profile.json`,
`asset.json`, `.agentprofiles.json`, `.agentfiles/state.json`), profile-root
subdirectory names (`assets`, `projects`), default `ProfileRef` metadata
(`Source = "local"`, `ManagedBy = "self"`), the asset-type subdirectory list,
asset starter filenames, and the managed-surface root list previously
duplicated between `internal/render` and `internal/sync`.

`render.allowedPrefixes` (slash-suffix form) and `sync`'s inline root list
(bare names) are now both replaced by `config.ManagedSurfaceRoots` (bare
names). `render.isAllowedTarget` adapts its match form to use
`target == root || strings.HasPrefix(target, root+"/")`, which is equivalent
to the previous slash-prefix check.

`sync.StatePath` was an exported package-level constant that no other package
imported; it is dropped in favor of `config.StateFileRelPath`.

## Decisions

- **One package, no subpackages.** — **Why:** the codebase is small and the
  values fit on one screen. Splitting into `config/profile`, `config/render`,
  etc. would add navigation cost without benefit.
- **Bare names (no trailing slash) for managed-surface roots.** — **Why:** the
  same list serves render's prefix check and sync's directory walk; bare
  names work for both with a single `+ "/"` adjustment in the matcher.
- **No ADR.** — **Why:** code-organization refactor with no rejected
  alternative or new architectural constraint, per
  `docs/guidelines/documentation.md`. Updated arc42 §5 to add the new package
  and updated `CLAUDE.md` invariant pointing at the consolidated location.
- **`AssetTypeDirs` duplicated as a string slice rather than reused from
  `asset.Type` constants.** — **Why:** `config` cannot import `asset` without
  creating a cycle (`asset` imports `config` for the manifest filename). The
  duplication is six strings and a comment marks the invariant.

## Assumptions

- **`sync.StatePath` had no external consumers.** — **Why:** `grep -rn
  sync.StatePath` returned zero hits across the repo, so dropping the
  exported constant in favor of `config.StateFileRelPath` is safe.
- **Test fixtures using literal `"asset.json"` are intentionally low-level.**
  — **Why:** they exercise on-disk shape, not the Go API. Switched only the
  test that referenced the now-removed `StatePath` symbol; left the rest as
  literals so the test still reads as a filesystem fixture.

## Other Notes

- arc42 §5 (`docs/architecture/05-building-block-view.md`) gained a new
  `config` block.
- `CLAUDE.md` package list, "Managed surfaces fence" invariant, and
  "Open refactor markers" section were updated to reflect the new home for
  these values.
- Task 0005 markers were left untouched — they cover error-type cleanup,
  which is a separate ticket.

## Change 1

New `internal/config` package.

```go
// before — values inline in each consumer
var allowedPrefixes = []string{
    "AGENTS.md", ".claude/", ".cursor/", ".codex/", ".opencode/", ".mcp.json",
}
```

```go
// after — single declaration in internal/config/config.go
var ManagedSurfaceRoots = []string{
    "AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json",
}
```

## Change 2

`render.isAllowedTarget` reads the shared list and matches with a slash-aware
check.

```go
// before
var allowedPrefixes = []string{"AGENTS.md", ".claude/", ".cursor/", ".codex/", ".opencode/", ".mcp.json"}

func isAllowedTarget(target string) bool {
    target = filepath.ToSlash(target)
    for _, prefix := range allowedPrefixes {
        if target == prefix || strings.HasPrefix(target, prefix) {
            return true
        }
    }
    return false
}
```

```go
// after — bare names from config; matcher appends "/" when checking prefix
func isAllowedTarget(target string) bool {
    target = filepath.ToSlash(target)
    for _, root := range config.ManagedSurfaceRoots {
        if target == root || strings.HasPrefix(target, root+"/") {
            return true
        }
    }
    return false
}
```

## Change 3

`sync.detectDeleteCandidates` walks the same shared list.

```go
// before
// FIX: task#0004
for _, root := range []string{"AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json"} {
    abs := filepath.Join(projectPath, root)
    ...
}
```

```go
// after — single source of truth
for _, root := range config.ManagedSurfaceRoots {
    abs := filepath.Join(projectPath, root)
    ...
}
```

## Change 4

`profile.Init` builds the asset subdir list from `config.AssetTypeDirs`
instead of inlining six paths.

```go
// before
for _, dir := range []string{
    filepath.Join(root, "assets", "skill"),
    filepath.Join(root, "assets", "agents_doc"),
    filepath.Join(root, "assets", "settings"),
    filepath.Join(root, "assets", "mcp"),
    filepath.Join(root, "assets", "rule"),
    filepath.Join(root, "assets", "hook"),
    filepath.Join(root, "projects"),
} {
    if err := fsutil.EnsureDir(dir); err != nil {
        return nil, err
    }
}
```

```go
// after — derived from config so adding an asset type only edits one file
dirs := make([]string, 0, len(config.AssetTypeDirs)+1)
for _, typeDir := range config.AssetTypeDirs {
    dirs = append(dirs, filepath.Join(root, config.AssetsDirName, typeDir))
}
dirs = append(dirs, filepath.Join(root, config.ProjectsDirName))
for _, dir := range dirs {
    if err := fsutil.EnsureDir(dir); err != nil {
        return nil, err
    }
}
```

## Change 5

`app.Service` profile-creation paths read default metadata from config.

```go
// before
ref := registry.ProfileRef{
    ID:   manifest.ID,
    Name: manifest.Name,
    Path: path,
    // FIX: task#0004 move these values to global config
    Source:       "local",
    ManagedBy:    "self",
    CreatedAt:    time.Now().UTC(),
    LastOpenedAt: time.Now().UTC(),
}
```

```go
// after — defaults centralized; touched once if convention changes
ref := registry.ProfileRef{
    ID:           manifest.ID,
    Name:         manifest.Name,
    Path:         path,
    Source:       config.DefaultProfileSource,
    ManagedBy:    config.DefaultProfileManagedBy,
    CreatedAt:    time.Now().UTC(),
    LastOpenedAt: time.Now().UTC(),
}
```

## Change 6

`sync.StatePath` removed; consumers (and the one `sync_test.go` reference)
switched to `config.StateFileRelPath`.

```go
// before
const StatePath = ".agentfiles/state.json"
...
return fsutil.WriteJSON(filepath.Join(preview.ProjectPath, StatePath), state)
```

```go
// after — owned by config so render and sync share one source for managed paths
return fsutil.WriteJSON(filepath.Join(preview.ProjectPath, config.StateFileRelPath), state)
```
