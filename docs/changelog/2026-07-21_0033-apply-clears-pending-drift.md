# 0033 Apply clears pending drift (drift → update) on an unrelated ignore-set change

Redefined `DriftKeep` as **leave alone + preserve prior baseline**, per
[ADR 0015](../adr/0015-drift-keep-preserves-baseline.md). The engine no
longer adopts the on-disk hash into `ManagedState` when a drift row is
kept; the TUI shell no longer materializes a default `DriftKeep`
resolution for every drift row. Together this closes the bug where any
Apply (even a pure ignore-set change) silently rebaselined all pending
drift rows and re-classified them as `update` on the next Plan.

## Decisions

- Preserve the prior baseline on Keep, do not adopt the on-disk hash —
  **Why:** Adopting the on-disk hash conflates two meanings (leave alone
  vs. accept local edits as canonical) and drops the drift warning
  silently. Promoting local edits into the profile is a separate future
  operation (Adopt, task 0035) with its own decision value and UI.
- Keep the `DriftKeep` enum value even though it is now identical to
  absence — **Why:** documents intent and stays symmetric with
  `UnknownKeep`; matches ADR 0015.
- Drift rows: emit a resolution only for `DriftOverwrite`; keep the
  Unknown rows emitting explicit `UnknownKeep` — **Why:** `UnknownKeep`
  is a state no-op, so the "always emit for clarity" pattern is cheap
  there. On Drift, an explicit `DriftKeep` used to trigger the rebaseline
  side effect; absence is now the only safe signal.

Rejected: dropping the `DriftKeep` enum entirely. Symmetry with
`UnknownKeep` and the explicit name in the row toggle UI both favour
keeping it.

## Assumptions

- A `ChangeDrift` row always has a prior baseline entry (drift requires
  `state.ManagedFiles[path]` to be non-empty and different from the
  on-disk hash), so `preview.ManagedState.ManagedFiles[change.Path]` is
  populated in practice. A defensive `preview.ManagedState == nil` guard
  is still added — cheap and matches the domain-error safety habit — and
  falls back to deleting the pre-filled rendered hash so state cannot
  carry a stale entry for a path with no prior baseline.

## Other Notes

- `docs/adr/0015-drift-keep-preserves-baseline.md` was already accepted
  ahead of the fix; no new ADR.
- `docs/adr/0010-sync-resolutions-and-first-apply.md` already carried the
  "**Superseded by ADR 0015**" pointer on the `ChangeDrift` default
  clause; no edit required.
- `docs/glossary.md` Drift entry already reflects the leave-alone
  semantics; no edit required.
- Test surface changed: `TestApply_DriftKeep_AdoptsCurrentAsBaseline`
  was rewritten as `TestApply_DriftKeep_PreservesPriorBaseline`;
  `TestApply_DefaultDrift_LeavesAlone` was extended to assert baseline
  preservation plus a next-Plan re-classification;
  `TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched` is new and
  pins the exact bug scenario. The shell test
  `TestPlanProjectScreen_OnApplyEmptyMapEmitsExplicitKeepResolutions`
  was renamed to `TestPlanProjectScreen_OnApplyEmptyMapOmitsDriftKeepAndKeepsUnknown`
  to reflect the new emission contract.

## `sync.Apply` ChangeDrift branch preserves the prior baseline

The Keep branch used to hash the on-disk file and stamp that hash into
`ManagedState`, which flipped `drift → update` on the next Plan.

```go
// before
case ChangeDrift:
    if driftByPath[change.Path] == DriftOverwrite {
        if err := writeRendered(...); err != nil { ... }
        continue
    }
    // DriftKeep (default): adopt the on-disk hash as the new
    // managed baseline so the path no longer trips drift
    // detection next plan.
    abs := filepath.Join(preview.ProjectPath, filepath.FromSlash(change.Path))
    currentHash, hashErr := utils.HashFile(abs)
    if hashErr != nil {
        domainErrs = append(domainErrs, hashErr)
        continue
    }
    recordedHashes[change.Path] = currentHash
```

```go
// after — Keep preserves the prior baseline (ADR 0015).
case ChangeDrift:
    if driftByPath[change.Path] == DriftOverwrite {
        if err := writeRendered(...); err != nil { ... }
        continue
    }
    // DriftKeep (default): preserve the prior managed baseline
    // so the path stays classified as drift on the next plan
    // (ADR 0015). The pre-filled rendered hash would otherwise
    // flip drift to update on the next Plan (bug 0033). A
    // ChangeDrift row implies state != nil today per
    // classifyDesired; the nil guard defends the invariant.
    if preview.ManagedState != nil {
        recordedHashes[change.Path] = preview.ManagedState.ManagedFiles[change.Path]
    } else {
        delete(recordedHashes, change.Path)
    }
```

## `onApply` emits a drift resolution only for `DriftOverwrite`

The shell used to materialize an explicit `DriftKeep` for every drift row
so a future default change would be visible in the sync input. With the
new semantics, an explicit `DriftKeep` and absence must stay identical —
and absence is the safe signal. Materializing Keep would still be
harmless with the fixed engine, but the code now matches the ADR 0015
contract exactly ("emit only Overwrite").

```go
// before
case app.ChangeDrift:
    decision := app.DriftKeep
    if s.driftResolutions[ch.Path] == app.DriftOverwrite {
        decision = app.DriftOverwrite
    }
    drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: decision})
```

```go
// after — DriftKeep default expressed by absence (ADR 0015, bug 0033).
case app.ChangeDrift:
    if s.driftResolutions[ch.Path] == app.DriftOverwrite {
        drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: app.DriftOverwrite})
    }
```
