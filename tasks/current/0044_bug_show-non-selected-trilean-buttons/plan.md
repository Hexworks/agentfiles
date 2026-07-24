# Plan — 0044 Show non-selected trilean options as row buttons

Task body: [description.md](./description.md).
Depends on: task 0035 (ADR 0020 — Adopt drift into profile).

Related docs (unchanged by this task, referenced for context):
- `docs/adr/0015-drift-keep-preserves-baseline.md`
- `docs/adr/0020-adopt-drift-into-profile.md`
- `docs/guidelines/tui.md`, `docs/guidelines/charm.md`, `docs/guidelines/sync_and_safety.md`
- `docs/glossary.md` (drift / unknown / trilean rows)

No new ADR, no arc42 update, no glossary edit. The description's four matrices plus the two new TUI table-driven tests are the canonical spec (`## Out of scope`, description.md:153-155).

## Change summary

1. Domain plumbing (`internal/sync`, `internal/appapi`, `internal/app`) — attach an `AdoptEligible bool` to every `FileChange`; populate it only for `ChangeDrift` rows whose state entry is v3 (non-empty `AssetID` **and** `SourceRel`).
2. TUI rendering (`internal/tui/shell/plan_project.go`) — replace the single `driftToggleBtn` / `unknownToggleBtn` cycling factories with factories that return a **set** of buttons (2 for trilean rows, 1 for bilean rows) matching the four matrices in `description.md:53-84`. Bump the Actions column width from 22 to 26.
3. Tests — add `TestPlan_DriftAdoptEligibility_*` in `internal/sync/sync_test.go`; add `TestPlanProjectRowButtonsMatchMatrix` and `TestPlanProjectMnemonicUniqueness` in `internal/tui/shell/plan_project_test.go`; adapt the existing button-set tests (`TreeActionsFnDriftOverwriteRendersOpenAndAdoptBtn`, `DriftToggleCyclesKeepOverwriteAdopt`, `UnknownAdoptShownOnlyWhenOwnedByAsset`, `MnemonicUniquenessExhaustive`) that hard-code the old single-button shape.

## Step-by-step execution

### Step 1 — Add `AdoptEligible` to `sync.FileChange`

File: `internal/sync/sync.go` (line 210).

- Add `AdoptEligible bool` on `FileChange`. Doc-comment: populated only for `ChangeDrift`; `true` iff the state entry for the path carries non-empty `AssetID` **and** `SourceRel` (v3 provenance). Zero-valued for every other `ChangeKind` and for legacy v2 state entries — mirrors `docs/adr/0020` and the `AdoptUnavailableError` guard in `applyDrift`.

### Step 2 — Populate `AdoptEligible` in `classifyDesired`

File: `internal/sync/sync.go:316-344`.

- In the `ChangeDrift` branch (line 341), read the entry once (`entry := state.ManagedFiles[file.Path]`) and return `FileChange{Path, Kind: ChangeDrift, Reason: ReasonDriftDetected, AdoptEligible: entry.AssetID != "" && entry.SourceRel != ""}`. Reuse the same variable for `baseline := entry.Hash` instead of the current `state.ManagedFiles[file.Path].Hash` re-lookup.
- Every other classify return keeps `AdoptEligible: false` (zero value). No behavior change for `ChangeCreate` / `ChangeUpdate` / `ChangeDelete` / `ChangeUnknown` — the flag is drift-only.
- `loadState` (`internal/sync/sync.go:809`) already rejects half-populated v3 entries, so `AssetID != "" && SourceRel != ""` and `AssetID != ""` alone are equivalent inputs. Keep the redundant check anyway — it makes the intent obvious at the call site without depending on the loader's invariant.

### Step 3 — Mirror the field on `appapi.FileChange`

File: `internal/appapi/appapi.go:65-72`.

- Add `AdoptEligible bool` with a doc-comment pointing at `sync.FileChange`'s copy.

### Step 4 — Copy through `previewFromSync`

File: `internal/app/service.go:285-307`.

- Extend the `appapi.FileChange{...}` literal at line 291 with `AdoptEligible: ch.AdoptEligible`.

### Step 5 — Rewrite drift toggle factory into a button-set factory

File: `internal/tui/shell/plan_project.go:598-680`.

