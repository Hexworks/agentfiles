# Task 0030 — Create Asset From Folder

Task: [./description.md](./description.md)

## Context

On the **Plan Project** screen (`internal/tui/shell/plan_project.go`, built in task 0029) the
treetable lists per-file changes. Files the profile does not own classify as
`? unknown` (`app.ChangeUnknown`). Today the only row-level actions are `[Open]` on every
file leaf plus a Keep/Delete toggle on unknown files. There is no way to turn an unmanaged
folder (e.g. `.claude/skills/something/`) into a profile-owned asset from this screen — the
user must leave, hand-author an asset, copy files in, and re-select it.

Task 0030 adds a **Register as Asset** action (mnemonic `r`) on **directory rows whose every
descendant file is unknown**. Pressing it opens the existing Create Asset modal pre-scoped to
that folder. On submit a new asset is created under the active profile, the folder's files are
**copied** into the profile's `assets/`, and the asset is selected for the current project so a
re-plan reclassifies those (new, managed) files as `create`/`update` instead of `? unknown`.

Out of scope (per task): changing `ChangeUnknown` classification, and moving/deleting the
original project files.

## Approach

Reuse the existing seams end-to-end:
- The modal lifecycle pattern already used by `editProfileScreen` (modal field + `modalKind` +
  `handleResolved`, see `internal/tui/shell/edit_profile.go:211-355,606-702`).
- The existing Create Asset modal `modals.NewCreateAsset(asset.Manifest)`
  (`internal/tui/modals/create_asset.go:29`) — pre-fill `Name` from the folder basename; the
  user still picks type/description. The folder's absolute source path is carried on the screen,
  not in the manifest (manifest has no source field).
- A new bundled use case `app.Service.CreateAssetFromFolder` keeps the
  create-asset-then-copy-then-select invariant in one consistency boundary (domain guideline:
  one invariant should not be split across partial updates).

### Files

**1. `internal/utils/fs.go` (+ `internal/utils/errors.go`, + `fs_test.go`)** — new recursive copy helper.
- Add `func CopyDir(src, dst string) errs.DomainError`: `filepath.WalkDir(src)`, for each regular
  file read bytes + `WriteFile(filepath.Join(dst, rel), data, mode)` preserving the file's mode and
  relative structure; skip directories (parents auto-created by `WriteFile`). No symlink following.
- Add typed `CopyDirError{Src, Dst, Err}` to `utils/errors.go` (per errors guideline).
- Test in `t.TempDir()`: nested tree copied with content + structure preserved.

**2. `internal/asset/asset.go` (+ `asset_test.go`)** — create-from-folder constructor.
- Add `func InitFromFolder(root string, manifest Manifest, sourceDir string) (string, errs.DomainError)`:
  `manifest.Validate()` → compute `dir` (same layout as `Init`: `assets/<type>/<id>/`) →
  `utils.EnsureDir(dir)` → `utils.CopyDir(sourceDir, dir)` → write `asset.json` **last** (so a
  stray source `asset.json` can't clobber ours). No type-specific starter files (real content
  comes from the folder).
- Test: given a temp source folder with `SKILL.md`, InitFromFolder creates the asset dir, copies
  the file, and writes a valid manifest.

**3. `internal/app/service.go` (+ test)** — bundled use case.
- Add `func (s *Service) CreateAssetFromFolder(profileRef, projectID string, manifest asset.Manifest, sourceDir string) (string, errs.DomainError)`:
  `resolveProject` → derive `manifest.ID` via `utils.Slug` if blank (mirrors `InitAsset:150-152`)
  → dup-check `loaded.Assets[id]` (reuse `AssetExistsError`) → `asset.InitFromFolder(loaded.Root, manifest, sourceDir)`
  → append id to `p.SelectedAssetIDs` and `project.Save(loaded.Root, p)` (inlined rather than
  calling `SelectAsset`, whose `loaded.Assets` map predates creation). Returns the asset id.
- Tests: creates asset + copies files + selects for project; dup id → `AssetExistsError`;
  unknown project → `ProjectNotFoundError`.

