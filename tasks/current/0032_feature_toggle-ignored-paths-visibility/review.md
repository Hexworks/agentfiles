# Toggle Ignored Paths visibility — review

Reviewed commit `7087409` ("feat: implement un-ignore") against the must-read guidelines plus the task topics (tui, charm, go). `make fmt/lint/build/test` all pass (528 tests green). **No security findings** — the replace-semantics switch keeps `validatePathKey`, the managed-surface fence, plan-before-apply, and the `assertIgnoredRegisterable` (incoming − prior) gate all intact; a pure ignore-set Apply cannot bypass validation.

The implementation is correct and well-commented. The findings below are design/maintainability and documentation issues, plus three test-coverage gaps. The dominant theme — flagged independently by the DDD, clean-architecture, clean-code, and SOLID reviewers — is that the desired-ignored-set algebra now lives in the TUI. The single highest-value fix is the safety-rule test gap (issue 4), because the safety rule exists precisely to catch a future mnemonic-rebind regression that the current walk cannot.

Each issue below offers a checklist. **Tick exactly one box per issue** (including a "Leave as-is" box where offered), then return.

## Desired ignored-set algebra lives only in the TUI

> [!WARNING]
>
> - docs/guidelines/domain_model.md ("Put Domain Rules In Domain Code")
> - docs/guidelines/clean_architecture.md (Common Closure: code that changes together lives together)
> - docs/guidelines/clean_code.md (Functions: one level of abstraction)

The rule that defines the complete persisted ignored set — `final = (persisted − unignored) ∪ live-ignored` — is computed inside `onApply` at `internal/tui/shell/plan_project.go:865-874`. `sync.Apply` was deliberately reduced to a verbatim writer (`normalizeIgnoredPaths`, `internal/sync/sync.go:384-389`) and `app.Apply` just forwards `r.IgnoredPaths`. So the one piece of genuine domain reconciliation logic — what the ignored set _should be_ after an un-ignore — exists only inside a Bubble Tea screen, reconstructed from three TUI-private maps, and is exercised only by a TUI test. Notably `assertIgnoredRegisterable` (`internal/app/service.go:404-426`) already recomputes `incoming − prior` defensively in the app layer, so the two halves of the same rule are split across layers.

This is defensible under ADR 0010's "the TUI assembles resolution sets" precedent (the parallel drift/unknown loop directly above does the same), so it is a placement question, not a bug.

```go
// internal/tui/shell/plan_project.go:865 — domain set-algebra in the view layer
desired := map[string]bool{}
for _, p := range s.persistedIgnored {
    if !s.unignored[p] { desired[p] = true } // (persisted − unignored)
}
for p := range s.ignoredPaths { desired[p] = true } // ∪ live-ignored
ignored := slices.Sorted(maps.Keys(desired))
```

Choose one:

- [x] Lift the formula into a pure `app` helper (e.g. `app.DesiredIgnored(persisted, unignored, newlyIgnored []string) []string`) that the screen calls, so the policy lives in the stable layer and is testable without bubbletea.
- [ ] Keep it TUI-side but extract a `func (s *planProjectScreen) desiredIgnoredPaths() []string` method (mirroring `visiblePersistedIgnored`) and add a focused unit test on the set algebra, so `onApply` reads as orchestration and the rule isn't guarded only end-to-end.
- [ ] Leave as-is — accept the ADR-0010 resolution-assembly precedent and take no action.

## Duplicated live-ignored vs persisted-ignored toggle machinery

> [!WARNING]
>
> - docs/guidelines/clean_code.md (Code Smells: needless repetition that can drift apart)
> - docs/guidelines/solid.md (Single Responsibility / Open-Closed)

The persisted-ignored path duplicates the live-unknown toggle machinery. `showFolderBtn` (`plan_project.go:609`) and `unignorePersistedBtn` (`:616`) are identical except for the handler — same `"Show"` label, same `'w'` mnemonic, same rationale comment; likewise `ignoreFolderBtn`/`reignorePersistedBtn` (`:602`/`:622`). The handler bodies `unignorePersisted`/`reignorePersisted` (`:697-712`) mirror `toggleIgnore` (`:682-691`), differing only in which map they mutate and `RefreshActions` (leaf, no shape change) vs `rebuildTree` (live, shape changes). The `'w'`/`'i'` mnemonics are now hard-coded in four button constructors — a future rebind must edit all four or the paths silently diverge, which is exactly what the mnemonic-uniqueness safety rule guards against.

