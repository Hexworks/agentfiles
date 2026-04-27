# global-config-refactor review

Task#0004 consolidated 13 hard-coded values into a new `internal/config` package and rewired six domain packages to read from it. The plan/changelog goals are met at the surface (`grep -rn task#0004 internal/` returns zero, all sites listed in the plan migrated, behavior preserved). Tests pass.

The review surfaced fourteen issues across seven topics. Three themes dominate:

1. **Stable-but-mutable globals** — `ManagedSurfaceRoots` and `AssetTypeDirs` are exported `var` slices. They are the safety fence the rest of the system relies on, but any package can `append` or reslice them at runtime. Five reviewers flagged this independently.
2. **`config` ownership boundaries** — most of the constants only have one consumer (e.g. `AssetManifestFileName` is only used by `asset` + `profile`; `StateFileRelPath` only by `sync`). Pushing them back into the owning aggregate would shrink `config` to the genuinely cross-cutting values (`ManagedSurfaceRoots`, `RegistryFileName`) and avoid the new package becoming a junk drawer.
3. **Refactor leftovers** — `internal/project/project.go:59` still inlines `"projects"`, `internal/render/render.go:109,113` still inlines `"AGENTS.md"`, and a stale `~/.llmprofiles.json` doc comment in `registry.go:21` was not refreshed despite the file being in the diff.

Plus one true security gap (`isAllowedTarget` accepts path traversal), missing boundary tests, naming/comment defects in `config.go`, and pre-existing CLAUDE.md drift the task chose not to fix.

Out-of-scope diff entries (skill files, `docs/guidelines/clean_*.md`, `docs/guidelines/git.md`, `docs/guidelines/security.md`, `.codex`, `.gitignore`, etc.) were not reviewed as part of this task; they are parallel branch work.

---

## Mutable exported safety-fence slices

> [!WARNING]
> [docs/guidelines/security.md](../docs/guidelines/security.md) — "Keep file access inside intended roots" / [docs/guidelines/clean_architecture.md](../docs/guidelines/clean_architecture.md) — Stable Abstractions / [docs/guidelines/solid.md](../docs/guidelines/solid.md) — DIP

`internal/config/config.go:41-48` and `:55-62` declare `AssetTypeDirs` and `ManagedSurfaceRoots` as exported package-level `var` slices. Go has no const slices, so a `var` is necessary, but the previous form (`render.allowedPrefixes` at the old `render/render.go:39`) was _unexported_ — tampering risk was confined to one package. Promoting the safety fence to an exported global widens the blast radius across the whole `internal/` tree and turns one of the project's named "Critical invariants" (CLAUDE.md:43) into a documentation contract rather than a code-enforced one.

Any package — at any time, from any goroutine, including tests with bad `init()` — can do `config.ManagedSurfaceRoots = append(config.ManagedSurfaceRoots, "/etc")`, `config.ManagedSurfaceRoots = nil`, or `config.ManagedSurfaceRoots[0] = "/"`. After that, `render.isAllowedTarget` (`internal/render/render.go:246`) admits arbitrary projection targets and `sync.detectDeleteCandidates` (`internal/sync/sync.go:190`) walks an attacker-chosen root for deletion candidates. The same applies to `AssetTypeDirs` — mutation makes `profile.Init` scaffold attacker-named directories.

```go
// internal/config/config.go:55
var ManagedSurfaceRoots = []string{
    "AGENTS.md", ".claude", ".cursor", ".codex", ".opencode", ".mcp.json",
}
// nothing prevents:
//   config.ManagedSurfaceRoots = append(config.ManagedSurfaceRoots, "..", "/")
// after which render.isAllowedTarget admits any target and
// sync.detectDeleteCandidates walks the entire repo.
```

Suggestions:

