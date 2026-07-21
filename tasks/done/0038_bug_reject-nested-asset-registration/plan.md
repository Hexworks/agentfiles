# Plan — Task 0038: Reject nested asset registration

Cross-links:
- Task description: [./description.md](./description.md)
- Sources: `internal/app/service.go`, `internal/render/render.go`
- Tests: `internal/app/service_create_asset_from_folder_test.go`
- Guidelines: `docs/guidelines/asset_authoring.md`, `docs/guidelines/errors.md`,
  `docs/guidelines/clean_architecture.md`, `docs/guidelines/clean_code.md`,
  `docs/guidelines/domain_model.md`, `docs/guidelines/solid.md`,
  `docs/guidelines/testing.md`

## Context

`app.RegisterableDirs` currently marks **every ancestor** of an all-unknown
subtree as registerable. For unknown `.claude/skills/foo/bar/baz.md` it
returns `.claude`, `.claude/skills`, `.claude/skills/foo`, and
`.claude/skills/foo/bar` — so the TUI's "register folder as asset" flow
offers nested and root-level folders that are not valid asset containers.

The chosen rule: a directory is registerable **iff** its parent path is a
known **asset-container root** AND every descendant leaf is unknown. Roots
are the four folder-shaped skill/command container dirs:
`.claude/skills`, `.codex/skills`, `.opencode/skills`, `.cursor/commands`.

The list is scattered in `internal/render/render.go` today (function-local
`skillRoots` map + hardcoded `.cursor/commands`). Centralize it as one
exported source of truth in `render` (`AssetContainerRoots()`) and have
`app.RegisterableDirs` consume it — so no duplicated list drifts. No new
error type: the existing `FolderNotRegisterableError` guard in
`CreateAssetFromFolder` (service.go:270) already rejects any `dirKey`
missing from the returned set.

## Files to edit

| File | Change |
| ---- | ------ |
| `internal/render/render.go` | Promote `skillRoots` map + `.cursor/commands` to package-level; add exported `AssetContainerRoots() []string`. `addSkillOutputs` consumes the promoted map + const — output unchanged. |
| `internal/app/service.go` | Rewrite `RegisterableDirs` to use `render.AssetContainerRoots()` and enforce the parent-must-be-root rule. Add `internal/render` import. |
| `internal/app/service_create_asset_from_folder_test.go` | Rewrite `TestRegisterableDirs` fixture to `.claude/skills/...` paths matching the acceptance table. Add `TestCreateAssetFromFolder_NestedFolderRejected` regression. |
| `docs/changelog/2026-07-21_0038-reject-nested-asset-registration.md` | New changelog per skill template. |
| `tasks/current/0038_bug_reject-nested-asset-registration/description.md` | Frontmatter status → `in-progress` then `in-review`. Add `## Plan` link to `./plan.md`. |

No ADR (bug fix tightens an existing domain rule; no architectural
decision). No `docs/guidelines/` change. No glossary change — the term
"asset-container root" is coined locally in a Godoc and does not need
elevation to glossary for one internal helper.

## Step-by-step execution

### 1. Centralize container roots in `internal/render`

Edit `internal/render/render.go`:

