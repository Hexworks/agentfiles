# Plan — 0033 Apply clears pending drift

Cross-links:

- Task body: [./description.md](./description.md)
- Decision: [../../../docs/adr/0015-drift-keep-preserves-baseline.md](../../../docs/adr/0015-drift-keep-preserves-baseline.md)
- Superseded clause: [../../../docs/adr/0010-sync-resolutions-and-first-apply.md](../../../docs/adr/0010-sync-resolutions-and-first-apply.md)
- Glossary Drift entry: [../../../docs/glossary.md](../../../docs/glossary.md)

## Summary

Redefine `DriftKeep` as **leave alone + preserve prior baseline**. Remove the
adopt-on-disk-hash side effect from `sync.Apply`. Stop `onApply` from
materializing a default Keep for untouched drift rows. Rewrite the two tests
that pin the old semantics; add a pure-ignore-set regression test.

Docs (ADR 0015, glossary Drift entry) already reflect this decision — no doc
edits required beyond a pointer note in ADR 0010's Keep clause.

## Files to edit

| File | Change |
| ---- | ------ |
| `internal/sync/sync.go` | `Apply` `ChangeDrift` branch: on non-overwrite, record the prior baseline (`preview.ManagedState.ManagedFiles[change.Path]`), not `HashFile(abs)`. Defensively guard `preview.ManagedState == nil`. Update `DriftKeep` doc comment (L105–113) + `Apply` doc block (L273–287). |
| `internal/tui/shell/plan_project.go` | `onApply` (L830–845): emit a `DriftResolution` only when `s.driftResolutions[ch.Path] == app.DriftOverwrite`; untouched rows fall through. Update the block comment (L812–816). |
| `internal/sync/sync_test.go` | Rewrite `TestApply_DriftKeep_AdoptsCurrentAsBaseline` → `TestApply_DriftKeep_PreservesPriorBaseline`. Extend `TestApply_DefaultDrift_LeavesAlone` to assert baseline preserved AND next `Plan` still classifies path as `ChangeDrift`. Add `TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched`. Keep `TestApply_DriftKeep_LeavesOnDiskAlone` + `TestApply_DriftOverwrite_RewritesDrift` + `TestApply_DuplicateResolutions_LastWins` as regression guards. |
| `docs/adr/0010-sync-resolutions-and-first-apply.md` | Add a one-line pointer at the top of the DriftKeep discussion: *"Superseded by ADR 0015 — Keep now preserves the prior baseline; adopt behavior removed."* |

No new files. No new `docs/guidelines/` entries. No new ADRs (0015 already
accepted).

## Execution steps

1. **Test-first: pin the fixed contract.**
   - Rewrite `TestApply_DriftKeep_AdoptsCurrentAsBaseline` → `TestApply_DriftKeep_PreservesPriorBaseline`:
     seed `state.ManagedFiles["AGENTS.md"] = "previous"`, on-disk = `"drifted"`,
     Apply with `Drift: [{Path, Decision: DriftKeep}]`, assert
     `state.ManagedFiles["AGENTS.md"] == "previous"` **and** next `Plan`
     re-classifies as `ChangeDrift`.
   - Extend `TestApply_DefaultDrift_LeavesAlone` with the same baseline +
     next-plan-drift assertions.
   - Add `TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched`: two
     managed files (one clean, one drifted), pre-existing
     `state.IgnoredPaths = [".foo"]`, Apply with `Resolutions{IgnoredPaths: nil}`.
     Assert drift file's baseline unchanged, clean file's rendered hash still
     recorded, `state.IgnoredPaths == nil`.
   - Run `go test ./internal/sync -run 'TestApply_DriftKeep_PreservesPriorBaseline|TestApply_DefaultDrift_LeavesAlone|TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched'` — the first two must FAIL red, the third must FAIL red. Red gate confirms the tests actually pin the bug.

