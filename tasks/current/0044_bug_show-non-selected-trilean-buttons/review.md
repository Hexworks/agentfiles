# Show non-selected trilean options as row buttons — review

Task 0044 renders both non-selected trilean options as separate row buttons
(replacing the single cycling button) and plumbs a new `AdoptEligible bool`
through `sync.FileChange` → `appapi.FileChange` → the TUI so drift rows on
legacy v2 state degrade to a bilean toggle.

Build/test/lint gate is green. All Definition-of-Done criteria trace to real
diff hunks; every hunk is justified by a criterion, an explicit description
matrix, or a documented "kept as smoke" decision from `plan.md` Step 7. No
security findings. The change is small and mechanical; findings are cohesion
/ naming / test-cleanup grade — none block merge.

Findings, roughly by impact:

1. Drift branch reads eligibility from the DTO, unknown branch reads it from
   a session-local map — two co-changing rules, two mechanisms.
2. The two `driftToggleButtons` / `unknownToggleButtons` factories are
   structural twins with hand-enumerated cases.
3. `AdoptEligible` is named and documented in schema-history terms
   (`v3 provenance`, `legacy v2`) at the boundary DTO.
4. `AssetID != "" && SourceRel != ""` is now literally duplicated across two
   `sync` sites (`classifyDesired` + `classifyDriftAdopt`).
5. Drift ships a derived bool (`AdoptEligible`); unknown ships the actual
   identifier (`OwningAssetID`) — same underlying idea, two shapes.
6. Four `TestPlanProjectScreen_TreeActionsFn*Renders*` per-state tests plus
   the older `MnemonicUniquenessExhaustive` are subsets of the new matrix
   walker / mnemonic sweep. Plan Step 7 asked to decide during
   implementation; they were left in.
7. `assertUniqueCaseInsensitiveMnemonics` and `assertUniquePlanMnemonics`
   run after `rebuildSet`, which panics on duplicate — the assertions can
   never observe a duplicate.
8. In `TestPlanProjectMnemonicUniqueness`, `kindOf` returns
   `kindRegisterableDir` for every non-persisted-ignored dir but the `ok`
   flag is only true for two hardcoded paths — the first return is
   misleading and a path-rename silently drops the row from the sweep.
9. `TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds` bundles four
   fixture assertions in one function; a create-branch failure leaves the
   update/unknown/delete branch still running with mixed reporting.
10. Manual ASCII case-fold in the test helper instead of `unicode.ToLower`.

Tick exactly one checkbox per issue for the solution you want applied.

## Asymmetric adopt-eligibility source at the two `treeActionsFn` branches

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — Common Closure Principle
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — "Similar concepts should look similar"

`treeActionsFn` reads the "row can offer Adopt" bit from two different
sources for the two kinds:

```go
// internal/tui/shell/plan_project.go:620-624
case appapi.ChangeDrift:
    btns = append(btns, s.driftToggleButtons(d.path, d.change.AdoptEligible)...)
case appapi.ChangeUnknown:
    btns = append(btns, s.unknownToggleButtons(d.path, s.unknownOwners[d.path] != "")...)
```

Both changed in this commit for the same reason (moving to the bilean vs.
trilean rendering). Drift reads the boundary field; Unknown reaches back into
screen-local `unknownOwners`. But `d.change.OwningAssetID` is already
populated on the same change (`appapi.FileChange` field, seeded in
`sync.Plan`), and `unknownOwners` is itself just a projection of that field
built at load time (`plan_project.go:392-397`). Both branches could read the
same shape.

`unknownOwners` still earns its keep elsewhere as a map keyed by path; this
is a call-site cleanup, not a map removal.

### Solutions

- [x] Change the unknown branch to `s.unknownOwners[d.path]` → `d.change.OwningAssetID != ""` so both branches read the DTO. One-line edit; call sites now look and read alike.
- [ ] Introduce a `func (ch appapi.FileChange) adoptEligible() bool` that returns `ch.AdoptEligible` for drift and `ch.OwningAssetID != ""` for unknown; both branches call it. Concentrates "is Adopt available for this row?" in one place.
- [ ] Leave as-is — the two sources are documented and the coupling risk is small.

## Twin factories hand-enumerate the "which two to render" table

