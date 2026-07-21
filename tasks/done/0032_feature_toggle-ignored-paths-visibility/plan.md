# Task 0032 — Toggle Ignored Paths visibility on the Plan Project screen

Task: [`./description.md`](./description.md)
Branch to create: `feature/toggle-ignored-paths-visibility`

## Context

Task 0031 lets a user **Ignore** an unknown folder on the Plan Project screen and
persists it to the repo's `.agentfiles/state.json` `ignored_paths` on Apply. Once
persisted, `sync.Plan` suppresses every `ChangeUnknown` under that folder
(`isUnderIgnored`), so the folder disappears entirely from the Changes table — no
row, no toggle. The only way to un-ignore today is hand-editing `state.json`.

This task adds the surface 0031 deferred: **view and un-ignore already-persisted
ignored folders** on the Plan Project screen, and switches persistence from
union to **replace** semantics now that the TUI can see and send the full set.

## Blocker (must resolve before implementation)

Working tree is **dirty** (`git status --porcelain`):
`.agentfiles/state.json`, `.claude/skills/af.task.implement/SKILL.md`,
`.codex/skills/af.task.implement/SKILL.md`. Step 5 requires a clean tree before
branching. **Ask the user to commit/stash these before I create the branch.**

## Design decisions (resolved from code, no open questions)

State the screen tracks, seeded fresh on every load:
- `persistedIgnored []string` — seeded from `app.Preview.IgnoredPaths` (sorted). The folders Plan suppressed.
- `unignored map[string]bool` — persisted folders pending un-ignore (pressed `[Show]`).
- `pinned map[string]bool` — persisted folders touched this session → kept visible regardless of the Show/Hide toggle. Monotonic (re-ignore keeps the pin).
- `showIgnored bool` — screen-level toggle, default `false`.
- existing `ignoredPaths map[string]bool` keeps its current meaning: live-unknown folders newly ignored this session.

A persisted-ignored folder is **visible** in the tree when `showIgnored || pinned[key]`.
Its button is `[Ignore]` (`i`) when `unignored[key]`, else `[Show]` (`w`). It is
always a collapsed leaf (`! ignored`, muted) — no children data exists.

Apply sends the complete desired set: `final = (persistedIgnored − unignored) ∪ ignoredPaths`.

Mnemonics: screen toggle uses `g` (Show Ignored) / `h` (Hide Ignored); row uses
`w` (Show) / `i` (Ignore). Verified collision-free against every cursor-row set
(`o,p,w,d,r,i`) plus screen-level `a,b`. `[Show Ignored]` is excluded from the
status bar (same rule as `[Apply]`).

## Step-by-step plan

### 1. Data plumbing — `internal/app/service.go`
- Add `IgnoredPaths []string` to `Preview` (struct at ~L292).
- In `previewFromSync` (~L298): after the nil guard, read
  `p.ManagedState.IgnoredPaths` (guard `ManagedState != nil`) into the new field.
- `assertIgnoredRegisterable` (~L394) already validates only `incoming − prior` and
  `Apply` already forwards `r.IgnoredPaths` verbatim to sync — **no change needed**.

### 2. Replace semantics — `internal/sync/sync.go`
- In `Apply` (~L364): replace `mergeIgnoredPaths(preview.ManagedState, r.IgnoredPaths)`
  with `normalizeIgnoredPaths(r.IgnoredPaths)`.
- Replace `mergeIgnoredPaths` (~L385) with `normalizeIgnoredPaths(selected []string) []string`:
  dedup + sort (`utils.DeduplicateAndSort`), return `nil` when empty. Rewrite the
  doc comment to describe replace (incoming written verbatim; the TUI owns the full set).

### 3. Plan Project screen — `internal/tui/shell/plan_project.go`
- `planNode`: add `persistedIgnored bool`.
- struct: add `persistedIgnored []string`, `unignored`, `pinned map[string]bool`,
  `showIgnored bool`, `showIgnoredBtn *mnemonic.Button`. Init maps in constructor.
- `buildButtons`: build the toggle via a new `refreshShowIgnoredBtn()` that picks
  label/mnemonic from `showIgnored` (`Show Ignored`/`g` ↔ `Hide Ignored`/`h`) and
  wires `toggleShowIgnored`.