- [ ] Hide the slice and expose an accessor returning a fresh copy: `func ManagedSurfaceRoots() []string { return slices.Clone(managedSurfaceRoots) }` (and same for `AssetTypeDirs`). Update the four range-loop call sites.
- [x] Stronger: expose a predicate `func IsManagedSurface(target string) bool` so callers consume the rule, not the data; co-locates rule+data in one package (also fixes the DDD finding "ManagedSurfaceRoots torn from its rule" below).
- [ ] Use a fixed-size array `[6]string` so callers can range over it but cannot reassign or `append`; combined with an unexported var + a `[:]` accessor.
- [ ] Add a `TestManagedSurfaceRootsContract` lock test asserting exact contents and ordering — accidental mutation in build/init fails CI.

## `isAllowedTarget` accepts path-traversal targets

> [!WARNING]
> [docs/guidelines/security.md](../docs/guidelines/security.md) — "reject path traversal, absolute paths where relative paths are required, and paths that escape the expected root after evaluation"

The new matcher in `internal/render/render.go:246-254` is a small _tightening_ over the old form (the old `strings.HasPrefix(target, "AGENTS.md")` would have admitted `AGENTS.mdfoo`; the new `target == root || HasPrefix(target, root+"/")` rejects it). That is a security improvement. However, the matcher still does not clean the target before checking it, and the refactor is the moment the safety fence becomes a load-bearing exported API — worth closing the gap now.

A projection with `target = ".claude/../../../../etc/passwd"` passes `isAllowedTarget` (it starts with `.claude/`) and is then handed to `filepath.Join(projectPath, target)` in `sync.Apply` (`internal/sync/sync.go:137`). The project path is the trusted root, but `filepath.Join` followed by no `filepath.Rel`-based escape check means a hostile manifest can write outside the project. CLAUDE.md invariant #2 and the task's own framing claim "the safety fence enforces managed-output containment" — today it only enforces a prefix.

```go
// internal/render/render.go:246
func isAllowedTarget(target string) bool {
    target = filepath.ToSlash(target)
    for _, root := range config.ManagedSurfaceRoots {
        // ".claude/../../../etc/passwd" satisfies HasPrefix(target, ".claude/")
        // and is accepted, then filepath.Join writes outside the project root.
        if target == root || strings.HasPrefix(target, root+"/") {
            return true
        }
    }
    return false
}
```

Suggestions:

- [ ] In `isAllowedTarget`, reject targets containing `..` segments, leading `/`, or any volume prefix; or call `filepath.Clean` first and verify the cleaned form still has the same managed root and contains no `..`.
- [ ] In `sync.Apply` (`sync.go:137`), after `filepath.Join(projectPath, rel)` compute `filepath.Rel(projectPath, abs)` and refuse to write if the result starts with `..`.
- [ ] Both — defense in depth, since render is read-only and sync is the actual write path.
- [x] Out of scope; track as task#0007 and leave a `FIX:` marker in `isAllowedTarget`.

## Missing boundary tests for the rewritten `isAllowedTarget`

> [!WARNING]
> [docs/guidelines/testing.md](../docs/guidelines/testing.md) — "unit-test domain rules and pure transformations first"

The plan's Risks section explicitly states the matcher was rewritten and "behavior unchanged" — yet `internal/render/render_test.go` only exercises the happy-path `TestBuildSkillAndAgentsDoc`. There is no table-driven test that locks the boundary semantics of the new matcher, even though it gates the project's #2 critical invariant. A future edit to either the matcher form or `ManagedSurfaceRoots` could silently widen the fence with no test failing.

The new uniform matcher is _not_ exactly equivalent to the old one in one direction: the old form using bare `"AGENTS.md"` would have accepted `"AGENTS.mdfoo"` (substring prefix), while the new form rejects it. This is arguably a fix, but the only proof is human reading.

```go
// internal/render/render_test.go — missing
func TestIsAllowedTarget(t *testing.T) {
    cases := []struct {
        target string
        want   bool
    }{
        {"AGENTS.md", true},
        {"AGENTS.mdfoo", false},     // bare-name root must not act as substring prefix
        {".claude", true},
        {".claude/x", true},
        {".claudefoo/x", false},     // dir root must not match sibling dir
        {".mcp.json", true},
        {".claude/../etc/passwd", false}, // pairs with the path-traversal finding
        {"randomfile", false},
    }
    // ...
}
```

Suggestions:

