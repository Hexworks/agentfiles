---
id: 0044
type: bug
status: in-review
topics: tui, sync_and_safety, charm
depends_on: 0035
---

# Show non-selected trilean options as row buttons on Plan Project

Today the drift/unknown row action column on the Plan Project screen shows a
**single** cycling button whose label is the *next* state in the trilean cycle
(Keep → Overwrite → Adopt → Keep for drift; Keep → Delete → Adopt → Keep for
unknown-owned-by-asset). Users must remember what the current selection means
and mentally simulate the cycle to reach the option they want.

Fix: on rows whose action is a **trilean**, render **both** non-selected
options as separate mnemonic buttons alongside `[Open]`. Pressing either sets
that option directly; no cycling. Rows whose action is a **bilean** keep a
single toggle button, since "the other option" is unambiguous.

This task also folds in a small `sync`/`appapi` change so the TUI can degrade
drift rows to bilean when Adopt is not available for that entry — otherwise
users would gain a more discoverable but doomed `[Adopt]` on legacy v2
state.json rows.

## Model change: `AdoptEligible` on drift rows

`sync.FileChange` and its `appapi.FileChange` mirror both gain
`AdoptEligible bool`. `classifyDesired` in `internal/sync` populates it only
for `ChangeDrift`, `true` when the state entry for the path carries a
non-empty `AssetID` **and** `SourceRel` (i.e. schema v3 entry). Zero-valued
(`false`) for every other `ChangeKind` and for legacy v2 state entries.

`sync.Apply` continues to return `AdoptUnavailableError` when a `DriftAdopt`
resolution is submitted against an ineligible row — the TUI gating is an
ergonomic prevention, not a correctness relaxation.

## Actions column width

Bumped from 22 to 26 to fit the widest legitimate combination
(`[Open] [Overwrite] [Adopt]` = 25 visible chars, +1 slack). Name column
shrinks by 4 chars at the same terminal width; acceptable per current TUI
sizing rules.

## Trilean-state matrix (canonical spec)

The matrices below are the acceptance-test data set. `[Open]o` is always the
first action button on any file row and is omitted from these tables for
clarity. Buttons are rendered left-to-right in the order shown (natural
severity: Keep < Overwrite/Delete < Adopt).

### Drift row (`ChangeDrift`) with `AdoptEligible == true` — trilean

| Current selection | Button 1     | Mnemonic | Button 2     | Mnemonic |
| ----------------- | ------------ | -------- | ------------ | -------- |
| Keep (default)    | `[Overwrite]` | `w`      | `[Adopt]`    | `t`      |
| Overwrite         | `[Keep]`     | `p`      | `[Adopt]`    | `t`      |
| Adopt             | `[Keep]`     | `p`      | `[Overwrite]` | `w`      |

### Drift row (`ChangeDrift`) with `AdoptEligible == false` — bilean (legacy v2 state)

| Current selection | Button 1     | Mnemonic |
| ----------------- | ------------ | -------- |
| Keep (default)    | `[Overwrite]` | `w`      |
| Overwrite         | `[Keep]`     | `p`      |

No status-column indicator: status stays `* drift`. Legacy v2 entries flip to
v3 on the next re-apply, so the missing `[Adopt]` is transient.

### Unknown row (`ChangeUnknown`) with `OwningAssetID != ""` — trilean

| Current selection | Button 1  | Mnemonic | Button 2 | Mnemonic |
| ----------------- | --------- | -------- | -------- | -------- |
| Keep (default)    | `[Delete]` | `d`      | `[Adopt]` | `t`      |
| Delete            | `[Keep]`  | `p`      | `[Adopt]` | `t`      |
| Adopt             | `[Keep]`  | `p`      | `[Delete]` | `d`      |

### Unknown row (`ChangeUnknown`) with `OwningAssetID == ""` — bilean

| Current selection | Button 1  | Mnemonic |
| ----------------- | --------- | -------- |
| Keep (default)    | `[Delete]` | `d`      |
| Delete            | `[Keep]`  | `p`      |

### Non-drift, non-unknown rows (`ChangeCreate` / `ChangeUpdate` / `ChangeDelete`)

No action buttons beyond `[Open]o`.

## Mnemonics

Reused verbatim from the current single-button implementation so no other
screen needs re-checking:

- `[Keep]` → `p`
- `[Overwrite]` → `w`
- `[Adopt]` → `t`
- `[Delete]` → `d`
- `[Open]` → `o` (always present on file rows)

