# Plan — 0045 Diff option for drifted files in Plan view

Task: [`./description.md`](./description.md)

## Goal

Add a read-only `[Diff]` row action (mnemonic `d`) on the Plan Project screen
for **`ChangeUpdate`** and **`ChangeDrift`** file rows only. Pressing it opens a
scrollable, viewport-backed modal (same frame as the `?` help modal) showing a
colored unified diff between the on-disk body and the rendered/managed body.
Direction flips by kind. Read-only — resolution stays on the existing toggles.

## Design overview

Dependency direction stays `tui/shell → actions → app → domain`:

- **`app`** gets `Service.DiffFile` — returns the two raw bodies by re-rendering
  read-only (no bodies cached in `Preview`). Local-read failure → typed error.
- **`actions`** forwards it through a single-input struct (mirrors `PlanProject`).
- **`internal/tui/components/diffview`** — new component: a unified-diff **text
  builder** (`go-udiff`, direction chosen by `ChangeKind`, colored `+`/`-`) plus
  a viewport-backed `modal.Content` (mirrors `help`).
- **`tui/shell/plan_project.go`** — new `[Diff]` button on update/drift file
  rows; press dispatches a command calling `DiffFile`, the result opens the diff
  modal. `esc` closes it.

Domain returns bytes; the TUI formats and colors the diff string. No render read
from the repo (Adopt-style exception not needed — desired body comes from render,
local body is read only for display).

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Desired/managed body of a path = render output `RenderedFile.Body` for that path | `internal/render/render.go:26`, `:54` | `type RenderedFile struct { Path string; Body []byte …`; `func Build(p *profile.Profile, proj *project.Manifest) (*ProjectPlan, …)` |
| Render is read-only and returns every desired file keyed by forward-slash target path | `internal/render/render.go:104` | `slices.SortFunc(renderedFiles, func(a, b RenderedFile) int { return strings.Compare(a.Path, b.Path) })` |
| On-disk local path = `filepath.Join(projectPath, filepath.FromSlash(path))` | `internal/sync/sync.go:553` | `a.mutated = append(a.mutated, filepath.Join(a.preview.ProjectPath, filepath.FromSlash(rel)))` |
| `FileChange.Path` is forward-slash project-relative | `internal/tui/shell/plan_project.go:972` | `// Paths are split on "/" because appapi.FileChange.Path is forward-slash relative per the domain's validatePathKey rule.` |
| `ChangeUpdate`/`ChangeDrift` are the only kinds with both a desired and a local body | `tasks/current/0045_feature_diff-drifted-files-in-plan/description.md:6` | task `notes:` frontmatter |
| `udiff.Unified(oldLabel, newLabel, old, new string) string` returns `""` when inputs are byte-identical | `go-udiff@v0.4.1/udiff.go` (external, in module cache) | `func Unified(oldLabel, newLabel, old, new string) string` |
| `go-udiff v0.4.1` is already resolved (transitive via glamour) and cached | `go.sum:17` | `github.com/aymanbagabas/go-udiff v0.4.1 h1:…` |
| Row action buttons are the cursor row's `treetable` action factory; every file leaf already gets `[Open]`/`o` | `internal/tui/shell/plan_project.go:587`, `:637` | `func (s *planProjectScreen) treeActionsFn() treetable.ActionsFunc`; `mnemonic.New("Open", 'o', …)` |
| `d` is free on update+drift rows: drift toggles use `p`/`w`/`t`, unknown `Delete` uses `d` but no `[Diff]` is offered there | `internal/tui/shell/plan_project.go:684`,`:688`,`:692`,`:714` | `New("Keep",'p'…)`,`New("Overwrite",'w'…)`,`New("Adopt",'t'…)`,`New("Delete",'d'…)` |
| Screen hosts one optional `modal.Modal` routed by `planModalKind`; `esc`→`Cancelled`→`ResolvedMsg`; `handleResolved` clears it | `internal/tui/shell/plan_project.go:45-50`,`:866` | `type planModalKind int`; `func (s *planProjectScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd` |
| Viewport-backed modal.Content pattern (esc closes, `SetSize` reflows) | `internal/tui/components/help/help.go:57`,`:74`,`:123` | `type content struct { … viewport viewport.Model …`; `func New(id string, req Request, width, height int) *modal.Modal`; `func (c *content) SetSize(width, height int)` |
| Single-input action struct convention for the seam | `internal/actions/inputs.go:64`, `internal/actions/projects.go:28` | `type PlanProjectInput struct { ProfileRef string; ProjectID string }`; `func (a *Actions) PlanProject(in PlanProjectInput) …` |