- [x] Add `TestIsAllowedTarget` in `internal/render/render_test.go` with the table above.
- [ ] Reference `config.ManagedSurfaceRoots` in the test setup so adding a new root without a corresponding behavior expectation is at least visible.

## `.agentfiles` is outside the fence — dead skip checks in `detectDeleteCandidates`

> [!WARNING]
> [docs/guidelines/security.md](../docs/guidelines/security.md) — "store only the minimum state needed for drift detection"

`config.StateFileRelPath = ".agentfiles/state.json"` is treated as a managed location (sync writes it at `sync.go:160`, reads it at `:165`, and explicitly skips it during the delete-walk at `:201` and `:214`). But `.agentfiles` is deliberately _not_ in `ManagedSurfaceRoots`. Consequence: the skip checks at `sync.go:201,214` are dead defensive code — `detectDeleteCandidates` only walks under `ManagedSurfaceRoots`, which never enter `.agentfiles`. The state file cannot be flagged for deletion via the walk, only via the `state.ManagedFiles` loop (which never lists itself).

The exclusions are harmless today but signal to a future reader that `.agentfiles` is in the walk set when it is not — an invariant that breaks quietly if anyone adds `.agentfiles` to `ManagedSurfaceRoots` later, at which point the state file becomes droppable on every apply where `ManagedFiles` is empty.

Separately: the security guideline says "store only the minimum state needed" and "use restrictive permissions for files that may contain user-local data". `fsutil.WriteJSON` writes the state file with no explicit permission mode — worth verifying it produces 0600 rather than 0644.

```go
// internal/sync/sync.go:206-219 — walk never enters .agentfiles, so this is dead
err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
    if err != nil { return err }
    if d.IsDir() { return nil }
    rel := fsutil.ToRelative(projectPath, path)
    if rel == config.StateFileRelPath {  // dead branch
        return nil
    }
    ...
```

Suggestions:

- [ ] Drop the two `rel == config.StateFileRelPath` skips in `detectDeleteCandidates` and add a comment explaining the state file is structurally unreachable from this walk.
- [x] Add `.agentfiles` to `ManagedSurfaceRoots` and let the skip be the single source of truth — pick one model and document it.
- [ ] Verify (or add `os.Chmod` after) `fsutil.WriteJSON` writes the state file with 0600 permissions; add a test asserting the mode.

## `config` is becoming a god-package — six unrelated concerns share one package

> [!WARNING]
> [docs/guidelines/clean_architecture.md](../docs/guidelines/clean_architecture.md) — Common Closure / Common Reuse / [docs/guidelines/solid.md](../docs/guidelines/solid.md) — SRP + ISP

`internal/config/config.go:9-62` mixes six independent value clusters: persistence filenames, profile-root subdirectory names, default `ProfileRef` metadata, asset starter filenames, the asset-type list, and the managed-surface root list. Each has a different reason to change. The changelog acknowledges this in its "One package, no subpackages" decision but justifies it on size, not cohesion.

Concrete CRP problem: `internal/registry/registry.go` only needs `RegistryFileName` (one symbol) yet importing `config` exposes it transitively to `ManagedSurfaceRoots`, `AssetTypeDirs`, `DefaultProfileSlug`, etc. Symmetrically `render.go` only needs `ManagedSurfaceRoots` but sits one symbol-completion away from registry filenames and starter scaffolding paths. The package-level form of ISP applies: callers pay for the whole surface.

Concrete CCP problem: the task framed scattered constants as a CCP violation, but the package each constant _truly_ changes with is the package that owns the concept. `RegistryFileName` only ever changes when registry-on-disk format changes; pulling it into a generic `config` separates closure rather than reinforcing it.

```go
// internal/config/config.go — six unrelated change-axes
const RegistryFileName        = ".agentprofiles.json"   // registry concern
const ProfileManifestFileName = "profile.json"          // profile concern
const AssetManifestFileName   = "asset.json"            // asset concern
const StateFileRelPath        = ".agentfiles/state.json" // sync concern
const SkillStarterFileName    = "SKILL.md"              // asset-scaffolding concern
var   ManagedSurfaceRoots     = []string{...}           // render+sync (genuinely shared)
var   AssetTypeDirs           = []string{...}           // duplicate of asset.Type
```

