# 0038 changes

Tightened the "register folder as asset" eligibility rule so nested and
above-root folders are no longer offered by the TUI or accepted by
`CreateAssetFromFolder`. Under the old rule, `app.RegisterableDirs`
returned every ancestor of an all-unknown subtree — for an unknown
`.claude/skills/foo/bar/baz.md` it offered `.claude`, `.claude/skills`,
`.claude/skills/foo`, and `.claude/skills/foo/bar` all as registerable.
Only the direct-child folder under a known asset-container root is a
sensible target for asset registration; everything else is either a
managed-surface root itself or a nested subpath of an already-registerable
folder.

The known asset-container roots (`.claude/skills`, `.codex/skills`,
`.opencode/skills`, `.cursor/commands`) previously lived in two places
inside `internal/render/render.go`: a function-local `skillRoots` map and
a hardcoded `.cursor/commands` string. Both moved to package scope, and
`render.AssetContainerRoots()` was added as the single exported source of
truth. `app.RegisterableDirs` now consumes it, so the two lists can no
longer drift.

## Decisions

- **Reuse `FolderNotRegisterableError` for nested `dirKey` rejection** —
  **Why:** the task explicitly directs no new error type; the existing
  guard at `service.go:270` already returns
  `FolderNotRegisterableError{DirKey: dirKey}` whenever `dirKey` is
  absent from `RegisterableDirs`, and under the tightened rule nested
  keys are naturally absent.
- **Centralize roots in `internal/render`, not `internal/surfaces`** —
  **Why:** the roots are a render-layer concern (they describe how skill
  assets project into agent-specific folders); `surfaces` already fences
  wider managed-surface writes and mixing the two concepts would blur
  the distinction. The task text also names `render` as the target.
- **`app → render` dependency is acceptable** — **Why:** `render` is a
  detail package with no upward imports; `sync` already imports it, so
  `app` importing it introduces no cycle and follows the outward
  direction (`app` orchestrates the detail layer).

Not done: no ADR was created — this bug fix tightens an existing domain
rule rather than making a new architectural decision. No new guideline
file was added — the change fits inside existing `clean_architecture` /
`errors` guidance.

## Assumptions

- **Ignore-flow (`assertIgnoredRegisterable`) inherits the tighter
  semantics** — **Why:** it shares `RegisterableDirs` with the register
  flow, and the task's redefinition is unconditional. Two existing tests
  (`TestAssertIgnoredRegisterable_ValidatesOnlyIncomingMinusPrior` and
  `TestActions_SyncProject_PersistsIgnoredPaths`) had to move their
  sample paths (`newdir/a.md`, `.codex/legacy`) under a container root to
  stay eligible. The behavior change is intentional: an ignored path
  outside a container root is no longer discoverable via the plan-project
  screen because the flow only surfaces asset-container children.

## Other Notes

- Render output is byte-identical after the refactor; only symbols moved
  from function-local to package-level.
- `AssetContainerRoots()` sorts its return value for deterministic
  iteration in the caller.
- Callers unchanged: `CreateAssetFromFolder` (service.go:270),
  `assertIgnoredRegisterable` (service.go:465), and the TUI plan-project
  screen (`plan_project.go:380`) all consume the same `map[string]bool`
  with tightened contents.

## Centralize asset-container roots in `internal/render`

Promoted the previously function-local `skillRoots` map and the hardcoded
`.cursor/commands` literal to package-level symbols, and exported
`AssetContainerRoots()` as the single source of truth for the
folder-registration eligibility gate.

```go
// before — inside addSkillOutputs
skillRoots := map[string]string{
    "codex":       ".codex/skills",
    "claude-code": ".claude/skills",
    "opencode":    ".opencode/skills",
}
// ...
if agent == "cursor" {
    target := filepath.ToSlash(filepath.Join(".cursor/commands", a.ID+".md"))
    // ...
}
```

```go
// after — package-scope, consumed by both render and app
var skillRoots = map[string]string{
    "codex":       ".codex/skills",
    "claude-code": ".claude/skills",
    "opencode":    ".opencode/skills",
}

const cursorCommandsRoot = ".cursor/commands"

// AssetContainerRoots returns the fixed set of managed-surface paths under
// which a direct child folder is eligible for asset registration.
func AssetContainerRoots() []string {
    out := make([]string, 0, len(skillRoots)+1)
    for _, root := range skillRoots {
        out = append(out, root)
    }
    out = append(out, cursorCommandsRoot)
    sort.Strings(out)
    return out
}
```

## Tighten `RegisterableDirs` to parent-must-be-root

