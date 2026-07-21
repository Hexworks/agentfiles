# Task 0031 — Ignore unknown asset folder

Description: [./description.md](./description.md)

## Context

On the **Plan Project** screen, unmanaged directories whose every descendant is
`? unknown` currently offer a **Register** action (task 0030) to adopt them as an
asset. This task adds the opposite escape hatch: **Ignore** the folder so it
stops appearing as a change. Ignore is an in-memory toggle (like drift/unknown
resolutions); on **Apply** the ignored folders persist into a new
`ignored_paths` list in `.agentfiles/state.json`, and the next Plan suppresses
any `? unknown` whose path sits under an ignored folder.

Eligibility for Ignore is identical to Register eligibility (a directory whose
whole subtree is unknown), so we reuse the existing `app.RegisterableDirs` set
instead of adding a duplicate predicate.

## Design decisions

- **Reuse `registerableDirs`** for ignore eligibility — same "all descendants
  unknown" rule. Both `[Register]` and `[Ignore]` appear on those rows.
- **Plain `[]string` ignored paths** through the call chain (no Decision enum):
  ignoring is purely additive, so a typed Decision struct would be ceremony.
  Paths are repo-relative forward-slash keys (same form as `managed_files`).
- **Union on Apply**: persisted `ignored_paths` = prior
  `preview.ManagedState.IgnoredPaths` ∪ this-session ignores. Required so
  already-persisted ignores (whose folders have vanished from the table) are not
  dropped when Apply rewrites state from scratch.
- **Collapse via tree rebuild**: `buildPlanTree` takes the ignored set, creates
  the folder node without trailing `/` and without descending into children,
  then `SetRoot`. No change to the `treetable` component.

## Step-by-step plan

### 1. Domain — `internal/sync/sync.go`
- Add field to `ManagedState`: `IgnoredPaths []string` with json tag
  `ignored_paths` (after `ManagedFiles`).
- `loadState`: validate each `IgnoredPaths` key via `validatePathKey`, returning
  `StateCorruptError` on a bad key (mirror the `ManagedFiles` loop).
- New helper `isUnderIgnored(rel string, ignored []string) bool`: true when
  `rel == ig` or `strings.HasPrefix(rel, ig+"/")` for any `ig`.
- `detectDeletesAndUnknowns` / `classifyDeleteOrUnknown`: suppress a discovered
  unknown when `isUnderIgnored(rel, state.IgnoredPaths)` — pass
  `state.IgnoredPaths` into `classifyDeleteOrUnknown` so the unknown never enters
  `unknownSet`. Deletes are unaffected.
- `Apply` signature gains `ignoredPaths []string`. Validate each via
  `validatePathKey` (accumulate into the up-front validation errors). Compute
  `finalIgnored := dedup+sort(preview.ManagedState.IgnoredPaths ∪ ignoredPaths)`
  (guard nil `ManagedState` on first apply) and set `IgnoredPaths: finalIgnored`
  on the new `ManagedState`. Use `utils.Deduplicate` + `slices.Sort`.

### 2. App — `internal/app/service.go`
- `Service.Apply` gains `ignoredPaths []string`; forward to
  `llmsync.Apply(syncPreview, ..., ignoredPaths)`. Plain `[]string`, no helper.

### 3. Actions — `internal/actions/inputs.go` + `projects.go`
- `SyncProjectInput` gains `Ignored []string`.
- `SyncProject` passes `in.Ignored` to `svc.Apply`.

### 4. TUI — `internal/tui/shell/plan_project.go`
- Struct: add `ignoredDirs map[string]bool`; init `{}` in `newPlanProjectScreen`.
- `handleLoaded`: keep `s.registerableDirs = app.RegisterableDirs(...)`; clear
  `s.ignoredDirs` on (re)load. Pass `s.ignoredDirs` to `buildPlanTree`.
- `buildPlanTree(projectName, changes, ignoredDirs)`: when the accumulated dir
  key `acc` is in `ignoredDirs`, create the node with `Label: part` (no `/`),
  add to parent + `dirs`, then **break** the inner parts loop so its child nodes
  are never created (collapse). Already-created node on later changes is found in
  `dirs` and also breaks.
- `treeActionsFn` dir branch:
  - if `s.ignoredDirs[d.path]` → `{showFolderBtn(d.path)}` (Register hidden).
  - else if `s.registerableDirs[d.path]` → `{registerAssetBtn, ignoreFolderBtn}`.
- New buttons:
  - `ignoreFolderBtn(path)` → `mnemonic.New("Ignore", 'i', toggleIgnore(path,true))`.
  - `showFolderBtn(path)` → `mnemonic.New("Show", 's', toggleIgnore(path,false))`.
- `toggleIgnore(path, ignore)`: set/delete `s.ignoredDirs[path]`, then
  `s.tree.SetRoot(buildPlanTree(s.projectName, s.preview.Changes, s.ignoredDirs))`
  (structure changed → SetRoot, not RefreshActions), `s.rebuildSet()`.
- `onApply`: collect sorted keys of `s.ignoredDirs` into `ignored []string`; pass
  via `actions.SyncProjectInput{... Ignored: ignored}`.

### 5. Tests
- `internal/sync/sync_test.go`:
  - State round-trip preserves `ignored_paths`.
  - `Plan` suppresses a `ChangeUnknown` under a persisted ignored folder; a
    sibling unknown outside it stays present.
  - `Apply` unions new ignored paths with prior state's `ignored_paths`.
  - Update existing `Apply(...)` call sites for the new param (pass `nil`).
- `internal/app` + `internal/actions` tests: update `Apply` / `SyncProject` call
  sites; add a case asserting `Ignored` flows through to persisted state.
- `internal/tui/shell` plan-project test: Ignore collapses the folder (children
  gone, label loses `/`, button shows `Show`); Show restores; `onApply` forwards
  the ignored set. Update the fake `planProjectActions.SyncProject` to capture
  `Ignored`.
- `internal/tui/components/treetable` `buildPlanTree` tests if present.

### 6. Docs
- `docs/manual/plan_project.md`: document `Ignore` (`i`) / `Show` (`s`) toggle on
  all-unknown directory rows; ignored folders persist to `ignored_paths` and
  vanish from future plans.
- `docs/glossary.md`: add an **ignored path** entry near managed-state / unknown
  terms.
- `docs/adr/0010-sync-resolutions-and-first-apply.md`: short addition noting
  `ignored_paths` as a persisted, union-on-apply suppression list for unknowns.
  No new ADR — mirrors task 0030, which added none for a comparable feature.

## Verification
```
make build && make test && make lint
./bin/af   # Plan Project → cursor on an all-unknown folder → press i
           #   → row collapses, label drops "/", button shows [Show]
           # → Apply → reopen Plan → folder no longer listed
           # → inspect <repo>/.agentfiles/state.json: ignored_paths contains it
```
Also confirm `s` on an ignored (not-yet-applied) folder restores its subtree,
and a second Apply keeps previously-persisted ignored_paths.

## Out of scope (per task)
- Reclassifying how `ChangeUnknown` is computed (task 0017).
- A screen to view / un-ignore already-persisted `ignored_paths` (follow-up).
- Mutating the ignored folder's files on disk.