Suggestions:

- [ ] Push single-consumer constants back to the owning aggregate: `RegistryFileName` → `registry`; `ProfileManifestFileName`/`AssetsDirName`/`ProjectsDirName` → `profile`; `AssetManifestFileName` + starter filenames + `EmptyDirSentinel` → `asset`; `StateFileRelPath` (or its split form) → `sync`; `DefaultProfile*` → `registry` (or expose `registry.NewLocalProfileRef(id, name, path)`). Keep only the genuinely cross-cutting `ManagedSurfaceRoots` (and `AssetTypeDirs` if it stays — see next finding) in `config`.
- [x] Split `config` into purpose-scoped sibling files (`config/paths.go`, `config/defaults.go`, `config/surfaces.go`) so each consumer imports only what it needs.
- [ ] Leave as-is — accept a shared "well-known names" pile because the codebase is small.

## `AssetTypeDirs` duplicates `asset.Type` — two parallel definitions of one domain set

> [!WARNING]
> [docs/guidelines/domain_model.md](../docs/guidelines/domain_model.md) — "Model Consistency Boundaries" / [docs/guidelines/clean_architecture.md](../docs/guidelines/clean_architecture.md) — REP / [docs/guidelines/solid.md](../docs/guidelines/solid.md) — OCP

The asset-type set is a domain concept already modeled in `internal/asset/asset.go:24-40` as typed `Type` constants and validated in `Manifest.Validate` at `:81-86`. The refactor introduces a _second_ authoritative list at `internal/config/config.go:41-48` with the same six strings, used only by `profile.Init`. The changelog acknowledges this as a deliberate cycle workaround ("`config` cannot import `asset` without creating a cycle"), but the cycle is itself a signal that the value belongs in `asset`, not in `config`.

The natural dependency direction is `profile → asset` (profile already imports asset at `profile/profile.go:14`). Exposing `asset.AllTypes() []Type` (or a package-level `var Types = []Type{...}`) and having `profile.Init` iterate it eliminates the duplication entirely with no cycle. Adding a seventh asset type then requires one edit in `asset` instead of synchronized edits in `asset.Type`, `asset.Validate`, `asset.Init` switch, and `config.AssetTypeDirs`.

The current form is the textbook needless-repetition smell: the comment "Must stay in sync with asset.Type constants" encodes the violation as a manual invariant with no compile-time check.

```go
// internal/asset/asset.go:24 — typed authoritative set
const (
    TypeSkill     Type = "skill"
    TypeAgentsDoc Type = "agents_doc"
    // ... 4 more
)

// internal/config/config.go:41 — second untyped copy, drift risk
var AssetTypeDirs = []string{
    "skill", "agents_doc", "settings", "mcp", "rule", "hook",
}
```

Suggestions:

- [x] Add `func AllTypes() []Type` (or `var Types = []Type{TypeSkill, ...}`) to `internal/asset/asset.go`. Drop `config.AssetTypeDirs`. Have `profile.Init` iterate `asset.AllTypes()` and call `string(t)` for the directory name.
- [ ] Move `asset.Type` constants down into `config` and let `asset` re-export. Eliminates the cycle from the other direction but loses typed `Type` distinction at the policy layer.
- [ ] Keep duplication, add a `TestAssetTypeDirsMatchesAssetTypes` test that fails when the two lists drift.

## `project.Save` still inlines `"projects"` — refactor leftover

> [!WARNING]
> [docs/guidelines/clean_code.md](../docs/guidelines/clean_code.md) — "Replace magic numbers and strings with named constants" / "Needless repetition"

The plan enumerates every site `task#0004` was meant to consolidate (plan §Scope) but `internal/project/project.go` is not in that table. So `project.Save` at `:59` still computes the path with literal `"projects"`. The string already lives in `config.ProjectsDirName` (`config.go:19`) and is used by `profile.Init` (`profile.go:59`) and `profile.scanProjects` (`profile.go:131`). If anyone changes the directory name in config, `profile` will read from the new location while `project.Save` keeps writing to the old one — silent split brain.