```go
func (s *planProjectScreen) showFolderBtn(dirPath string) *mnemonic.Button {
    return mnemonic.New("Show", 'w', func() tea.Cmd { return s.toggleIgnore(dirPath, false) })
}
func (s *planProjectScreen) unignorePersistedBtn(dirPath string) *mnemonic.Button {
    return mnemonic.New("Show", 'w', func() tea.Cmd { return s.unignorePersisted(dirPath) }) // byte-identical but for handler
}
```

Choose one:

- [x] Unify both pairs behind one handler `setIgnored(path string, ignored, persisted bool)` that branches on `persisted` for the rebuild-vs-refresh choice, plus one button factory taking the target handler — single home for label+mnemonic+rationale.
- [ ] Keep the two paths but extract shared `'w'`/`'i'` mnemonic constants (and consolidate the rationale comment to one place) so a label/key change is a single edit.
- [ ] Leave as-is — the divergence (rebuild vs refresh) is real and the duplication is shallow.

## Glossary and docs contradict the new replace semantics

> [!WARNING]
>
> - docs/guidelines/domain_model.md ("update the glossary when a durable domain term appears"; don't let docs and code drift)

The commit flips `ignored_paths` from union to replace and documents it in ADR 0010, but `docs/glossary.md:145-147` still asserts "On apply the list is **unioned** with the previously-persisted paths, never dropped" — now exactly backwards. Separately, three durable terms this task introduces — **un-ignore**, **persisted-ignored**, **pinned** — appear nowhere in the glossary, even though the existing `Resolution` / `Ignored Path` entries cross-reference each other.

```text
docs/glossary.md:146 (stale):  "the list is unioned with the previously-persisted paths, never dropped"
internal/sync/sync.go:384      normalizeIgnoredPaths → "writes the incoming ignored set verbatim (replace, not merge)"
```

Choose one:

- [x] Rewrite the `Ignored Path` glossary entry for replace semantics (TUI sends the complete desired set; Apply writes it verbatim; dropping a key un-ignores), cross-reference ADR 0010's task-0032 addendum, **and** add a definition for **un-ignore** (note persisted-ignored / pinned as TUI-state terms).
- [ ] Fix only the factually-wrong union→replace sentence now; defer the new-term glossary entries to a follow-up.
- [ ] Leave as-is.

## Mnemonic safety-walk never exercises the `g` (hidden) state with a visible row

> [!WARNING]
>
> - docs/guidelines/testing.md (the task SAFETY RULE: "walk every selection state and assert mnemonic uniqueness")

`TestPlanProjectScreen_MnemonicUniquenessOnPersistedIgnoredRow` (`plan_project_test.go:946-973`) loops both row states (`[Show]`/`[Ignore]`) but calls `s.toggleShowIgnored()` (line 951) in **both** iterations, so the screen toggle is always `Hide Ignored`/`h`. The `g` candidate is never registered while a persisted-ignored `w`/`i` row is the cursor row — yet that state is reachable in production: press `[Show]` (pins the row), then `[Hide Ignored]` flips the button back to `g` while the pinned row stays visible (the scenario in `TestPlanProjectScreen_ShownRowStaysPinnedAfterHide:827`). The collision is benign today but the safety rule exists to catch a future rebind regression, and the walk as written only ever exercises `h`.

```go
s.unignorePersisted("zsub") // pin
s.toggleShowIgnored()       // hide -> button is now [Show Ignored]/'g'
// land cursor on the still-pinned zsub row, then:
s.rebuildSet()
assertUniquePlanMnemonics(t, s.set, /*state*/2, row) // g + w (or i) + a + b all present
```

Choose one:

- [x] Add a third loop arm (or sibling test) that pins a row, calls `toggleShowIgnored()` back to the `g` state, lands the cursor on the pinned `w`/`i` row, and asserts uniqueness.
- [ ] In the existing loop, assert `s.showIgnoredBtn` is `'g'` for one iteration and `'h'` for the other so both screen-toggle runes are walked alongside a row button.

## Missing direct assertions for three task-spec behaviors

> [!WARNING]
>
> - docs/guidelines/testing.md (Test One Behavior At A Time)

Three behaviors the description pins by name lack a direct assertion: (1) the toggle button label/mnemonic flip `Show Ignored`/`g` ⇄ `Hide Ignored`/`h` (`refreshShowIgnoredBtn`, `plan_project.go:224-230`) — no test reads the button's label/mnemonic across states; (2) the `! ignored` row's Resolution column being blank (`actionValue` returns `""` for the dir node) — `TestPlanProjectScreen_StatusForPersistedIgnoredDir:873` pins only Status; (3) `assertIgnoredRegisterable` exempting an already-persisted key while validating a newly-added one _under replace semantics_ — no dedicated test (covered only indirectly). All three behave correctly; they're just not guarded.

```go
assertBtn(t, s.showIgnoredBtn, "Show Ignored", 'g') // default hidden
s.toggleShowIgnored()
assertBtn(t, s.showIgnoredBtn, "Hide Ignored", 'h') // shown
```

Choose one:

- [x] Add all three assertions (button-flip unit test, blank-Resolution assertion on the persisted-ignored dir, and a replace-semantics exemption test for `assertIgnoredRegisterable`).
- [ ] Add only the `assertIgnoredRegisterable` replace-semantics exemption test (the safety-relevant one); skip the two cosmetic-column assertions.
- [ ] Leave as-is — behaviors are exercised indirectly.

## `previewFromSync` aliases the domain `IgnoredPaths` slice

> [!WARNING]
>
> - docs/guidelines/go.md (keep I/O at the edges; the app layer mirrors the domain — cf. the element-by-element `Changes` copy)

`previewFromSync` (`internal/app/service.go:308-318`) copies `Changes` into a fresh slice but assigns `IgnoredPaths` by direct reference (`ignored = p.ManagedState.IgnoredPaths`), so `app.Preview.IgnoredPaths` aliases the live `ManagedState` backing array that `Apply` still reuses. No active bug — the TUI clones in `handleLoaded` before sorting — but it's the aliasing footgun the sibling `Changes` copy deliberately avoids: any future caller that sorts/appends in place would corrupt domain state. The related `handleLoaded` clone-and-sort (`plan_project.go:383-384`) uses `append([]string(nil), ...)` then `slices.Sort`, slightly inconsistent with the `slices.Sorted` idiom used elsewhere in the same file.

```go
var ignored []string
if p.ManagedState != nil {
    ignored = p.ManagedState.IgnoredPaths // aliases domain slice; Changes (above) is copied
}
```

Choose one:

- [x] Clone at the boundary: `ignored = slices.Clone(p.ManagedState.IgnoredPaths)`, matching the `Changes` treatment so the app mirror owns its backing array.
- [ ] Keep the reference but add a one-line comment that `Preview.IgnoredPaths` aliases domain state and is read-only to callers.
- [ ] Leave as-is.

## `buildPlanTree` injected-leaf: unstated invariant + construct-then-overwrite

> [!WARNING]
>
> - docs/guidelines/go.md (make domain assumptions explicit)
> - docs/guidelines/clean_code.md (Data/Objects: avoid hybrids with hidden mode bits)

In the merged build loop (`plan_project.go:935-941`) an injected ignored key is always appended as a brand-new terminal child, keyed only by its last segment, never consulting the `dirs` dedup map. If an injected key `a/b` ever coincided with a real dir `a/b` (because a change row `a/b/c.md` exists) the tree would gain a duplicate node. This cannot happen — `sync.Plan`'s `isUnderIgnored` suppresses the whole subtree of a persisted-ignored folder — but the load-bearing invariant is silent and untested for the collision case. Separately, the leaf is built as `planNodeFile` then _reconstructed_ as a `planNodeDir{persistedIgnored:true}` when `it.ignored`, leaving a dead value and forcing three readers (`statusValue`, `statusStyle`, `treeActionsFn`) to branch on the mode bit inside their dir case.

```go
leaf := planNode{kind: planNodeFile, path: it.path, change: it.change}
if it.ignored {
    leaf = planNode{kind: planNodeDir, path: it.path, persistedIgnored: true} // dead value above; no dirs[] collision check
}
parent.Children = append(parent.Children, &treetable.Node{Label: part, Data: leaf})
```

Choose one:

- [ ] Add a one-line comment at the injection site stating the relied-on invariant (Plan suppresses the injected subtree, so no change row collides), and build the two node kinds in an `if/else` so neither value is dead.
- [x] Above plus register the injected leaf in `dirs[it.path]` and skip if present, so a future change to Plan's suppression cannot produce a duplicate node.
- [ ] Leave as-is.

## Stale `StatusKeys` doc comment lists wrong mnemonics

> [!WARNING]
>
> - docs/guidelines/clean_code.md (Comments: intent must stay accurate)

`StatusKeys` (`plan_project.go:422-427`) carries the comment "exposes the cursor row's toggle mnemonic (o, k, or d)". After 0031/0032 the cursor row can host `w`, `i`, `r`, `p` in addition to `o`/`d`, and `k` is not a mnemonic any button uses. The comment predates this commit but this task is what made the row-mnemonic set diverge from it.

```go
// StatusKeys exposes the cursor row's toggle mnemonic (o, k, or d) plus  <- now factually wrong
```

Choose one:

- [x] Replace the parenthetical with a non-enumerated phrasing ("the cursor row's action mnemonics") so it can't drift again as row buttons grow.
- [ ] Leave as-is — pre-existing, out of this commit's strict scope.
