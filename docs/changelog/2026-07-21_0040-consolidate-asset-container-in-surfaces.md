# 0040 changes

Consolidated the asset-container-root data and the folder-registration
eligibility predicate in `internal/surfaces`. Changelog 0038 had chosen
to keep `AssetContainerRoots()` inside `internal/render` and let `app`
import `render` to enforce the eligibility rule. This entry reverses
that placement decision: the roots, the per-agent `SkillRoot` map,
`CursorCommandsRoot`, the `RegisterableFolders` predicate, and a new
`ClassifyFolderRejection` helper now all live in `surfaces`, next to
the outer managed-surface fence they refine. `app.RegisterableDirs`
becomes a thin `[]FileChange → []surfaces.Leaf` adapter, dropping the
`app → render` import edge.

The rejection error gains a `Reason` field
(`ReasonNotUnderContainerRoot` / `ReasonHasManagedDescendants` /
`ReasonAbsentFromPlan`) so the TUI can render a targeted message per
rejection mode instead of a single generic one.

## Decisions

- **Move to `surfaces`, reversing 0038's `render` placement.** —
  **Why:** 0038 accepted an `app → render` import edge to consume
  `render.AssetContainerRoots()`. That edge crossed a domain seam
  (`app` is orchestration, `render` is a detail package for computing
  the desired file set) and existed only to import a data constant
  and a predicate. `surfaces` already owns the outer managed-surface
  fence and its matcher; placing the tighter inner fence alongside
  keeps both halves of the safety fence in one package and lets `app`
  reach both through a single import.
- **Emit a typed `FolderRejectionReason` at the service boundary.** —
  **Why:** the register-folder flow can fail for three distinct
  reasons — the folder's parent is not a container root, it is a root
  but contains at least one managed leaf, or it is legitimately
  registerable but the caller's `dirKey` is stale relative to the just
  computed plan. A single opaque `FolderNotRegisterableError` forced
  the TUI to guess; three sentinel values let each rejection surface a
  distinct, actionable message.
- **Keep `app.RegisterableDirs` as a thin adapter.** — **Why:** the TUI
  and existing tests speak `FileChange`/`ChangeKind`. Repointing every
  caller at `surfaces.Leaf` would balloon the diff and leak the leaf
  vocabulary into orchestration. The adapter compiles the two-field
  conversion (`Path`, `IsUnknown`) in one place.
- **Memoize the container-root slice and set with `sync.OnceValue`.**
  — **Why:** both `AssetContainerRoots` and `IsAssetContainerRoot` are
  called on every plan; the underlying data is package-scope constants
  that never change. `sync.OnceValue` gives the O(1) lookup for the
  predicate without letting the caller mutate the shared slice.

Considered but rejected:

- Keeping the roots in `render` and duplicating the predicate in
  `surfaces` — leaves the drift risk 0038 was already trying to close.
- Introducing a new package (`assetsurface` / `registration`) — creates
  a third fence-adjacent domain, splits ownership, and adds an import
  hop without new responsibility.
- Passing the rejection reason as a plain `string` — loses the compile
  time exhaustiveness the TUI needs to render a targeted message per
  case.

## Assumptions

- No caller reads the container-root list on a hot loop, so the
  `sync.OnceValue` initialization cost is amortized over the whole
  process lifetime.
- The three rejection reasons cover every path through
  `RegisterableFolders`. If a fourth semantics is added, both the
  sentinel set and the TUI branch table must be updated together;
  the current test suite in `internal/surfaces` enumerates all three.

## Other Notes

- Render output is byte-identical after the refactor; only symbols
  moved from `render`-scoped to `surfaces`-scoped.
- Glossary gained two entries: `Asset Container Root` (definition of
  the inner fence) and `Registerable Folder` (the eligibility rule
  restated in domain terms). The `Managed Surfaces` entry now links to
  both.
- No ADR was created for this refactor — it moves symbols across a
  seam already justified by ADR 0002 (managed surfaces fence) and
  refines the 0038 decision rather than introducing a new architectural
  choice. The reversal is captured here in the Decisions block above.

Docs updated: glossary (`Asset Container Root`, `Registerable Folder`,
extended `Managed Surfaces`), arc42 §5 building block view
(expanded `surfaces` responsibilities), CLAUDE.md (package list line
for `surfaces`).

## Move `AssetContainerRoots` + friends from `render` to `surfaces`

Promoted the previously `render`-scoped `skillRoots` map,
`cursorCommandsRoot` constant, and `AssetContainerRoots()` function
into `surfaces`, and exported `SkillRoot(agent)` and
`CursorCommandsRoot()` accessors so `render.addSkillOutputs` reads its
container roots from the same source `app.RegisterableDirs` does.