The plan's verification grep (`'"profile.json"\|"asset.json"\|".agentprofiles.json"'`) does not include `"projects"`, which is why the leftover slipped through.

```go
// internal/project/project.go:59
path := filepath.Join(profileRoot, "projects", manifest.ID+".json")
//                                  ^^^^^^^^^^ should be config.ProjectsDirName
```

Suggestions:

- [x] Add `config` import to `internal/project/project.go` and replace the literal with `config.ProjectsDirName`.
- [ ] Broaden the verification grep in the plan/CLAUDE.md to include `"projects"`, `"assets"`, `"SKILL.md"`, `"codex.toml"`, `"AGENTS.md"` so the next refactor catches all leftovers.

## `render.go` still hard-codes `"AGENTS.md"` despite owning a config constant

> [!WARNING]
> [docs/guidelines/clean_code.md](../docs/guidelines/clean_code.md) — "Needless repetition"

`internal/render/render.go:109` reads `filepath.Join(a.Dir, "AGENTS.md")` and `:113` writes to `files["AGENTS.md"] = RenderedFile{Path: "AGENTS.md", ...}` — three literals on two adjacent lines. The same string is `config.AgentsDocStarterFileName` (which `asset.Init` already uses to _create_ the file at `asset.go:123`). Two halves of the same flow (scaffold → render) refer to the same filename through two unconnected mechanisms; renaming the starter would silently corrupt rendering. The literal also appears in `config.ManagedSurfaceRoots` — three independent copies in the codebase. Same smell for `"SKILL.md"` at `render.go:191` (mirrors `config.SkillStarterFileName`).

```go
// internal/render/render.go:109-113
body, err := os.ReadFile(filepath.Join(a.Dir, "AGENTS.md"))
// ...
files["AGENTS.md"] = RenderedFile{Path: "AGENTS.md", Body: body, Mode: 0o644, Source: a.ID}
```

Suggestions:

- [x] Replace both literals at `render.go:109,113` with `config.AgentsDocStarterFileName`. Apply the same to `render.go:191` for `config.SkillStarterFileName`.
- [ ] Out of scope — plan listed `render.go:127-135` as out of scope and the agents_doc literal sits in the same area; track as a follow-up cleanup.

## `EmptyDirSentinel` is misleadingly named and grouped

> [!WARNING]
> [docs/guidelines/clean_code.md](../docs/guidelines/clean_code.md) — "Choose names that describe domain meaning, not just data shape"

`config.EmptyDirSentinel` (`config.go:35`) is consumed by `asset.Init` (`asset.go:131-134`) as the `default` branch when scaffolding asset types that have no dedicated starter file. Its actual role is "fallback starter file written so the asset directory is not empty" — not a sentinel in any technical sense (a sentinel implies a marker value the code later checks for; this file is never looked up again). Worse, the comment that introduces this constant block says "Asset starter filenames written by asset.Init when scaffolding a new asset" — which is exactly what `EmptyDirSentinel` _is_, contradicting the chosen name.

The misleading name also masks an existing flagged smell (`asset.go:130`: `// FIX: we don't want a default case, this should be an error instead`). If the default branch is removed, the constant disappears with it; "Sentinel" obscures that this is a stop-gap fallback rather than a deliberate file marker.

```go
// internal/config/config.go:35
EmptyDirSentinel = ".keep"

// internal/asset/asset.go:131-134
default:
    if err := os.WriteFile(filepath.Join(dir, config.EmptyDirSentinel), []byte{}, 0o644); err != nil {
        return "", err
    }
```

Suggestions:

- [ ] Rename to `FallbackStarterFileName` (or `PlaceholderStarterFileName`); move into the asset-starter group it actually belongs to.
- [x] Resolve the linked `FIX:` first by removing the default branch entirely (return an error for unknown types). If `mcp`/`rule`/`hook` should not produce a starter file, both the constant and the branch can disappear.

## `StateFileRelPath` mixes a directory and a filename

> [!WARNING]
> [docs/guidelines/go.md](../docs/guidelines/go.md) — "Keep packages cohesive" (consistent decomposition)