## Execution plan

### 1. Boundary type — `internal/appapi/appapi.go`

- Add `DiffBodies{ Local, Desired []byte }` (named struct, not a 2-tuple beside
  an error — per `go.md` "prefer structs over tuples").

### 2. `app` — `Service.DiffFile` + typed errors

- `internal/app/errors.go`: add `DiffDesiredMissingError{ Path string }` (path
  absent from the render plan — defensive; update/drift always render) and
  `DiffLocalReadError{ Path string; Err error }`, each with `Error() string`
  (and `Severity()`/`errs.DomainError` matching the neighbours in that file).
- `internal/app/service.go`: add
  `func (s *Service) DiffFile(profileRef, projectID, path string) (appapi.DiffBodies, errs.DomainError)`:
  1. `s.resolveProject(profileRef, projectID)` → `loaded`, `proj`.
  2. `render.Build(loaded.Profile, proj)` (collapse `[]errs.DomainError` via
     `errs.Errors`) — read-only re-render.
  3. find `RenderedFile.Path == path` → `Desired`; not found → `DiffDesiredMissingError`.
  4. `os.ReadFile(filepath.Join(proj.Path, filepath.FromSlash(path)))` → `Local`;
     error → `DiffLocalReadError`.
  5. return `appapi.DiffBodies{Local, Desired}`.

### 3. `actions` seam

- `internal/actions/inputs.go`: `DiffFileInput{ ProfileRef, ProjectID, Path string }`.
- `internal/actions/projects.go`:
  `func (a *Actions) DiffFile(in DiffFileInput) (appapi.DiffBodies, errs.DomainError)`
  → `a.svc.DiffFile(in.ProfileRef, in.ProjectID, in.Path)`.

### 4. `internal/tui/components/diffview` (new package)

- `diff.go` — `func BuildDiff(kind appapi.ChangeKind, local, desired []byte) string`:
  - direction: **Update** → `udiff.Unified("current", "incoming", local, desired)`;
    **Drift** → `udiff.Unified("managed", "local", desired, local)`.
  - `""` result → return the `No differences` message string.
  - otherwise colorize per line: `+` → `styles.CreateStyle`, `-` →
    `styles.DeleteStyle`, `@@`/header → `styles.MutedStyle` (text stays readable
    without color — the `+`/`-` glyphs remain).
- `modal.go` — viewport-backed `modal.Content` mirroring `help.content`:
  `func New(id, diffText string, width, height int) *modal.Modal` with `esc`
  close, `SetSize` reflow, scroll-percent footer. Also
  `func NewError(id, message string, width, height int) *modal.Modal` (or reuse
  `New` with the pre-rendered error text) so a `DiffLocalReadError` renders
  inside the same frame — not a toast, not a blank pane.

### 5. `tui/shell/plan_project.go`

- Add `DiffFile(in actions.DiffFileInput) (appapi.DiffBodies, errs.DomainError)`
  to the `planProjectActions` interface.
- Add `planModalDiff` to the `planModalKind` enum.
- `treeActionsFn`: for `ChangeUpdate` and `ChangeDrift` file rows append
  `s.diffFileBtn(d.path, d.change.Kind)` after `[Open]`.
- `diffFileBtn` → `mnemonic.New("Diff", 'd', func() tea.Cmd { return s.onDiff(path, kind) })`.
- `onDiff` returns a `tea.Cmd` that calls `actions.DiffFile` and returns a new
  `diffReadyMsg{ kind, bodies, err }` (I/O off the Update path per `tui.md`).
- `handleDiffReady`: on `err` open `diffview.New` with the rendered error text;
  else `diffview.BuildDiff(kind, bodies.Local, bodies.Desired)` → open modal
  (`openModal(m, planModalDiff)`). Size from cached `s.width/height`.
- `handleResolved`: `planModalDiff` → clear only (already cleared up-front), no
  post-action.
- Wire `diffReadyMsg` into the `Update` modal-open passthrough list and the main
  `switch` (like `registerAssetDoneMsg`).

### 6. `go.mod`

- `go mod tidy` to promote `github.com/aymanbagabas/go-udiff` from `go.sum`-only
  transitive to a direct `require`.

## Tests

Unit-first, real-stack where a boundary is crossed (`testing.md` cross-boundary rule).

### `internal/tui/components/diffview` (unit)

- `TestBuildDiff_UpdateDirection` — update: old side = local, new side = desired
  (assert a known removed line is `-`, added line is `+`).
- `TestBuildDiff_DriftDirection` — drift: old side = managed(desired), new = local
  (opposite `+`/`-` sides for the same input pair).
