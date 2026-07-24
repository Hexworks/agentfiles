# 0044 changes

The Plan Project screen used to render a single cycling toggle button on drift
and unknown rows whose label was the *next* state in the trilean cycle
(Keep → Overwrite → Adopt → Keep for drift; Keep → Delete → Adopt → Keep for
unknown-owned-by-asset). Users had to remember what the current selection meant
and mentally simulate the cycle to reach the option they wanted.

The row-actions column now renders each non-selected trilean option as its own
mnemonic button. Bilean rows — drift on legacy v2 state entries, unknowns with
no owning asset — keep a single toggle since "the other option" is unambiguous.
Actions column width was bumped 22 → 26 to fit the widest legitimate combination
(`[Open] [Overwrite] [Adopt]`).

A small domain change carries the drift-row eligibility signal through to the
TUI without asking it to inspect state.json: `sync.FileChange` and
`appapi.FileChange` gain an `AdoptProvenance struct{AssetID, SourceRel string}`,
populated only for `ChangeDrift` rows whose state entry carries both keys (v3
provenance). Its `Available()` method answers "may this row offer Adopt?"; the
TUI degrades to bilean when it returns false, so legacy v2 entries no longer
surface a doomed `[Adopt]` button. `sync.Apply` still returns
`AdoptUnavailableError` as defense-in-depth.

## Decisions

- Reused existing mnemonics verbatim (`p` Keep, `w` Overwrite, `t` Adopt, `d`
  Delete, `o` Open) — **Why:** avoids re-checking every other screen for
  mnemonic collisions and keeps muscle memory intact for users on 0035.
- Bilean rows stay single-toggle rather than expanding to `[Keep]/[Delete]` +
  `[Keep]/[Overwrite]` — **Why:** the "other option" is unambiguous, so an
  extra button adds visual noise without new information.
- No status-column indicator for `AdoptEligible == false` drift rows — **Why:**
  legacy v2 entries flip to v3 on the next re-apply, so the missing `[Adopt]`
  is transient. Adding a badge would need to be removed once v2 is fully aged
  out.
- `AdoptProvenance` lives on `sync.FileChange` (not derived at TUI time from
  `preview.ManagedState`) — **Why:** the boundary types intentionally hide
  ManagedState from the TUI. Adding it to the boundary mirror keeps the TUI
  ignorant of provenance schema while giving it (via `Available()`) the answer
  it needs. Carrying the keys rather than a bare bool keeps the shape symmetric
  with `OwningAssetID` on unknown rows.

## Assumptions

- The screen-level mnemonic alphabet (`{o, w, p, t, d, r, i, a, g, h, b}`) has
  enough slack that the new "both non-selected options visible at once"
  rendering never introduces a collision — **Why:** the plan verified every
  matrix row is pair-wise unique against `[Open]o` + screen-level buttons, and
  the new `TestPlanProjectMnemonicUniqueness` walks every combination to prove
  it stays that way.

## Other Notes

- New tests: `TestPlan_DriftAdoptEligibility_{TrueOnV3StateEntry,FalseOnV2LegacyEntry,ZeroOnNonDriftKinds}`
  in `internal/sync/sync_test.go` pin the domain flag; `TestPlanProjectRowButtonsMatchMatrix`
  and `TestPlanProjectMnemonicUniqueness` in
  `internal/tui/shell/plan_project_test.go` pin the TUI matrix and mnemonic
  invariants respectively.
- The old cycle-based tests (`TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt`,
  `TestPlanProjectScreen_TreeActionsFnDriftKeepRendersOpenAndOverwriteBtn`,
  `TestPlanProjectScreen_TreeActionsFnDriftOverwriteRendersOpenAndAdoptBtn`,
  `TestPlanProjectScreen_TreeActionsFnDriftAdoptRendersOpenAndKeepBtn`,
  `TestPlanProjectScreen_TreeActionsFnUnknownKeepRendersOpenAndDeleteBtn`,
  `TestPlanProjectScreen_TreeActionsFnUnknownDeleteRendersOpenAndKeepBtn`) were
  rewritten or replaced.
- No ADR / arc42 / glossary update. This is a rendering fix; the matrices in
  `tasks/current/0044_bug_show-non-selected-trilean-buttons/description.md`
  plus the test tables are the canonical spec.

## Add AdoptEligible to `sync.FileChange`

```go
// before
type FileChange struct {
    Path          string
    Kind          ChangeKind
    Reason        ReasonKind
    OwningAssetID string
}
```

```go
// after — flag flows through Preview to the TUI without exposing ManagedState
type FileChange struct {
    Path          string
    Kind          ChangeKind
    Reason        ReasonKind
    OwningAssetID string
    // AdoptEligible is populated only for ChangeDrift rows. True iff the
    // managed-state entry for the path carries non-empty AssetID and
    // SourceRel (v3 provenance).
    AdoptEligible bool
}
```