`config.go:13` declares `StateFileRelPath = ".agentfiles/state.json"` as a single concatenated string while every other path constant in the file is split: `AssetsDirName`/`ProjectsDirName` for dirs, `RegistryFileName`/`ProfileManifestFileName`/`AssetManifestFileName` for filenames. The composite form forces consumers in `sync.go:160,165,201,214` to compare `rel == config.StateFileRelPath` against a value that already contains a separator. On Windows, `filepath.Join` produces `.agentfiles\state.json` and the equality check against `.agentfiles/state.json` will fail. Today this happens to work because earlier code happens to `ToSlash`-normalize, but the brittleness is invisible.

```go
// current
StateFileRelPath = ".agentfiles/state.json"

// sync.go:201 — only works because rel was ToSlash-normalized somewhere upstream
if desired[rel] == "" && rel != config.StateFileRelPath { ... }
```

Suggestions:

- [x] Split into `StateDirName = ".agentfiles"` + `StateFileName = "state.json"`; update consumers to `filepath.Join(projectPath, config.StateDirName, config.StateFileName)`.
- [ ] Keep the composite, rename to `StateRelSlashPath` (or similar) to signal the slash is intentional, document the Windows behavior, ensure all comparison sites `filepath.ToSlash` first.

## `config.go` doc comments don't document non-obvious invariants

> [!WARNING]
> [docs/guidelines/clean_code.md](../docs/guidelines/clean_code.md) — "Comment non-obvious invariants, safety rules, and compatibility requirements"

Three invariants are encoded only as code, not as comments:

1. `ManagedSurfaceRoots` (`config.go:55`) — comment says "treat as read-only" but does not document that entries must be _bare_ names without trailing `/`. The plan called this out; `render.isAllowedTarget`'s `root+"/"` form depends on it; adding a slash silently breaks the matcher.
2. `StateFileRelPath` (`config.go:13`) — does not document that consumers compare on slash form (see Windows note above).
3. `DefaultProfileSource = "local"` and `DefaultProfileManagedBy = "self"` (`config.go:24-28`) — documented as "stamped onto a freshly registered ProfileRef" but the comment doesn't say what other values would be legal or why both fields exist. A maintainer reading the constants alone has no way to know whether changing `"self"` to `"agentfiles"` would break anything.

Separately, Go convention is that each exported identifier has its own doc comment beginning with the identifier name; the four `const ( ... )` groups in `config.go` only have group-level block comments. `go doc config.AssetsDirName` will render only the group comment.

```go
// internal/config/config.go:9-14 — group label, no per-identifier docs, no invariants
const (
    RegistryFileName        = ".agentprofiles.json"
    ProfileManifestFileName = "profile.json"
    AssetManifestFileName   = "asset.json"
    StateFileRelPath        = ".agentfiles/state.json" // contains "/", consumers compare on slash form
)
```

Suggestions:

- [ ] Add a "bare-name, no trailing slash" invariant comment to `ManagedSurfaceRoots`.
- [ ] Add a "stored slash-separated; compare via `filepath.ToSlash`" note to `StateFileRelPath` (or split per the previous finding).
- [ ] On the default-profile group, link `Source`/`ManagedBy` to the `registry.ProfileRef` fields they populate.
- [x] Add per-identifier `// Foo is …` comments at least for the symbols `go doc` users will reach for (`RegistryFileName`, `AssetsDirName`, `ManagedSurfaceRoots`, etc.).

## Test fixtures inconsistently mix `config.*` and literal paths

> [!WARNING]
> [docs/guidelines/testing.md](../docs/guidelines/testing.md) — "Be consistent. Similar concepts should look similar"

`internal/sync/sync_test.go:49` writes the state file via `filepath.Join(projectRoot, config.StateFileRelPath)` — but lines 21-37 still scaffold the asset tree using literal `"assets"`, `"agents_doc"`, `"asset.json"`, `"AGENTS.md"`, `".codex"`. The changelog explicitly justifies the literals as "intentionally low-level… exercise on-disk shape, not the Go API." That argument is reasonable in isolation, but if it applies to `"asset.json"` it applies equally to `".agentfiles/state.json"`. The single line that _was_ converted now stands out as inconsistent rather than principled.