- Add package-level (near existing package vars, or above `addSkillOutputs`):

  ```go
  // skillRoots maps each supported agent to its skill container directory
  // — the folder under which one child folder per skill is projected. Kept
  // at package scope so AssetContainerRoots can derive from it without
  // duplicating strings.
  var skillRoots = map[string]string{
      "codex":       ".codex/skills",
      "claude-code": ".claude/skills",
      "opencode":    ".opencode/skills",
  }

  // cursorCommandsRoot is Cursor's folder-shaped asset container. Cursor
  // flattens each skill to a single .md file inside it, but the container
  // itself still acts as a root for folder-registration purposes.
  const cursorCommandsRoot = ".cursor/commands"

  // AssetContainerRoots returns the fixed set of managed-surface paths
  // under which a direct child folder is eligible for asset registration.
  // These are the only folder-shaped asset containers; agents_doc/settings
  // render to single files and have no child folder to register.
  //
  // Consumed by app.RegisterableDirs as the single source of truth so no
  // duplicated hard-coded list drifts. Returns a fresh sorted slice; the
  // caller may mutate it.
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

- Delete the function-local `skillRoots := map[string]string{...}` (lines
  217–221) — `addSkillOutputs` now references the package-level var.
- Replace `".cursor/commands"` literal at line 240 with `cursorCommandsRoot`.
- Add `"sort"` to the import block if not present.

Render output is unchanged — same paths, same map contents; only the
symbols moved out of the function body.

### 2. Rewrite `RegisterableDirs` in `internal/app/service.go`

Add `"github.com/hexworks/agentfiles/internal/render"` to the import block.

Replace the body of `RegisterableDirs` (service.go:220–245) with:

```go
// RegisterableDirs returns the set of directory keys in changes that are
// eligible for asset registration: the directory's parent path is a known
// asset-container root (render.AssetContainerRoots) and every descendant
// leaf under it is unknown. Ancestors above a container root and folders
// nested deeper than a direct child are never returned — the "register
// folder as asset" flow only makes sense for a direct child of a
// container root. CreateAssetFromFolder re-asserts on this set so a stale
// or nested dirKey is rejected with FolderNotRegisterableError.
func RegisterableDirs(changes []FileChange) map[string]bool {
    roots := make(map[string]bool, 4)
    for _, r := range render.AssetContainerRoots() {
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
    out := map[string]bool{}
    for dir, n := range total {
        if n > 0 && unknown[dir] == n {
            out[dir] = true
        }
    }
    return out
}
```

Behavior after change:

- `.claude/skills/foo/x.md` (unknown) → i=2 dir=`.claude/skills/foo`,
  parent=`.claude/skills` (root) → counted. Other ancestors skipped
  because their parent is not a root. → `.claude/skills/foo` registerable.
- `.claude/skills/foo/bar/y.md` (unknown) → i=2 dir=`.claude/skills/foo`
  counted (parent is root); i=3 dir=`.claude/skills/foo/bar` skipped
  (parent is not a root). → `foo` registerable, `bar` NOT.
- `.claude/skills` root itself → i=0 dir=`.claude`, parent="" → not root;
  i=1 dir=`.claude/skills`, parent=`.claude` → not root. → not registerable.
- Partly-managed `.claude/skills/foo` — one create leaf inflates `total`
  but not `unknown` → excluded (existing invariant preserved).
- `docs/whatever` outside any root → nothing counted → not registerable.

Existing callers (`CreateAssetFromFolder` service.go:270,
`assertIgnoredRegisterable` service.go:465, `plan_project.go:380`) need no
change — set semantics unchanged, contents tightened.

### 3. Rewrite tests in `internal/app/service_create_asset_from_folder_test.go`

Replace `TestRegisterableDirs` fixture with the acceptance-criterion
table:

```go
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
        if !got[dir] {
            t.Errorf("dir %q should be registerable", dir)
        }
    }
    for _, dir := range []string{
        ".claude/skills/bar",
        ".claude/skills/nest/deep",
        ".claude/skills",
        ".claude",
        "docs/whatever",
        "docs",
    } {
        if got[dir] {
            t.Errorf("dir %q must NOT be registerable", dir)
        }
    }
}
```

Add a nested-`dirKey` regression next to
`TestCreateAssetFromFolder_NonRegisterableFolderRejected`.

### 4. Write changelog

Path: `docs/changelog/2026-07-21_0038-reject-nested-asset-registration.md`.
Populate from `.claude/skills/af.task.implement/changelog-template.md`.

### 5. Task frontmatter & Plan link

Handled by the implement-task workflow (status transitions + this file's
placement).

## Verification

- `make build` — compile succeeds.
- `make test` — full suite green. Specifically:
  - `go test ./internal/app -run TestRegisterableDirs`
  - `go test ./internal/app -run TestCreateAssetFromFolder`
  - `go test ./internal/render`
- `make lint` (`go vet ./...`) — clean.
- Smoke: build then `./bin/af` on a repo with `.claude/skills/foo/bar/x.md`
  → register-asset picker offers `.claude/skills/foo` only.

## Guideline compliance

- **clean_architecture** — new dependency `app → render` respects inward
  direction (sync already imports render). No cycle introduced.
- **errors** — no new error type; reuses `FolderNotRegisterableError`.
- **solid / clean_code** — single source of truth for container roots.
- **testing** — table-driven fixture, typed `errors.As` assertion.
- **asset_authoring** — no projection targets altered; render output identical.