- `Body`: button row → `[Apply]  [<Show/Hide> Ignored]  [Back]` (insert between).
- `StatusKeys`: unchanged (toggle stays off the bar).
- `rebuildSet`: also `set.Add(s.showIgnoredBtn)`.
- `handleLoaded`: seed `persistedIgnored` (sorted clone of `preview.IgnoredPaths`),
  reset `unignored`/`pinned`/`showIgnored`, then build the tree via new `rebuildTree()`.
- New `visiblePersistedIgnored() []string`: persisted keys where `showIgnored || pinned`.
- New `rebuildTree()`: `SetRoot(buildPlanTree(name, preview.Changes, ignoredPaths, visiblePersistedIgnored()))`.
- `statusValue` / `statusStyle`: dir node with `persistedIgnored` → `"! ignored"` /
  `styles.MutedStyle`; other dirs/root stay `""` / empty style.
- `treeActionsFn` dir branch: if `d.persistedIgnored` → `[Ignore]`(`i`) when
  `unignored[d.path]` else `[Show]`(`w`); existing live-unknown branches unchanged.
- New handlers: `unignorePersisted(path)` (`unignored=true`, `pinned=true`,
  `RefreshActions`+`rebuildSet`), `reignorePersisted(path)` (`delete(unignored)`,
  pin stays, `RefreshActions`+`rebuildSet`), `toggleShowIgnored()`
  (`showIgnored = !showIgnored`, `refreshShowIgnoredBtn`, `rebuildTree`, `rebuildSet`).
- `onApply`: build `final` set = `(persistedIgnored − unignored) ∪ ignoredPaths`,
  sorted, into `SyncProjectInput.IgnoredPaths`.

### 4. `buildPlanTree` — add injection (`internal/tui/shell/plan_project.go`)
- New signature `buildPlanTree(projectName string, changes []app.FileChange, collapsed map[string]bool, injectedIgnored []string)`.
- Merge changes + injected keys into one slice sorted by path (`strings.Compare`),
  build incrementally over the shared `dirs` map so injected leaves interleave at
  their natural nested/sorted position. Change rows use existing per-part logic
  (collapse via `collapsed`); injected keys create expanded parent dirs then a
  terminal collapsed leaf with `planNode{persistedIgnored:true}` (no trailing slash,
  no children).
- Update existing callers/tests passing 3 args to pass `nil` for `injectedIgnored`.

### 5. Tests (`internal/tui/shell/plan_project_test.go`, `internal/sync/sync_test.go`, `internal/app/*_test.go`)
- Mnemonic uniqueness with cursor on a shown-ignored row in both `[Show]`/`[Ignore]`
  states, with `[Show Ignored]`/`[Hide Ignored]` present (extend the exhaustive walk).
- Default hidden: persisted rows absent until `[Show Ignored]`; toggle reveals then hides.
- A folder touched via `[Show]` stays pinned visible after `[Hide Ignored]`.
- `[Show]` flips button to `[Ignore]` and back; desired set updates accordingly.
- `onApply` builds `(persisted − unignored) ∪ newlyIgnored`.
- `buildPlanTree` injects a persisted folder as a nested collapsed `! ignored` leaf
  at its natural sorted position.
- sync: rewrite `TestApply_UnionsAndPersistsIgnoredPaths` → replace semantics
  (prior `.cursor/old` + incoming `.codex/new` ⇒ state ends with only `.codex/new`).
- app: `previewFromSync` exposes `ManagedState.IgnoredPaths` on `Preview`.

### 6. Docs
- [`docs/adr/0010-sync-resolutions-and-first-apply.md`](../../../docs/adr/0010-sync-resolutions-and-first-apply.md):
  update the ignored-paths addendum — union → replace (task 0032), TUI owns the full desired set.
- [`docs/manual/plan_project.md`](../../../docs/manual/plan_project.md): document
  `[Show Ignored]`/`[Hide Ignored]` and the persisted-folder `[Show]`⇄`[Ignore]` row toggle.
- No new ADR or guideline file warranted (extends 0031's mechanism).

### 7. Skill bookkeeping
- Set `description.md` frontmatter `status: in-progress` then `in-review`; add a `## Plan` link.
- Changelog: `docs/changelog/2026-06-19_0032-toggle-ignored-paths-visibility.md`.

## Verification
```
make build && make test && make lint
./bin/af   # Plan Project on a project with persisted ignored_paths:
           #  [Show Ignored] (g) → folders appear as "! ignored" leaves
           #  cursor on one → [Show] (w) → row pins, button flips to [Ignore]
           #  [Hide Ignored] (h) → pinned row stays
           #  [Apply] → reopen Plan → un-ignored folder's files reappear as ? unknown
```