> [!WARNING]
>
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) — Open/Closed Principle
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — "Needless repetition"

`driftToggleButtons` (`internal/tui/shell/plan_project.go:657-672`) and
`unknownToggleButtons` (`internal/tui/shell/plan_project.go:690-705`) are
structural mirror twins. Same bilean fallback, same severity-ascending
ordering (Keep < middle < Adopt), same "return the two non-selected options"
rule. The only variation is the middle option (Overwrite vs. Delete) and the
decision enum type. A future policy change — adding a fourth decision,
hiding an option in a new state — has to be applied identically in both.

```go
// drift
switch current {
case appapi.DriftOverwrite:
    return []*mnemonic.Button{s.driftBtnKeep(path), s.driftBtnAdopt(path)}
case appapi.DriftAdopt:
    return []*mnemonic.Button{s.driftBtnKeep(path), s.driftBtnOverwrite(path)}
}
return []*mnemonic.Button{s.driftBtnOverwrite(path), s.driftBtnAdopt(path)}
```

Two 15-line procedures side-by-side; both restate the "drop the third option
when ineligible" rule.

### Solutions

- [x] Extract `trileanToggleButtons[T comparable](current T, keep, mid, adopt func() *mnemonic.Button, eligible bool) []*mnemonic.Button` and call from both factories. Closes both rules under one predicate; a fourth decision becomes one entry.
- [ ] Table-drive it: `[]struct{decision D; btn func() *Button}` in severity order; both factories become `filter(all, != current)` with a slice trim when `!adoptEligible`.
- [ ] Leave as-is — two mirrors are short enough that the parallelism is legible, and no fourth decision is on the roadmap.

## `AdoptEligible` name and doc are pinned to schema history at the boundary

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — Ubiquitous Language
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) — Stable Abstractions Principle

Two related name/doc slippages:

1. **Name.** `AdoptEligible` reads as a _permission_ check. The domain
   already uses "eligible" for a different concept (asset-registration
   eligibility, `docs/glossary.md:205-211`). What this field actually
   encodes is _provenance completeness_ — the v3 state entry has both
   `AssetID` and `SourceRel` so `applyDrift`'s reverse-mapping can resolve
   a target. ADR 0020 calls that pair the **provenance keys**. The
   existing failure message on the Apply side is `"legacy v2 state entry
missing asset provenance"` (`internal/sync/sync.go` `AdoptUnavailableError`
   reason). Field name and Apply-error reason should share a stem.

2. **Boundary doc.** The `appapi.FileChange.AdoptEligible` doc
   (`internal/appapi/appapi.go:72-77`) documents its semantics in terms of
   "v3 provenance" — a `sync`-internal schema concept. Compare
   `OwningAssetID` (line 68) which describes its purpose in TUI-facing
   terms ("gates the row-level Adopt action") without naming a schema
   version. The boundary DTO should describe _what the consumer can do_,
   not _how the producer computes it_.

### Solutions

- [x] Rename to `HasAdoptProvenance` on both `sync.FileChange` and `appapi.FileChange` — names the actual fact, aligns with `AdoptUnavailableError`'s wording, drops the collision with "eligible" as a permission concept.
- [ ] Rename to `AdoptAvailable` on `appapi.FileChange` only, keep `AdoptEligible` on `sync.FileChange` — makes the DTO action-centric while leaving the domain type unchanged.
- [ ] Keep the name but rewrite only the `appapi.FileChange.AdoptEligible` doc comment in TUI-facing terms ("true when the row may offer an [Adopt] button; false means the button must be hidden") — smallest edit, keeps the schema-history explanation on the `sync` side.
- [ ] Leave both name and doc unchanged.

## `AssetID != "" && SourceRel != ""` predicate duplicated across two `sync` sites

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) — Duplication
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — Model behavior on the entity that owns the data

`classifyDesired` sets `AdoptEligible` (`internal/sync/sync.go:353`) and
`classifyDriftAdopt` returns `AdoptUnavailableError` on the same predicate
(`internal/sync/sync.go:598-599`). Both live inside `sync` and both check
`entry.AssetID != "" && entry.SourceRel != ""` on a `ManagedFileEntry`. If
schema v4 changes what "adopt-ready" means, both sites drift.

### Solutions