```go
// before — internal/render/render.go
var skillRoots = map[string]string{
    "codex":       ".codex/skills",
    "claude-code": ".claude/skills",
    "opencode":    ".opencode/skills",
}

const cursorCommandsRoot = ".cursor/commands"

func AssetContainerRoots() []string { /* sort + return */ }
```

```go
// after — internal/surfaces/surfaces.go
var skillContainerRoots = map[string]string{
    "codex":       ".codex/skills",
    "claude-code": ".claude/skills",
    "opencode":    ".opencode/skills",
}

const cursorCommandsRoot = ".cursor/commands"

func SkillRoot(agent string) (string, bool) {
    root, ok := skillContainerRoots[agent]
    return root, ok
}

func CursorCommandsRoot() string { return cursorCommandsRoot }

var assetContainerRootsSlice = sync.OnceValue(func() []string { /* ... */ })
var assetContainerRootSet    = sync.OnceValue(func() map[string]struct{} { /* ... */ })

func AssetContainerRoots() []string   { return slices.Clone(assetContainerRootsSlice()) }
func IsAssetContainerRoot(p string) bool {
    _, ok := assetContainerRootSet()[p]
    return ok
}
```

## Move `RegisterableFolders` + `ClassifyFolderRejection` into `surfaces`

The eligibility predicate travels with the roots so the two halves of
the inner fence live together. `app.RegisterableDirs` is now the thin
`FileChange → Leaf` adapter.

```go
// before — internal/app/service.go
func RegisterableDirs(changes []FileChange) map[string]bool {
    rootList := render.AssetContainerRoots()
    roots := make(map[string]bool, len(rootList))
    for _, r := range rootList { roots[r] = true }
    total := map[string]int{}
    unknown := map[string]int{}
    for _, ch := range changes {
        parts := strings.Split(ch.Path, "/")
        for i := 0; i < len(parts)-1; i++ {
            parent := ""
            if i > 0 { parent = strings.Join(parts[:i], "/") }
            if !roots[parent] { continue }
            dir := strings.Join(parts[:i+1], "/")
            total[dir]++
            if ch.Kind == ChangeUnknown { unknown[dir]++ }
        }
    }
    out := map[string]bool{}
    for dir, n := range total {
        if n > 0 && unknown[dir] == n { out[dir] = true }
    }
    return out
}
```

```go
// after — internal/app/service.go
func RegisterableDirs(changes []FileChange) map[string]bool {
    return surfaces.RegisterableFolders(leavesFromChanges(changes))
}

func leavesFromChanges(changes []FileChange) []surfaces.Leaf {
    leaves := make([]surfaces.Leaf, len(changes))
    for i, ch := range changes {
        leaves[i] = surfaces.Leaf{Path: ch.Path, IsUnknown: ch.Kind == ChangeUnknown}
    }
    return leaves
}
```

## Add `Reason` field to `FolderNotRegisterableError`

Every emission site now attaches the classified reason so the TUI can
render a targeted message per case.

```go
// before
type FolderNotRegisterableError struct{ DirKey string }
func (e FolderNotRegisterableError) Error() string {
    return fmt.Sprintf("folder %q is not registerable", e.DirKey)
}
```

```go
// after
type FolderNotRegisterableError struct {
    DirKey string
    Reason surfaces.FolderRejectionReason
}

func (e FolderNotRegisterableError) Error() string {
    switch e.Reason {
    case surfaces.ReasonNotUnderContainerRoot:
        return fmt.Sprintf("folder %q is not under a known asset-container root", e.DirKey)
    case surfaces.ReasonHasManagedDescendants:
        return fmt.Sprintf("folder %q contains managed files and cannot be registered as a new asset", e.DirKey)
    case surfaces.ReasonAbsentFromPlan:
        return fmt.Sprintf("folder %q is not present as an unknown-only leaf in the current plan", e.DirKey)
    default:
        return fmt.Sprintf("folder %q is not registerable", e.DirKey)
    }
}
```

Both call sites (`Service.CreateAssetFromFolder` at `service.go:256`
and `Service.assertIgnoredRegisterable` at `service.go:455`) populate
`Reason` via `surfaces.ClassifyFolderRejection(dirKey, leaves)`.

## Add container-root + rejection-mode tests in `surfaces`

Direct tests on `AssetContainerRoots`, `IsAssetContainerRoot`,
`RegisterableFolders`, and `ClassifyFolderRejection` moved into
`internal/surfaces/surfaces_test.go`. The service-boundary
regressions in `internal/app/service_create_asset_from_folder_test.go`
now exercise the three rejection modes explicitly, not just the
happy-path guard.