Same pattern in `internal/render/render_test.go:18,22,28,32` (literal `"asset.json"` etc., file not modified by this task but covered by the same justification).

Beyond style: fixture-construction paths like `filepath.Join(profileRoot, "assets", "agents_doc", "base", "asset.json")` exist _because_ the test mimics what `profile.Init` and `asset.Init` would lay down. If someone renames `AssetManifestFileName` to `"manifest.json"` in config, production follows but the test keeps writing `"asset.json"` files, then `profile.Load` silently finds zero assets, and `if !sawDrift` may pass for the wrong reason (the asset is missing rather than mismatched).

```go
// internal/sync/sync_test.go:21-49 — literal paths everywhere except line 49
os.MkdirAll(filepath.Join(profileRoot, "assets", "agents_doc", "base"), 0o755)
os.WriteFile(filepath.Join(profileRoot, "assets", "agents_doc", "base", "asset.json"), ...)
os.WriteFile(filepath.Join(profileRoot, "assets", "agents_doc", "base", "AGENTS.md"), ...)
// ...
os.WriteFile(filepath.Join(projectRoot, config.StateFileRelPath), mustJSON(t, state), 0o644)
//                                       ^^^^^^^^^^^^^^^^^^^^^^^^ only constant in the file
```

Suggestions:

- [x] Replace fixture-construction literals (`"assets"`, `"asset.json"`) in `sync_test.go:21,24` and `render_test.go:18,22,28,32` with `config.AssetsDirName` and `config.AssetManifestFileName`. Keep target-side literals (`"AGENTS.md"`, `".codex/old.txt"`) — those _are_ the on-disk shape under test.
- [ ] Replace the manual `os.MkdirAll` + `os.WriteFile` ladder with an `asset.Init`-based helper so fixtures use the same code path as production scaffolding.
- [ ] Revert line 49 to literal `".agentfiles/state.json"` so the whole test reads as on-disk fixture (the inverse of the above).

## `ManagedSurfaceRoots` data lives in `config`, but the rule lives in `render`/`sync`

> [!WARNING]
> [docs/guidelines/domain_model.md](../docs/guidelines/domain_model.md) — "Put domain rules in domain code … keep managed-surface safety checks in the rendering and sync path"

Managed Surfaces is a first-class glossary term and one of the project's named invariants. The domain-model guideline explicitly says managed-surface checks belong in render and sync. The refactor moved the _data_ (`ManagedSurfaceRoots`) to `config`, but kept the _rule_ (`isAllowedTarget` in `render.go:246`, the delete-walk in `sync.go:190`) in the consuming packages. Worst of both worlds: a reader who sees `config.ManagedSurfaceRoots` cannot tell whether the safety fence is enforced at all, and `render.isAllowedTarget` and `sync`'s walk re-implement the same matcher logic against the same data — the duplication the refactor was supposed to remove.

A DDD-aligned fix would either (a) keep the slice in `render` and have `sync` import it from `render`, mirroring the actual rule flow, or (b) introduce a small `surfaces` package that owns both the list _and_ the matcher (`IsAllowed(target string) bool`, `Roots() []string`).

```go
// internal/config/config.go:55  — data only, no rule
var ManagedSurfaceRoots = []string{"AGENTS.md", ".claude", ...}

// internal/render/render.go:246  — render still owns the matcher
// internal/sync/sync.go (delete-walk) — re-walks the same roots without calling
// the matcher, so render and sync each carry half the rule.
```

Suggestions:

- [x] Move `ManagedSurfaceRoots` and `isAllowedTarget` into one new package (`surfaces`) that exposes `IsAllowed(target)` and `Roots() []string`. Have `sync` call `surfaces.IsAllowed`/`surfaces.Roots` instead of walking raw strings.
- [ ] Keep the slice in `config` but co-locate the predicate there too (`config.IsManagedTarget`) so the rule is not split from its data.
- [ ] Move the slice back to `render` and have `sync` import it from `render`.

## Pre-existing CLAUDE.md / `registry.go` doc drift not fixed alongside in-diff edits