## Populate `AdoptEligible` in `classifyDesired`

```go
// before
baseline := state.ManagedFiles[file.Path].Hash
if baseline != "" && baseline != currentHash {
    return FileChange{Path: file.Path, Kind: ChangeDrift, Reason: ReasonDriftDetected}, false, nil
}
```

```go
// after — read the entry once; provenance drives AdoptEligible
entry := state.ManagedFiles[file.Path]
if entry.Hash != "" && entry.Hash != currentHash {
    return FileChange{
        Path:          file.Path,
        Kind:          ChangeDrift,
        Reason:        ReasonDriftDetected,
        AdoptEligible: entry.AssetID != "" && entry.SourceRel != "",
    }, false, nil
}
```

## Rewrite the drift/unknown toggle factories

```go
// before — single cycling button whose label is the next state in the cycle
func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button {
    switch s.driftResolutions[path] {
    case appapi.DriftOverwrite:
        return mnemonic.New("Adopt", 't', ...)
    case appapi.DriftAdopt:
        return mnemonic.New("Keep", 'p', ...)
    }
    return mnemonic.New("Overwrite", 'w', ...)
}
```

```go
// after — set of non-selected buttons, bilean vs. trilean per AdoptEligible
func (s *planProjectScreen) driftToggleButtons(path string, adoptEligible bool) []*mnemonic.Button {
    current := s.driftResolutions[path]
    if !adoptEligible {
        if current == appapi.DriftOverwrite {
            return []*mnemonic.Button{s.driftBtnKeep(path)}
        }
        return []*mnemonic.Button{s.driftBtnOverwrite(path)}
    }
    switch current {
    case appapi.DriftOverwrite:
        return []*mnemonic.Button{s.driftBtnKeep(path), s.driftBtnAdopt(path)}
    case appapi.DriftAdopt:
        return []*mnemonic.Button{s.driftBtnKeep(path), s.driftBtnOverwrite(path)}
    }
    return []*mnemonic.Button{s.driftBtnOverwrite(path), s.driftBtnAdopt(path)}
}
```

The unknown factory follows the same shape, gated on the row's
`OwningAssetID != ""` instead of adopt provenance.

> The review fix-up below supersedes the two snippets above: the `AdoptEligible
> bool` became an `AdoptProvenance` struct and the two hand-enumerated factories
> were collapsed into one generic helper.

## Bump Actions column width

```go
// before
treetable.WithActions(treetable.Column{Title: "Actions", Width: 22}, s.treeActionsFn()),
```

```go
// after — fits `[Open] [Overwrite] [Adopt]` (25 visible chars + 1 slack)
treetable.WithActions(treetable.Column{Title: "Actions", Width: 26}, s.treeActionsFn()),
```

## Review fix-up

The 0044 review applied these cohesion/naming/test changes before merge:

- **`AdoptEligible bool` → `AdoptProvenance struct{AssetID, SourceRel string}`**
  on both `sync.FileChange` and `appapi.FileChange`, with an `Available()`
  method. The drift row now carries the actual reverse-mapping keys (symmetric
  with `OwningAssetID` on unknown rows) instead of a derived bool. The struct
  name states the fact; the `appapi` doc is written in TUI-facing terms.
- **`ManagedFileEntry.HasAdoptProvenance() bool`** is the single predicate for
  `AssetID != "" && SourceRel != ""`, called by both `classifyDesired` (to seed
  the drift row) and `classifyDriftAdopt` (to guard the reverse-write).
- **`trileanToggleButtons[T comparable](...)`** — one generic helper holds the
  "render the two non-selected options, bilean when ineligible" rule; the drift
  and unknown factories delegate to it instead of hand-enumerating twin
  `switch`es.
- **`treeActionsFn`** reads both eligibility signals off the `FileChange` DTO
  (`AdoptProvenance.Available()` for drift, `OwningAssetID != ""` for unknown).
  The now-unread `unknownOwners` projection map was removed.
- **Tests trimmed to the matrix walker + mnemonic sweep.** Removed the four
  narrow `TreeActionsFn*KeepRenders*` per-state tests, `UnknownAdoptShownOnlyWhenOwnedByAsset`,
  `ToggleUnknownSwapsState`, and `MnemonicUniquenessExhaustive` — each was a
  subset of `TestPlanProjectRowButtonsMatchMatrix` / `TestPlanProjectMnemonicUniqueness`.
  Dropped the dead `assertUniquePlanMnemonics` / `assertUniqueCaseInsensitiveMnemonics`
  helpers (`Set.Add` already panics on any duplicate, so the walk is the guard).
  Added `kindRegisterableDir` to the mnemonic sweep's `want` list so its row
  predicate is load-bearing, and split `TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds`
  into four isolated `t.Run` subtests.