- [x] Add a method on `ManagedFileEntry`: `func (e ManagedFileEntry) HasAdoptProvenance() bool { return e.AssetID != "" && e.SourceRel != "" }`. Both `sync` sites call it; the TUI-visible flag is set from the same source of truth.
- [ ] Extract a package-level `hasAdoptProvenance(entry ManagedFileEntry) bool` helper — same effect, less binding to the type.
- [ ] Leave as-is — two identical two-term predicates are not yet costly.

## Drift ships a derived bool, unknown ships the actual owner ID

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) — Concept Boundaries / Keep Persistence a Detail

Both `ChangeDrift` and `ChangeUnknown` gate a possible `Adopt` action on
provenance, but the two cases carry that provenance on `FileChange`
differently:

- `ChangeUnknown` — `OwningAssetID string` (`sync.go:214-218`), the actual
  provenance datum. TUI projects it into a separate `unknownOwners`
  map.
- `ChangeDrift` — `AdoptEligible bool` (`sync.go:219-226`), a derived
  predicate over `(entry.AssetID, entry.SourceRel)` computed once in
  `classifyDesired`.

The bool discards information the domain has. `classifyDriftAdopt` later
re-reads `entry.AssetID` / `entry.SourceRel` from state anyway (sync.go:598)
— the TUI is only ever told "yes/no" and cannot show, for example, the
target asset in a tooltip or verify against a re-plan.

### Solutions

- [x] Replace `AdoptEligible bool` with `AdoptSource *ManagedFileEntry` (or a narrower `AdoptProvenance struct{AssetID, SourceRel string}`) populated only when both keys are non-empty. Symmetric with `OwningAssetID`, keeps future needs open.
- [ ] Add `OwningAssetID string` on drift rows too (populated from `entry.AssetID` for v3 entries); TUI tests `ch.OwningAssetID != ""` uniformly for drift and unknown. Cheapest unification; loses `SourceRel`.
- [ ] Keep `AdoptEligible bool` as-is — the TUI never needs the underlying identifiers today; add them only when a real caller does.

## Redundant per-state / per-matrix tests after adopting the matrix walker

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Test one behavior at a time; test code should be boring

The plan (Step 7) explicitly asked to "decide during implementation" whether
to keep or drop these; they were kept. Concretely:

- `TestPlanProjectScreen_TreeActionsFn{Drift,Unknown}{Bilean,Trilean}KeepRenders...`
  (`plan_project_test.go:269-331`) — four tests that each cover exactly one
  row of the four matrices (always the `Keep` state).
- `TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset`
  (`plan_project_test.go:450`) — one owned + one orphan Keep-state row.
- `TestPlanProjectScreen_ToggleUnknownSwapsState`
  (`plan_project_test.go:513`) — orphan Keep→Delete→Keep.
- `TestPlanProjectScreen_MnemonicUniquenessExhaustive`
  (`plan_project_test.go:778`) — 5 rows, drift trilean only, no orphan
  unknown, case-sensitive helper.

Each of these is a subset of `TestPlanProjectRowButtonsMatchMatrix` /
`TestPlanProjectMnemonicUniqueness`. The changelog mentions the retention
was deliberate; the tests still work. But they lock the factory shape twice
and add maintenance surface with no new signal.

### Solutions

- [x] Delete all four narrow `TreeActionsFn*Keep*Renders*` tests + `UnknownAdoptShownOnlyWhenOwnedByAsset` + `ToggleUnknownSwapsState` + `MnemonicUniquenessExhaustive` (with helper `assertUniquePlanMnemonics`). Note the deletion in the changelog per plan Step 7.
- [ ] Keep one intentionally-named smoke test (e.g. `TestPlanProjectScreen_DriftKeepRendersOpenAndOverwriteBtns`) with a comment `// smoke; full coverage in TestPlanProjectRowButtonsMatchMatrix`, delete the others.
- [ ] Leave all as-is — the redundancy is cheap and per-state failures name themselves.

## `assertUniqueCaseInsensitiveMnemonics` / `assertUniquePlanMnemonics` can never observe a duplicate

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Assert behavior, not mock mechanics