**4. `internal/actions/inputs.go` + `internal/actions/assets.go`** — TUI seam.
- `CreateAssetFromFolderInput{ProfileRef, ProjectID string; Manifest asset.Manifest; SourceDir string}`.
- `func (a *Actions) CreateAssetFromFolder(in CreateAssetFromFolderInput) (string, errs.DomainError)`
  forwarding to the service method.

**5. `internal/tui/shell/plan_project.go` (+ test)** — the screen wiring.
- Extend the `planProjectActions` interface with `CreateAssetFromFolder(actions.CreateAssetFromFolderInput) (string, errs.DomainError)`.
- Add fields: `modal *modal.Modal`, `modalKind planModalKind`, `width, height int`, `registerSourceDir string`.
  Add a small `planModalKind` enum (`none`, `registerAsset`).
- `planDirNode(n)` helper (mirrors `planFileNode`) returning ok only for `planNodeDir`.
- `dirAllUnknown(n *treetable.Node) bool`: recurse `n.Children`; every file leaf must be
  `app.ChangeUnknown`, require ≥1 file. (Literal task rule: offered on dir rows whose contents are
  all unknown. Ancestor dirs of an all-unknown subtree qualify too — the user chooses the right
  folder; we do not over-restrict.)
- `treeActionsFn`: for `planNodeDir` rows where `dirAllUnknown`, return `[registerAssetBtn(d.path)]`
  (mnemonic `r`). File-row behavior unchanged.
- `onRegisterAsset(dirPath)`: set `registerSourceDir = filepath.Join(s.projectPath, filepath.FromSlash(dirPath))`,
  open `modals.NewCreateAsset(asset.Manifest{Name: path.Base(dirPath)})`, set `modalKind=registerAsset`,
  size via `modalSize`, return `s.modal.Init()`.
- `Update`: add modal-guard + `modal.ResolvedMsg` + `tea.WindowSizeMsg` branches mirroring
  `edit_profile.go`. Add `handleResize` (store width/height, resize modal). `handleKey` forwards to
  modal when active.
- `handleResolved`/`afterRegisterAsset`: on `Confirmed`, extract `asset.Manifest`, run a command
  that calls `CreateAssetFromFolder` with `registerSourceDir`; on success emit a success toast **and**
  `loadCmd()` to re-plan (stay on screen — do NOT reuse `mutationCmd`, which pops). On error, toast
  the typed error. Use a dedicated `registerAssetDoneMsg{name string; err errs.DomainError}`.
- `Body`: composite modal over body (`s.modal.Render(...)`) like `edit_profile.go:349-355`, nil-guarded.
- `InputFocused`: return `s.modal != nil && s.modal.Active()`.
- `rebuildSet` already folds `tree.Buttons()` → the `r` button registers automatically; no mnemonic
  clash on dir rows ({r,a,b}).
- Test: table-driven `dirAllUnknown` over a built `buildPlanTree` (all-unknown dir → true; mixed →
  false; empty → false). Optionally a fake-actions test for `afterRegisterAsset` success → reload.

**6. `docs/manual/plan_project.md`** — document the new `r` Register-as-Asset action.

## Guidelines applied
- **Clean architecture / domain**: copy + asset construction live in `utils`/`asset` (infra/domain);
  the bundled use case lives in `app`; the TUI only collects input, opens the modal, and renders.
  No business logic in `internal/tui`.
- **Errors**: typed `CopyDirError`; reuse `AssetExistsError`/`ProjectNotFoundError`.
- **Testing pyramid**: unit tests for `CopyDir`, `InitFromFolder`, the service method, and the
  `dirAllUnknown` helper; manual TUI pass for the modal flow.

## Docs / ADR
- No ADR: reuses existing patterns (modal lifecycle, app use case, managed-surface fence), no new
  durable architecture decision.
- Update `docs/manual/plan_project.md`.
- Changelog written at skill step 14 (`docs/changelog/`).

## Verification
```
make build && make test && make lint
./bin/af   # Plan Project → cursor on an unknown folder → r → fill modal → submit
           # → toast "Asset created", plan reloads, folder's rendered files now show create/update
```