> [!WARNING]
> [docs/guidelines/documentation.md](../docs/guidelines/documentation.md) — "document current reality, not planned state"

Two stale-docs items sit inside files this task already edited:

1. **CLAUDE.md:63-67** references five guideline files under their old names (`go_guidelines.md`, `domain_model_guidelines.md`, `sync_and_safety_guidelines.md`, `asset_authoring_guidelines.md`, `documentation_guidelines.md`); the current files are `go.md`, `domain_model.md`, `sync_and_safety.md`, `asset_authoring.md`, `documentation.md`. CLAUDE.md:57 also says "per go_guidelines". The rename happened in commit `ef3c365`, not this task — but this task touched CLAUDE.md (added the `config` bullet at :28, updated invariant #2 at :43, removed the task#0004 line from "Open refactor markers" at :55-58). The broken section sits four lines from the section that was edited.
2. **`internal/registry/registry.go:21`** says `// ProfileRef is the lightweight, global metadata stored in ~/.llmprofiles.json.` — actual file is `~/.agentprofiles.json` (now `config.RegistryFileName`). This task touched the same file (lines 14, 49) and is the natural point to refresh the comment, especially since the centralization is what makes this kind of stale doc trivially fixable in the future.

Additionally, the new `clean_architecture.md`, `clean_code.md`, `solid.md`, `testing.md`, `git.md`, `security.md` files exist under `docs/guidelines/` but are absent from CLAUDE.md's "Guidelines referenced from docs/" index.

```text
# CLAUDE.md:63-67 — five broken links the task left behind
- `docs/guidelines/go_guidelines.md`               # actual: go.md
- `docs/guidelines/domain_model_guidelines.md`     # actual: domain_model.md
- `docs/guidelines/sync_and_safety_guidelines.md`  # actual: sync_and_safety.md
- `docs/guidelines/asset_authoring_guidelines.md`  # actual: asset_authoring.md
- `docs/guidelines/documentation_guidelines.md`    # actual: documentation.md

// internal/registry/registry.go:21
// ProfileRef is the lightweight, global metadata stored in ~/.llmprofiles.json.
//                                                          ^^^^^^^^^^^^^^^^^ wrong; actually .agentprofiles.json
```

Suggestions:

- [ ] Fix the five filenames in CLAUDE.md:63-67 and the inline `per go_guidelines` reference at :57; add the new guideline files to the index.
- [ ] Fix the `~/.llmprofiles.json` doc comment at `registry.go:21` (preferred form: "the global registry file — see `config.RegistryFileName`" so it stays linked to the single source of truth).
- [x] Out of scope (pre-existing drift); leave for a separate docs-cleanup task, also create this cleanup task.

## `config` package name collides with user-facing notion of "configuration"

> [!WARNING]
> [docs/guidelines/domain_model.md](../docs/guidelines/domain_model.md) — "Use the project language" / "hide domain rules behind vague names like data, item, manager, or handler"

The glossary in `docs/glossary.md` enumerates the project's vocabulary (Registry, Profile, Asset, Managed Surfaces, Managed State, Drift, Preview) — "configuration" is not in it. In a project whose entire purpose is to manage _configuration files_ for AI assistants, the word is dangerously overloaded: inside the codebase "configuration" already means _the user-facing artifacts being rendered into target repos_ (`.claude/settings.local.json`, `codex.toml`, the entire `Settings` asset type). A package named `internal/config` colliding with that meaning will confuse anyone reading the import list.

The values it owns aren't "config" in the developer sense (no env-driven settings, no runtime knobs) — they're well-known names of the bounded context. A domain-flavored name (`wellknown`, `conventions`) would carry meaning.

Suggestions:

- [ ] Rename the package to `wellknown` (or `conventions`); update glossary if the term is durable.
- [ ] Rename to `paths` if the cross-cutting items end up being predominantly path-shaped.
- [ ] Eliminate the package entirely by pushing each value back to the aggregate it belongs to (the "split" suggestion in the god-package finding above) — removes the naming problem at the root.
- [x] Keep `config` — the duplication risk is small enough.