`mnemonic.Set.Add` panics on any case-insensitive duplicate
(`internal/tui/components/mnemonic/set.go:51-62`); `rebuildSet` calls
`set.Add` for every button. Any duplicate that `rebuildSet` would produce
triggers the panic before the assertion runs. The
`assertUniqueCaseInsensitiveMnemonics` call at `plan_project_test.go:1393`
and the older `assertUniquePlanMnemonics` at line 836 can never observe a
duplicate — the code path never reaches them in the failure case.

The load-bearing part of `TestPlanProjectMnemonicUniqueness` is the
cursor walk + `saw`-map (line 1401-1410) proving every row kind was
visited. That part is real.

### Solutions

- [x] Drop `assertUniqueCaseInsensitiveMnemonics` and `assertUniquePlanMnemonics`; add a one-line comment on the `rebuildSet` call ("`Set.Add` panics on duplicate — walk asserts the panic never fires"). Retains the coverage sweep, loses dead helpers.
- [ ] Keep the helpers but relabel them "defense-in-depth" with a comment explaining that `Set.Add` should have panicked first.
- [ ] Leave as-is — the helpers are cheap and their names are legible.

## `TestPlanProjectMnemonicUniqueness` `kindOf` return is misleading

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Test code should be boring

```go
// internal/tui/shell/plan_project_test.go:1337-1343
if d, ok := planDirNode(n); ok {
    if d.persistedIgnored {
        return kindPersistedIgnored, true
    }
    return kindRegisterableDir, d.path == "reg" || d.path == "reg/only-unknown"
}
```

Every non-persisted plain dir returns `kindRegisterableDir` as the first
value; only two hardcoded paths make the `ok` flag true. The caller
(`if k, ok := kindOf(n); ok`) silently discards the `false` cases, so a
path rename (`"reg"` → `"regdir"`) drops the row from the sweep without
failing any test. `kindRegisterableDir` also isn't in the `want` list
(line 1401-1406), so the second predicate is not load-bearing — it exists
only to distinguish two "registerable-esque" dirs from every other dir.

### Solutions

- [x] Add `kindRegisterableDir` to the `want` list so the predicate is load-bearing; the walk already forces `s.registerableDirs["reg"] = true`, so this is achievable and turns silent misses into failures.
- [ ] Return `(0, false)` instead of `(kindRegisterableDir, cond)` when the path does not match — makes the "not counted" branch obvious.
- [ ] Split `kindOf` into `dirKind` / `fileKind` so dir vs. registerable-dir vs. persisted-ignored branches are explicit at the call site.

## `TestPlan_DriftAdoptEligibility_ZeroOnNonDriftKinds` bundles four assertions in one func

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) — Test one behavior at a time

`internal/sync/sync_test.go:1368-1431` mixes:

- A `{}`-scoped `ChangeCreate` fixture (line 1371-1385).
- A separate temp-dir fixture producing `ChangeUpdate` + `ChangeUnknown` +
  `ChangeDelete` (line 1390-1431).

If the create-branch fails on line 1382, the update/unknown/delete branch
still runs with mixed reporting (the inner block-scope's temp-dir hides
the setup). All four assertions are legitimate; they'd read cleaner as
four `t.Run` subtests. (Positive: all sync tests hit the real `Plan`
code path with real profile.Init/Load + real state on disk — no fakes.)

### Solutions

- [x] Split into four `t.Run` subtests keyed by ChangeKind — each with its own `t.TempDir()` and setup, individual failures name themselves.
- [ ] Leave as-is — the test is short enough that failures are still readable.

## Manual ASCII case-fold in `assertUniqueCaseInsensitiveMnemonics`

> [!WARNING]
>
> - [`docs/guidelines/go_guidelines.md`](../../../docs/guidelines/go_guidelines.md) — Prefer stdlib idioms in test helpers

```go
// internal/tui/shell/plan_project_test.go:1419-1422
low := r
if r >= 'A' && r <= 'Z' {
    low = r + ('a' - 'A')
}
```

Works because mnemonics are ASCII by contract, but `unicode.ToLower(r)` is
the standard-library idiom. Reader does not have to reason about ranges or
the `'a' - 'A'` offset; the intent is stated in the function name.

### Solutions

- [x] Replace with `low := unicode.ToLower(r)` and add `"unicode"` to imports.
- [ ] Leave as-is with a one-line `// mnemonics are ASCII by contract` comment.
- [ ] Leave untouched — the behavior is correct.