Rename `driftToggleBtn(path string) *mnemonic.Button` → `driftToggleButtons(path string, adoptEligible bool) []*mnemonic.Button` (rename lets the compiler surface every existing call site; the return-type change alone would silently break the tests). Behavior per description.md matrices (severity order Keep < Overwrite < Adopt, left-to-right):

- `adoptEligible == false` (bilean, matches "Drift — AdoptEligible == false" matrix at description.md:61-66):
  - Current Keep → `[[Overwrite] w → toggleDrift(path, DriftOverwrite)]`
  - Current Overwrite → `[[Keep] p → toggleDrift(path, DriftKeep)]`
- `adoptEligible == true` (trilean, description.md:53-59):
  - Current Keep → `[[Overwrite] w, [Adopt] t]`
  - Current Overwrite → `[[Keep] p, [Adopt] t]`
  - Current Adopt → `[[Keep] p, [Overwrite] w]`

Same for `unknownToggleBtn` → `unknownToggleButtons(path string, adoptEligible bool) []*mnemonic.Button`, per description.md:71-84 (bilean when `unknownOwners[path] == ""`, trilean when non-empty). Substitute `Delete/d` for `Overwrite/w`.

Update `treeActionsFn` (line 598) to append the returned slice via `btns = append(btns, s.driftToggleButtons(d.path, d.change.AdoptEligible)...)` for `ChangeDrift` and `btns = append(btns, s.unknownToggleButtons(d.path, s.unknownOwners[d.path] != "")...)` for `ChangeUnknown`. `[Open]o` stays as the leading button on file rows (line 618).

No cycle-through: every button's handler passes its target `DriftDecision` / `UnknownDecision` directly to `toggleDrift` / `toggleUnknown`. The existing `toggleDrift` / `toggleUnknown` methods already accept the target as an argument (`plan_project.go:694,705`) so no signature change there.

Drop the "next-state cycling" comments on the old factories; the new factories render the *non-selected* options directly.

### Step 6 — Bump Actions column width 22 → 26

File: `internal/tui/shell/plan_project.go:253`.

- `treetable.WithActions(treetable.Column{Title: "Actions", Width: 26}, s.treeActionsFn())`. Justification is in description.md:40-44 ( `[Open] [Overwrite] [Adopt]` = 25 visible chars + 1 slack). Name column shrinks by 4 chars at the same terminal width; acceptable per current TUI sizing rules.

### Step 7 — Update existing tests broken by the button-set shape

File: `internal/tui/shell/plan_project_test.go`.

The following tests assume `driftToggleBtn(path).Trigger()` cycles state and hard-code single-button return shapes. Rewrite to the new factories:

- `TestPlanProjectScreen_ActionValueReflectsResolutionMap` (line 248) — replace `driftToggleBtn("drift.md").Trigger()` with a direct call to `toggleDrift("drift.md", appapi.DriftOverwrite)` (or the new factory's first-index Trigger with `adoptEligible=false`); same for unknown.
- `TestPlanProjectScreen_TreeActionsFnDriftKeepRendersOpenAndOverwriteBtn` (line 266) — assert 2 buttons `[Open, Overwrite]` for `AdoptEligible=false`, 3 buttons `[Open, Overwrite, Adopt]` for `AdoptEligible=true`. Split into two test funcs; each pins the matrix's row for one `AdoptEligible` value.
- `TestPlanProjectScreen_TreeActionsFnDriftOverwriteRendersOpenAndAdoptBtn` (line 279), `TreeActionsFnDriftAdoptRendersOpenAndKeepBtn` (line 297), `TreeActionsFnUnknownKeepRendersOpenAndDeleteBtn` (line 314), `TreeActionsFnUnknownDeleteRendersOpenAndKeepBtn` (line 327) — same treatment: assert the exact button set per matrix.
- `TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt` (line 457) — delete; the new factory has no cycle. Replaced by `TestPlanProjectRowButtonsMatchMatrix` (Step 8) which drives `toggleDrift` / `toggleUnknown` directly.
- `TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset` (line 488) — rewrite to assert the exact button set per matrix instead of cycle transitions. Also folded into `TestPlanProjectRowButtonsMatchMatrix`.
- `TestPlanProjectScreen_ToggleUnknownSwapsState` (line 552) — replace `fn(n)[1].Trigger()` (cycle) with pressing the button at each state, per matrix.
- `TestPlanProjectScreen_MnemonicUniquenessExhaustive` (line 815) — extend the override set to cover the drift bilean case (`AdoptEligible=false`) and the unknown bilean case (`OwningAssetID=""` — already present as `orphan.md`). Wrap the walk so it drives the new factory's per-state button counts. Also convert the `sawToggleLabel` check to include `Adopt`. This test is superseded by the more comprehensive `TestPlanProjectMnemonicUniqueness` in Step 8 — keep it in place as the coarse smoke check, or delete it if the new test's coverage is strictly greater. Decide during implementation; if deleted, note in the changelog.
- `TestPlanProjectScreen_ApplyEmitsAdoptResolutions` (line 522) and `OnApplyWithSelectionsBuildsCorrectSlices` (line 630) — unchanged; both write directly to `driftResolutions` / `unknownResolutions` and don't touch the buttons.

### Step 8 — New TUI table-driven tests

File: `internal/tui/shell/plan_project_test.go`.

Add two exhaustive tests keyed off the description's four matrices, plus a wall-clock guard against the "cursor stuck on a row that skips the toggle" hole task 0042 review flagged.

**`TestPlanProjectRowButtonsMatchMatrix`** — table drives every combination in the four matrices in description.md:53-84:

```go
type matrixRow struct {
    kind          appapi.ChangeKind
    adoptEligible bool          // for drift: entry.AdoptEligible; for unknown: OwningAssetID != ""
    current       string        // "Keep" | "Overwrite" | "Adopt" | "Delete"
    wantLabels    []string      // labels of the non-Open buttons, in render order
    wantMnemonics []rune        // parallel to wantLabels
}
```

For every row: build a `planNode`, seed the resolution map to match `current`, call the new button-set factory, assert `len(buttons) == 1 + len(wantLabels)` (`[Open]o` first), assert each subsequent button's label + mnemonic, then press each button and assert the resulting `driftResolutions[path]` / `unknownResolutions[path]` matches the button's target decision (Keep → absent, Overwrite → `DriftOverwrite`, Adopt → `DriftAdopt`, Delete → `UnknownDelete`).

**`TestPlanProjectMnemonicUniqueness`** — for every row kind × every `current` state per matrix (dir non-registerable / dir registerable / dir ignored / dir persisted-ignored / create / update / delete / drift-bilean × 2 / drift-trilean × 3 / unknown-owned × 3 / unknown-orphan × 2), navigate the tree cursor onto that row, call `s.rebuildSet()`, assert every registered button (row-level + screen-level `[Apply] a`, `[Show/Hide Ignored] g|h`, `[Back] b`) has a distinct case-insensitive mnemonic rune. Fails loud on duplication. Assert the walk actually visited each row kind (`saw[kind]` map) so a future cursor-navigation regression that silently skips a row surfaces as a test failure, not a passed test with no work done.

### Step 9 — New sync tests

File: `internal/sync/sync_test.go`.

Add three targeted classifier tests:

- **`TestPlan_DriftAdoptEligibility_TrueOnV3StateEntry`** — real profile+project, `writeStateEntries` with `{Hash: "prev", AssetID: "base", SourceRel: "AGENTS.md"}`, on-disk `AGENTS.md` body = `"drifted"`. Assert the resulting `preview.Changes[i].AdoptEligible == true` on the drift row.
- **`TestPlan_DriftAdoptEligibility_FalseOnV2LegacyEntry`** — same but via `writeStateV2Legacy` (bare hash string). Assert `AdoptEligible == false` on the drift row.
- **`TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds`** — first-apply fixture (no state) with a fresh `AGENTS.md`. Assert `AdoptEligible == false` on the `ChangeCreate` row. Then a fixture that produces `ChangeUpdate` (rendered body changes, no drift) plus a `ChangeUnknown` and a `ChangeDelete`. Assert all three carry `AdoptEligible == false`.

### Step 10 — Existing sync + app-service invariants

- `TestApply_DriftAdopt_WritesProfileAndClearsDrift` (sync_test.go:1004): unchanged; the `AdoptUnavailableError` guard is orthogonal to `AdoptEligible`.
- `TestState_LoadV2LegacyEntries_LeavesAdoptDisabled` (sync_test.go:1138): unchanged; keeps proving that a v2 → `DriftAdopt` submission still returns `AdoptUnavailableError`.
- Task 0035's DoD checkbox for `AdoptUnavailableError` on ineligible rows stays green (description.md:141-143).

### Step 11 — Quality gate (from `af.task.implement` Step 5)

`make fmt && make lint && make build && make test`. No warning tolerance; if any hook fails, fix the root cause, don't work around.

## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| `sync.FileChange` fields today | `internal/sync/sync.go:209-219` | `type FileChange struct { Path string; Kind ChangeKind; Reason ReasonKind; OwningAssetID string }` |
| `classifyDesired` returns the `ChangeDrift` branch here | `internal/sync/sync.go:341` | `return FileChange{Path: file.Path, Kind: ChangeDrift, Reason: ReasonDriftDetected}, false, nil` |
| Drift branch has non-nil `state` guaranteed | `internal/sync/sync.go:317-319` | `if state == nil { return FileChange{... ChangeCreate ...} }` — every later branch runs with `state != nil` |
| `state.ManagedFiles` value type is v3 `ManagedFileEntry` with `AssetID` + `SourceRel` fields | `internal/sync/sync.go:36-40` | `type ManagedFileEntry struct { Hash string \`json:"hash"\`; AssetID string \`json:"asset_id,omitempty"\`; SourceRel string \`json:"source_rel,omitempty"\` }` |
| `loadState` rejects half-populated v3 entries so `AssetID != "" ⇔ SourceRel != ""` on any loaded state | `internal/sync/sync.go:809-810` | `if (trimmedAsset == "") != (trimmedSource == "") { return nil, StateCorruptError{...} }` |
| `appapi.FileChange` mirror fields today | `internal/appapi/appapi.go:65-72` | `type FileChange struct { Path string; Kind ChangeKind; OwningAssetID string }` |
| `previewFromSync` copies `sync.FileChange` → `appapi.FileChange` here | `internal/app/service.go:289-296` | `changes[i] = appapi.FileChange{ Path: ch.Path, Kind: appapi.ChangeKind(ch.Kind), OwningAssetID: ch.OwningAssetID }` |
| Existing single-button `driftToggleBtn` signature | `internal/tui/shell/plan_project.go:654` | `func (s *planProjectScreen) driftToggleBtn(path string) *mnemonic.Button` |
| Existing single-button `unknownToggleBtn` signature | `internal/tui/shell/plan_project.go:668` | `func (s *planProjectScreen) unknownToggleBtn(path string) *mnemonic.Button` |
| `treeActionsFn` appends toggle button after `[Open]` on file rows | `internal/tui/shell/plan_project.go:614-625` | `btns := []*mnemonic.Button{s.openFileBtn(d.path)}; ... btns = append(btns, s.driftToggleBtn(d.path))` (drift) / `s.unknownToggleBtn(d.path)` (unknown) |
| Actions column width today | `internal/tui/shell/plan_project.go:253` | `treetable.WithActions(treetable.Column{Title: "Actions", Width: 22}, s.treeActionsFn())` |
| `toggleDrift` / `toggleUnknown` already accept the target decision as argument (no cycle inside) | `internal/tui/shell/plan_project.go:694,705` | `func (s *planProjectScreen) toggleDrift(path string, next appapi.DriftDecision) tea.Cmd` |
| `mnemonic.Set.Add` panics on duplicate rune (case-insensitive) | `internal/tui/components/mnemonic/set.go:51-62` | `if existing, ok := s.byRune[r]; ok { panic(...) }` — every uniqueness test relies on this |
| Screen-level buttons registered in the `mnemonic.Set` | `internal/tui/shell/plan_project.go:485-493` | `set.Add(s.applyBtn); set.Add(s.showIgnoredBtn); set.Add(s.backBtn)` |
| `showIgnoredBtn` mnemonic flips `g ↔ h` | `internal/tui/shell/plan_project.go:232-238` | `mnemonic.New("Hide Ignored", 'h', ...) / mnemonic.New("Show Ignored", 'g', ...)` |
| `[Register] r` on registerable dir rows | `internal/tui/shell/plan_project.go:629-631` | `return mnemonic.New("Register", 'r', ...)` |
| `[Ignore] i` / `[Show] w` on dir rows | `internal/tui/shell/plan_project.go:640-645` | `mnemonic.New("Show", 'w', handler) / mnemonic.New("Ignore", 'i', handler)` |
| Mnemonic alphabet across all row/screen buttons — needed to prove uniqueness has slack: `{o, w, p, t, d, r, i, a, g, h, b}` (11 runes, all distinct) | This plan + the pinned constants above | Derived — verified by walking every `mnemonic.New(...)` call in `plan_project.go` |

## Acceptance criteria (all boundary-crossing checks pinned to real stacks)

- [ ] `sync.FileChange` and `appapi.FileChange` both gain `AdoptEligible bool`; `previewFromSync` copies the field through.
- [ ] `TestPlan_DriftAdoptEligibility_TrueOnV3StateEntry` in `internal/sync/sync_test.go` uses real `profile.Load` + real `Plan` + real on-disk state (`writeStateEntries`) inside `t.TempDir()`, asserts the drift row's `AdoptEligible == true`.
- [ ] `TestPlan_DriftAdoptEligibility_FalseOnV2LegacyEntry` uses real `writeStateV2Legacy` + real `Plan`, asserts `AdoptEligible == false` on the drift row.
- [ ] `TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds` asserts `AdoptEligible == false` on `ChangeCreate`, `ChangeUpdate`, `ChangeUnknown`, and `ChangeDelete` rows via a real `Plan` over a real project fixture.
- [ ] `driftToggleBtn` is replaced by `driftToggleButtons(path, adoptEligible) []*mnemonic.Button`; the returned button set matches description.md:53-66 for both `adoptEligible` values; pressing button N calls `toggleDrift(path, N)` directly with no cycle.
- [ ] `unknownToggleBtn` is replaced by `unknownToggleButtons(path, adoptEligible) []*mnemonic.Button`; the returned button set matches description.md:71-84; pressing button N calls `toggleUnknown(path, N)` directly with no cycle.
- [ ] `treeActionsFn` appends the returned buttons after `[Open]o` unchanged in order.
- [ ] Actions column width in `plan_project.go` bumped from 22 to 26.
- [ ] `TestPlanProjectRowButtonsMatchMatrix` in `internal/tui/shell/plan_project_test.go` walks every combination in the four description matrices, builds real `planProjectScreen` state via `newPlanProjectScreen` + `planLoadInto`, asserts the rendered button labels + mnemonics match exactly and in order, and asserts pressing each button sets the drift/unknown resolution map to the button's target value.
- [ ] `TestPlanProjectMnemonicUniqueness` walks every cursor position across every row kind listed in Step 8, calls `s.rebuildSet()`, and asserts no two active buttons (row + screen-level combined) share a case-insensitive mnemonic rune; explicitly asserts the walk visited every row kind.
- [ ] `sync.Apply` still returns `AdoptUnavailableError` when a `DriftAdopt` resolution is submitted against an `AdoptEligible == false` row — existing `TestState_LoadV2LegacyEntries_LeavesAdoptDisabled` (`internal/sync/sync_test.go:1138`) stays green without edits.
- [ ] `make fmt && make lint && make build && make test` all pass with no new warnings.
- [ ] Smoke run pinned as a DoD checkbox (moved out of `## Verification` per af.task.plan's boundary-crossing rule): `./bin/af` → Profiles → project → Plan on a project whose state.json carries a v3 drift entry. Drift row cursor shows `[Open][Overwrite][Adopt]` when Keep-selected; pressing `w` sets Overwrite and the button set becomes `[Open][Keep][Adopt]`; pressing `t` sets Adopt and the button set becomes `[Open][Keep][Overwrite]`.
- [ ] Smoke: unknown row inside a known asset dir cycles the analogous three states via the button set; an unknown row outside any known asset dir shows the single bilean `[Keep]/[Delete]` toggle (no `[Adopt]`).
- [ ] Smoke: a drift row backed by a legacy v2 state entry renders only `[Open][Overwrite]` (Keep-selected) / `[Open][Keep]` (Overwrite-selected); `[Adopt]` is absent; re-applying the project promotes the entry to v3 and the next Plan renders the trilean set on the same row.

## Out of scope (repeated from description for the reader)

- Screen-level button layout (Apply / Show Ignored / Back stay).
- `sync.Apply` semantics beyond the existing `AdoptUnavailableError` guard.
- `state.json` schema, migration, or `app.Service.Apply` write path.
- Button visuals / `mnemonic.Button` package itself.
- ADR / arc42 / glossary docs.
- Status-column indicator for `AdoptEligible == false` drift rows.
- Bilean row expansion — bilean drift (legacy v2) and bilean unknown (orphan) keep single-toggle behavior.
- Other screens (`EditAsset`, `SelectProjectAssets`, …) — no trilean toggles live there today.