- `TestBuildDiff_EqualBodiesShowsNoDifferences` — byte-identical inputs → the
  `No differences` message, not empty.

### `internal/app` (real-stack, no fakes)

- `TestDiffFile` — real `svc.InitAsset` + real project in `t.TempDir()`; write a
  **drifted** managed file and an **updated** managed file on disk; assert
  `DiffFile` returns the expected `Local` (on-disk bytes) and `Desired`
  (rendered bytes) for each. Covers `render.Build` + real `os.ReadFile`.
- `TestDiffFile_LocalReadFailureReturnsTypedError` — remove the on-disk file
  between plan and diff; assert `errors.As` yields `DiffLocalReadError`.

### `internal/actions` (seam)

- `TestDiffFile_ForwardsToService` — light forwarding assertion mirroring the
  existing project-action tests.

### `internal/tui/shell` (fake actions)

- `TestDiffButton` — one file row per `ChangeKind`; assert `treeActionsFn`
  returns a `d`/`[Diff]` button **exactly** for `ChangeUpdate` and `ChangeDrift`,
  and never for `ChangeCreate`/`ChangeDelete`/`ChangeUnknown`.
- `TestPlanProjectScreen_DiffButtonDispatchesDiffFile` — press `d`, assert the
  command calls `actions.DiffFile` with the row path and opens the modal.
- `TestPlanProjectScreen_DiffLocalReadErrorRendersInModal` — fake `DiffFile`
  returns a typed error; assert the modal opens showing the error (no panic, no
  blank).
- Existing `TestPlanProjectMnemonicUniqueness` already walks the update+drift
  rows and calls `rebuildSet` (which panics on a duplicate rune) — it now
  registers `d` alongside `o`/`p`/`w`/`t` with no collision, satisfying
  Acceptance Criterion 2. Extend its assertions/comment to name the `[Diff]`
  button explicitly.

## Acceptance Criteria (DoD checkboxes)

- [x] `[Diff]` (`d`) renders on Plan Project **only** for `ChangeUpdate` and
      `ChangeDrift` file rows; absent on create/delete/unknown. `go test
      ./internal/tui/shell -run TestDiffButton` passes.
- [x] `d` mnemonic never collides (Update = `o`,`d`; Drift = `o`,`p`/`w`/`t`,`d`).
      `go test ./internal/tui/shell -run TestPlanProjectMnemonicUniqueness` passes.
- [x] `Service.DiffFile(profileRef, projectID, path)` returns on-disk (local) and
      rendered (desired) bodies via read-only re-render, reachable through the
      `actions` seam; `tui/shell` never imports `internal/app`. `go test
      ./internal/app -run TestDiffFile` passes (real render + real `os.ReadFile`,
      drifted + updated file).
- [x] Unified-diff text produced with `go-udiff` in `internal/tui`; direction is
      Update → old=local/new=desired, Drift → old=managed/new=local. `go test
      ./internal/tui/components/diffview -run TestBuildDiff` passes.
- [x] Byte-identical bodies show a `No differences` message, not a blank pane
      (`TestBuildDiff_EqualBodiesShowsNoDifferences`).
- [x] Pressing `d` opens a scrollable viewport-backed modal (same frame as `?`
      help) with the colored unified diff; `esc` closes it back to the tree, `q`
      stays global quit. Added/removed lines visually distinguished.
- [x] A local-file read failure surfaces a typed error rendered inside the modal
      (not a panic or blank): `go test ./internal/app -run
      TestDiffFile_LocalReadFailureReturnsTypedError` and
      `go test ./internal/tui/shell -run
      TestPlanProjectScreen_DiffLocalReadErrorRendersInModal` pass.
- [x] Baseline gate: `make build && make test && make lint` all pass.
- [x] Smoke (ticked only after a live run): `./bin/af` → Plan Project on a
      project with a drifted managed file → cursor on the drift row → press `d` →
      modal shows the unified diff (managed vs local) → `esc` returns to the tree.

## Documentation updates

- `docs/manual/plan_project.md` — document the `[Diff]` (`d`) action and its
  update/drift scope (this file backs the screen's `?` help topic).
- `docs/architecture/05-building-block-view.md` — add the `diffview` component
  and the `DiffFile` app-service/actions seam entry.
- `docs/changelog/2026-07-24_0045-diff-drifted-files-in-plan.md` — changelog for
  the feature (written at implement time).

## ADRs

None. The feature follows established patterns (read-only render reuse, existing
modal/seam conventions); promoting an already-resolved transitive dependency to a
direct require is not a durable architectural decision. Noted here so the absence
is deliberate, not an omission.