Every matrix row is pair-wise unique against `[Open]o` + screen-level
`[Apply]a` / `[Show/Hide Ignored]g|h` / `[Back]b`.

## Behavior notes

- Buttons render only on the cursor row (unchanged from today).
- Pressing button N calls `toggleDrift(path, N)` / `toggleUnknown(path, N)`
  directly with the target decision — no cycle-through.
- Pressing the button whose label matches the currently-selected value is
  impossible: that button is never rendered.
- Resolution column ("Current Action") keeps its existing rendering.

## Acceptance Criteria

- [ ] `sync.FileChange` and `appapi.FileChange` gain `AdoptEligible bool`;
      `classifyDesired` sets it to `true` only for `ChangeDrift` whose state
      entry has non-empty `AssetID` and `SourceRel`, and `false` otherwise —
      `TestPlan_DriftAdoptEligibility_TrueOnV3StateEntry`,
      `TestPlan_DriftAdoptEligibility_FalseOnV2LegacyEntry`,
      `TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds`.
- [ ] `driftToggleBtn` is replaced by a factory returning the button set
      dictated by the two Drift matrices (bilean when `AdoptEligible ==
false`, trilean otherwise). Pressing button N calls `toggleDrift(path, N)`
      directly with no cycle.
- [ ] `unknownToggleBtn` returns the button set dictated by the two Unknown
      matrices (bilean when `OwningAssetID == ""`, trilean otherwise).
      Pressing button N calls `toggleUnknown(path, N)` directly with no cycle.
- [ ] `treeActionsFn` appends the returned buttons after `[Open]` unchanged
      in order.
- [ ] Actions column width in `plan_project.go` bumped from 22 to 26.
- [ ] Per-row-state test walks every combination in the four matrices,
      asserts the rendered button labels + mnemonics match exactly and in
      order, and asserts pressing each button sets the drift/unknown
      resolution map to the button's target value —
      `TestPlanProjectRowButtonsMatchMatrix`.
- [ ] Mnemonic-uniqueness test walks every cursor position across every
      row kind (dir/create/update/delete/drift-trilean×3 states/
      drift-bilean×2 states/unknown-owned×3 states/unknown-orphan×2 states)
      and asserts no two active buttons (row + screen-level combined) share
      a mnemonic — `TestPlanProjectMnemonicUniqueness`.
- [ ] `sync.Apply` still returns `AdoptUnavailableError` when a `DriftAdopt`
      resolution is submitted against an `AdoptEligible == false` row —
      existing test coverage from task 0035 remains green.

## Out of scope

- Screen-level button layout changes (Apply / Show Ignored / Back stay).
- Any change to `sync.Apply` semantics beyond keeping the existing
  `AdoptUnavailableError` guard as defense-in-depth.
- Any change to state.json schema, migration, or `app.Service.Apply` write
  path.
- Changing button visuals or the `mnemonic.Button` package itself.
- ADR / arc42 / glossary docs: this is a rendering fix, not an
  architectural decision. The matrices in this file plus the test tables
  are the canonical spec.
- Status-column indicator for `AdoptEligible == false` drift rows: no
  hint is rendered; the missing button is the only signal, and legacy v2
  entries recover on the next re-apply.
- Bilean row expansion: bilean drift (legacy v2) and bilean unknown
  (orphan) keep single-toggle behavior. No extra button.
- Other screens (`EditAsset`, `SelectProjectAssets`, etc.) — no trilean
  toggles exist there today.

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/sync -run 'TestPlan_DriftAdoptEligibility_.*'` green.
- `go test ./internal/tui/shell -run 'TestPlanProjectRowButtonsMatchMatrix|TestPlanProjectMnemonicUniqueness'`
  green.
- Smoke: `./bin/af` → Profiles → project → Plan → v3-state drift row
  visible with `[Open][Overwrite][Adopt]` when Keep-selected; pressing `w`
  sets Overwrite and the button set becomes `[Open][Keep][Adopt]`;
  pressing `t` sets Adopt and the button set becomes
  `[Open][Keep][Overwrite]`.
- Smoke: unknown row inside a known asset dir shows the analogous three
  states; an unknown row outside any known asset dir still shows the
  single bilean `[Keep]`/`[Delete]` toggle.
- Smoke: a drift row backed by a legacy v2 state entry renders only
  `[Open][Overwrite]` (Keep-selected) / `[Open][Keep]` (Overwrite-selected);
  `[Adopt]` is absent; re-applying the project promotes the entry to v3 and
  the next Plan renders the trilean set on the same row.