2. **Fix `sync.Apply`.**
   - In the `ChangeDrift` case: after the `DriftOverwrite` branch, replace the
     `HashFile(abs)` block with:

     ```go
     if preview.ManagedState != nil {
         recordedHashes[change.Path] = preview.ManagedState.ManagedFiles[change.Path]
     } else {
         delete(recordedHashes, change.Path)
     }
     ```

     (Any `ChangeDrift` row implies `state != nil` today per `classifyDesired`,
     but the nil guard is cheap and matches the domain-error safety habit.)
   - Update `DriftKeep` doc comment (`internal/sync/sync.go` ~L105–113) to the
     leave-alone semantics from ADR 0015.
   - Update `Apply` doc block (~L273–287) to remove the "adopts the current
     on-disk hash" line.

3. **Fix `onApply` in the TUI shell.**
   - Replace the drift branch of the `range s.preview.Changes` loop so it emits
     a resolution **only** when the user picked `DriftOverwrite`:

     ```go
     case app.ChangeDrift:
         if s.driftResolutions[ch.Path] == app.DriftOverwrite {
             drift = append(drift, app.DriftResolution{
                 Path: ch.Path, Decision: app.DriftOverwrite,
             })
         }
     ```

   - Rewrite the block comment (~L812–816) to describe the new contract
     ("emit only Overwrite; default Keep is expressed by absence, per ADR
     0015").

4. **Docs pointer.**
   - Add one line to `docs/adr/0010-sync-resolutions-and-first-apply.md` in the
     DriftKeep discussion pointing to ADR 0015.

5. **Green gate.**
   - `go test ./internal/sync` — the three targeted tests turn green; every
     other existing sync test stays green.
   - `go test ./internal/tui/shell -run 'PlanProject'` (or the closest existing
     scope) stays green.

6. **Full gate.**
   - `make build && make test && make lint` all pass.

7. **Manual verification** (per description's Verification block).
   - `./bin/af` → open a project with pending drift + a persisted ignored
     folder → un-ignore the folder → **Apply** → reopen **Plan** → the
     drifting files are STILL `drift`, not `update`. `.agentfiles/state.json`
     baseline for those paths is unchanged from before Apply.

## ADRs

- **New:** none. ADR 0015 already exists and is accepted.
- **Updated:** ADR 0010 — one-line pointer to ADR 0015 next to its DriftKeep
  clause.

## Documentation

- Glossary Drift entry already reflects the leave-alone semantics — verify
  wording still matches after the fix; no change expected.
- No `docs/architecture/` chapter needs an edit: the runtime view already
  describes plan/apply generically; the specific `DriftKeep` contract lives in
  ADR 0015 and code comments.

## Guidelines

- No new/updated files under `docs/guidelines/`. The change is a bugfix aligned
  with `sync_and_safety.md` ("Distinguish Update From Drift", "Keep Managed
  State Accurate") — no principle changes.

## Out of scope

- **Adopt** (promote local edits into the profile). Tracked as task **0035**.
- Repairing already-corrupted `state.json` in this repo. Already fixed by hand.
- Any new drift UI affordance beyond the existing Keep ↔ Overwrite toggle.

## Acceptance Criteria

- [ ] `Apply` with `DriftKeep` (or no drift entry) preserves the **prior**
      `state.json` baseline for the path — neither on-disk nor rendered hash —
      `TestApply_DriftKeep_PreservesPriorBaseline`.
- [ ] A path classified `drift` still classifies as `drift` on the next `Plan`
      after a Keep/default Apply — `TestApply_DefaultDrift_LeavesAlone`
      (extended) + `TestApply_DriftKeep_PreservesPriorBaseline`.
- [ ] A pure ignore-set Apply leaves every unrelated drift baseline untouched —
      `TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched`.
- [ ] `DriftOverwrite` still writes the rendered body and records the rendered
      hash — `TestApply_DriftOverwrite_RewritesDrift` unchanged and green.
- [ ] `onApply` emits `DriftResolution` entries **only** for `DriftOverwrite`
      rows — verified by reading the code + shell test if one exists; otherwise
      pinned by the sync-level ignore-set regression test above.
- [ ] `sync.DriftKeep` and `sync.Apply` doc comments state the leave-alone
      contract.
- [ ] ADR 0010's DriftKeep clause points at ADR 0015.
- [ ] `make build && make test && make lint` pass on the branch.

## Verification

```
make build && make test && make lint
./bin/af   # reproduce steps in description.md → drift stays drift after Apply
```
