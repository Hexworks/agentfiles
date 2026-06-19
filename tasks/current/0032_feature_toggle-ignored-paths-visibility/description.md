---
id: 0032
type: feature
status: active
topics: tui, charm, go
---

# Toggle Ignored Paths visibility on the Plan Project screen

Follow-up to task 0031 (`ignore-unknown-asset-folder`). 0031 lets the user
**Ignore** an unknown folder in-session and persists it to the target repo's
`.agentfiles/state.json` `ignored_paths` on Apply. Once persisted, `sync.Plan`
suppresses any `ChangeUnknown` under that folder (`isUnderIgnored`,
`internal/sync/sync.go`), so the folder vanishes **entirely** from the
**Plan Project** "Changes" table — no row, no toggle. Today the only way to
un-ignore is to hand-edit `state.json`.

This task adds the surface 0031 deferred: **view and un-ignore already-persisted
ignored folders** on the Plan Project screen.

## Behaviour

### Screen-level toggle button

- New screen-level button rendered in the body button row, **between `[Apply]`
  and `[Back]`**: `[Apply]  [Show Ignored]  [Back]`.
- It is a toggle:
  - hidden state → label **Show Ignored**, mnemonic `g`.
  - shown state → label **Hide Ignored**, mnemonic `h`.
- Default is **hidden** (already-ignored folders not shown).
- Always rendered, even when the project has zero persisted ignored paths
  (predictable layout; toggling just shows an empty result).
- Excluded from the status bar, same rule as `[Apply]` (the body button already
  shows it). See `StatusKeys` in `internal/tui/shell/plan_project.go`.

`g` was chosen over `i` because 0031 already binds `i` to the row-level
**Ignore** button, and the mnemonic `Set` enforces uniqueness across cursor-row
+ screen-level buttons together — a screen-level `i` would collide whenever the
cursor sat on a registerable folder.

### Shown-ignored folder rows

Persisted-ignored folders carry **no `FileChange` and no children** (Plan
suppressed the subtree). They are surfaced from `ManagedState.IgnoredPaths`,
which must be plumbed to the TUI (see Data plumbing).

When **Show Ignored** is on, each persisted-ignored folder renders as a
**collapsed leaf** (no trailing `/`, no subtree — same shape as a 0031-ignored
folder), injected into the tree at its **natural nested path position**
(creating intermediate parent dir nodes as needed), interleaved with the normal
change rows and sorted by path. One unified tree, not a separate block.

| Column     | Value for a shown-ignored row |
| ---------- | ----------------------------- |
| Name       | folder path segment           |
| Status     | `! ignored` (muted style)      |
| Resolution | (blank)                        |
| Actions    | `[Show]` (mnemonic `w`) — un-ignore |

`! ignored` is distinct from `? unknown` so a persisted-ignored folder reads
differently from a live unknown.

### Un-ignore / re-ignore (per-row toggle)

- Pressing `[Show]` (`w`) on a persisted-ignored folder **un-ignores** it. The
  folder then "becomes part of the in-memory state": its row is **pinned
  visible** regardless of the Show/Hide Ignored toggle, and its action button
  flips to **`[Ignore]`** (mnemonic `i`) so the user can toggle back within the
  session. Mirrors 0031's `Show`⇄`Ignore` toggle on live unknown folders.
- `[Register]` is **not** offered on these rows — the folder is not in
  `RegisterableDirs` (Plan suppressed it, so we lack the file list).
- The row stays a collapsed leaf in both states (no children data available
  until the folder is un-ignored, Applied, and re-Planned).

## Data plumbing

`app.Preview` (`internal/app/service.go`) does **not** currently expose the
ignored set. Add `IgnoredPaths []string` to `app.Preview`, populated from
`llmsync.Preview.ManagedState.IgnoredPaths` in `previewFromSync`. The screen
seeds its in-memory state from this on each load.

## Persistence (replace semantics)

0031's `mergeIgnoredPaths` (`internal/sync/sync.go`) only **unions** prior +
newly-selected keys; nothing removes. Because the TUI now sees the full
persisted set, switch `SyncProject` to **replace** semantics:

- On Apply the screen sends the **complete desired ignored set**:
  `final = (persisted − unignored) ∪ newlyIgnored`, via
  `actions.SyncProjectInput.IgnoredPaths` (and `app.ResolutionSet`).
- `sync.SyncProject` writes the incoming set **verbatim** into
  `state.json` `ignored_paths` (replace, not merge). Drop the union.
- `assertIgnoredRegisterable` still validates only the **newly-added** keys
  (`incoming − prior`, where `prior = preview.ManagedState.IgnoredPaths`);
  already-persisted keys remain exempt.

### Apply effect

Un-ignoring a persisted folder and pressing `[Apply]` rewrites `ignored_paths`
with that key removed. On the **next** Plan, `sync.Plan` no longer suppresses it
→ its files reappear as real `? unknown` rows (with subtree). An Apply whose
only change is the ignore-set is valid — no file writes, just a state rewrite.

## Out of scope

- Rendering the contents / children of a persisted-ignored folder. There is no
  data for them until the folder is un-ignored, Applied, and re-Planned.
- A bulk "un-ignore all" action.
- Changing 0031's in-session Ignore behaviour on live unknown folders.
- Changing how `ChangeUnknown` is classified (task 0017).

## Tests

Per the safety rule, walk every selection state and assert mnemonic uniqueness.
Candidate alphabet now includes `g` / `h` (screen) and the row `w` / `i`.

- Mnemonic uniqueness with the cursor on a shown-ignored row (`[Show]` `w` or
  `[Ignore]` `i`) while `[Show Ignored]`/`[Hide Ignored]` (`g`/`h`), `[Apply]`
  (`a`), and `[Back]` (`b`) are present.
- Default hidden: persisted-ignored rows are absent until `[Show Ignored]` is
  pressed.
- Toggle reveals then hides the rows; a folder touched via `[Show]` stays pinned
  visible after `[Hide Ignored]`.
- `[Show]` on a persisted folder flips its button to `[Ignore]`; `[Ignore]`
  flips it back, and the desired set updates accordingly.
- Apply builds the complete desired set `(persisted − unignored) ∪ newlyIgnored`.
- `sync.SyncProject` replace semantics: given a prior `ignored_paths` and an
  incoming set, `state.json` ends with the incoming set verbatim.
- `assertIgnoredRegisterable` validates only `incoming − prior`; persisted keys
  are exempt.
- Tree injection: a persisted-ignored folder appears nested at its natural
  sorted position as a collapsed leaf.

## Verification

```
make build && make test && make lint
./bin/af   # Plan Project (a project with persisted ignored_paths)
           # → [Show Ignored] (g) → ignored folders appear as "! ignored" leaves
           # → cursor on one → [Show] (w) → row pins, button flips to [Ignore]
           # → [Apply] → reopen Plan → folder's files reappear as ? unknown
```
