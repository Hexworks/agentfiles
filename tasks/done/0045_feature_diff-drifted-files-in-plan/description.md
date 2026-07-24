---
id: 0045
type: feature
status: done
topics: tui, go, sync_and_safety
notes: Diff is only meaningful for change kinds that have BOTH a managed/desired body and a local on-disk body. That is ChangeUpdate (desired rendered vs on-disk) and ChangeDrift (desired managed vs on-disk drifted). ChangeCreate (no local), ChangeDelete (no local), and ChangeUnknown (no managed) cannot be diffed. Current row action wiring lives in internal/tui/shell/plan_project.go (treeActionsFn ~L587, openFileBtn ~L636); Open is added to all file leaves.
---

# Diff option for drifted files in Plan view

## Acceptance Criteria

- [ ] A `[Diff]` row action (mnemonic `d`) is rendered on the Plan Project
      screen **only** for `ChangeUpdate` and `ChangeDrift` file rows. It is
      absent on `ChangeCreate`, `ChangeDelete`, and `ChangeUnknown` rows (they
      lack one side of the diff). Verified by
      `go test ./internal/tui/shell -run TestDiffButton`: build a preview with
      one row per kind and assert the action list contains the `d` button
      exactly for update+drift.
- [ ] The `d` mnemonic does not collide on any row: Update row = `o`,`d`;
      Drift row = `o`,`p`/`w`/`t`,`d`. Covered by the existing
      mnemonic-uniqueness assertion in the shell tests.
- [ ] `Service.DiffFile(profileRef, projectID, path)` returns the on-disk
      (local) body and the rendered (desired/managed) body for `path`,
      re-rendering read-only. It is reachable through the `actions` seam that
      `tui/shell` uses (mirroring `PlanProject`), and `tui/shell` never imports
      `internal/app`. Verified by `go test ./internal/app -run TestDiffFile`:
      a fixture profile+project with a drifted file and an updated file returns
      the expected two bodies for each.
- [ ] The unified-diff text is produced with `github.com/aymanbagabas/go-udiff`
      in `internal/tui` (domain returns bytes, tui formats strings). Direction
      is kind-dependent: **Update** → old = local, new = managed/desired;
      **Drift** → old = managed, new = local. Verified by a diff-text builder
      unit test asserting the `-`/`+` sides for each kind on a fixed
      input → output pair.
- [ ] When the two bodies are byte-identical the modal shows a `No differences`
      message instead of an empty pane (covered by the same builder test with
      equal inputs).
- [ ] Pressing `d` opens a scrollable modal (viewport-backed `modal.Content`,
      same frame as the `?` help modal) showing the unified diff; `esc` closes
      it and returns to the Plan tree (`q` stays the global quit). Added/removed
      lines are visually distinguished (lipgloss `+`/`-` coloring).
- [ ] A read failure for the local file (e.g. it was deleted between plan and
      diff) surfaces a typed error rendered inside the modal, not a panic or
      silent blank.

## Out of scope

- Editing or applying from the diff view — it is read-only; resolution stays on
  the existing Keep/Overwrite/Adopt/Delete toggles.
- Binary-file detection, syntax highlighting, and any max-size cap on the diff
  (managed assets are text config; render the unified diff as-is).
- Diffs for `ChangeCreate` / `ChangeDelete` / `ChangeUnknown` rows (missing one
  side — no `[Diff]` action offered).
- Side-by-side (split) diff layout — unified diff only.
- Caching rendered bodies in `Preview`/`FileChange`; `DiffFile` re-renders
  lazily on demand.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/shell -run TestDiffButton` — `[Diff]` (`d`) present on
  update+drift rows, absent on create/delete/unknown.
- `go test ./internal/app -run TestDiffFile` — `DiffFile` returns the correct
  local and desired bodies for a drifted file and an updated file.
- Diff-text builder unit test — asserts old/new direction flips by kind
  (Update: old=local/new=managed; Drift: old=managed/new=local) and that equal
  inputs yield the `No differences` message.
- Smoke: `./bin/af` → Plan Project on a project with a drifted managed file →
  move cursor to the drift row → press `d` → modal shows the unified diff
  (local vs managed) → `esc` returns to the tree.