Rewrote the eligibility loop to only count directories whose parent path
matches a known asset-container root. Ancestors above a root and
grandchildren below one are now silently skipped, so they never appear in
the returned set. Partly-managed folders remain excluded via the existing
`total == unknown` check.

```go
// before — every ancestor of an all-unknown subtree qualifies
func RegisterableDirs(changes []FileChange) map[string]bool {
    total := map[string]int{}
    unknown := map[string]int{}
    for _, ch := range changes {
        parts := strings.Split(ch.Path, "/")
        acc := ""
        for i := 0; i < len(parts)-1; i++ {
            if acc == "" {
                acc = parts[i]
            } else {
                acc = acc + "/" + parts[i]
            }
            total[acc]++
            if ch.Kind == ChangeUnknown {
                unknown[acc]++
            }
        }
    }
    // ...
}
```

```go
// after — only direct-child folders of a container root are counted
func RegisterableDirs(changes []FileChange) map[string]bool {
    rootList := render.AssetContainerRoots()
    roots := make(map[string]bool, len(rootList))
    for _, r := range rootList {
        roots[r] = true
    }
    total := map[string]int{}
    unknown := map[string]int{}
    for _, ch := range changes {
        parts := strings.Split(ch.Path, "/")
        for i := 0; i < len(parts)-1; i++ {
            parent := ""
            if i > 0 {
                parent = strings.Join(parts[:i], "/")
            }
            if !roots[parent] {
                continue
            }
            dir := strings.Join(parts[:i+1], "/")
            total[dir]++
            if ch.Kind == ChangeUnknown {
                unknown[dir]++
            }
        }
    }
    // ...
}
```

## Update `TestRegisterableDirs` to the acceptance table + add nested-rejection regression

Replaced the abstract `a`/`b`/`c/deep` fixture with `.claude/skills/...`
paths that exercise the acceptance-criteria table: direct-child folders
qualify, partly-managed and nested/root/outside folders do not. Added
`TestCreateAssetFromFolder_NestedFolderRejected` to lock in the
`FolderNotRegisterableError` guarantee for a nested `dirKey`.

```go
// before
func TestRegisterableDirs(t *testing.T) {
    changes := []FileChange{
        {Path: "a/x.md", Kind: ChangeUnknown},
        {Path: "a/y.md", Kind: ChangeUnknown},
        {Path: "b/z.md", Kind: ChangeUnknown},
        {Path: "b/w.md", Kind: ChangeCreate},
        {Path: "c/deep/i.md", Kind: ChangeUnknown},
    }
    got := RegisterableDirs(changes)
    for _, dir := range []string{"a", "c", "c/deep"} {
        if !got[dir] { t.Errorf("dir %q should be registerable", dir) }
    }
    if got["b"] {
        t.Errorf("dir %q has a managed leaf and must not be registerable", "b")
    }
}
```

```go
// after — table matches the acceptance criteria in description.md
func TestRegisterableDirs(t *testing.T) {
    changes := []FileChange{
        {Path: ".claude/skills/foo/SKILL.md", Kind: ChangeUnknown},
        {Path: ".claude/skills/foo/helper.md", Kind: ChangeUnknown},
        {Path: ".claude/skills/bar/SKILL.md", Kind: ChangeUnknown},
        {Path: ".claude/skills/bar/managed.md", Kind: ChangeCreate},
        {Path: ".claude/skills/nest/deep/f.md", Kind: ChangeUnknown},
        {Path: "docs/whatever/notes.md", Kind: ChangeUnknown},
    }
    got := RegisterableDirs(changes)
    for _, dir := range []string{".claude/skills/foo", ".claude/skills/nest"} {
        if !got[dir] { t.Errorf("dir %q should be registerable", dir) }
    }
    for _, dir := range []string{
        ".claude/skills/bar",
        ".claude/skills/nest/deep",
        ".claude/skills",
        ".claude",
        "docs/whatever",
        "docs",
    } {
        if got[dir] { t.Errorf("dir %q must NOT be registerable", dir) }
    }
}
```

## Migrate ignore-flow tests to container-root paths

`TestAssertIgnoredRegisterable_ValidatesOnlyIncomingMinusPrior` and
`TestActions_SyncProject_PersistsIgnoredPaths` both seeded sample paths
that had no container-root parent (`newdir/a.md`, `.codex/legacy`). Under
the tightened rule those keys are no longer registerable, so the tests
now seed paths under `.claude/skills/` and `.codex/skills/`. The
behavioral contract they pin (replace-semantics + state persistence) is
unchanged; only the sample data moved.
