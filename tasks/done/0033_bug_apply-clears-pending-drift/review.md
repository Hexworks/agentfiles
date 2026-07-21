The core fix (Apply's `ChangeDrift` Keep branch preserving the prior baseline;
`onApply` no longer materializing an explicit `DriftKeep`) is correct, well
tested, and matches ADR 0015. Security and SOLID passes returned NO FINDINGS.

The remaining review signal falls in three buckets:

1. **Doc-comment hygiene inside the touched Go files** — a few block
   comments are inaccurate (the pre-fill comment describes a
   never-taken branch), too long (the `DriftKeep` doc comment), or now
   subtly stale (`DriftDecision` zero-value gloss). Project style
   already says "don't reference the current task, fix, or callers" in
   code — the bug/task numbers leaked into the code comments trip that
   rule.
2. **One real correctness hazard beyond the diff** — the new Keep
   branch trusts `preview.ManagedState.ManagedFiles[change.Path]` to be
   non-empty, but Go's map zero-value is `""`. If the drift invariant
   in `classifyDesired` ever changes, the empty string quietly poisons
   `recordedHashes`, and the next `Plan` will emit `ChangeDelete` /
   `ChangeUnknown` for the path. Cheap guard fixes it.
3. **Documentation twins are out of sync** — the two rendered
   `af.drift-cleanup` skill glossaries under `.claude/` and `.codex/`
   still teach the pre-fix "DriftKeep adopts on-disk hash" meaning,
   and the main `docs/glossary.md` Drift entry still cites `ADR 0010`
   as its sole authority even though the entry describes the ADR
   0015 semantics.

Minor test-quality improvements round out the list. No blocker for the
task's Definition of Done — the DoD gate passed all seven criteria and
`make build && make test && make lint` is green.

Tick exactly one checkbox per issue for the fix you want applied. An
"accept as-is" option is included where the finding is a judgment
call rather than a defect.

## Pre-fill block comment misdescribes when the switch mutates `recordedHashes`

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Comments #2 / #3 ("Use comments to explain intent" / "Do not repeat what the next line already says").

The comment above `recordedHashes := map[string]string{}` in
`internal/sync/sync.go:304-310` reads:

> The switch below only needs to overwrite paths where the on-disk
> content diverges (kept drift preserves the prior baseline per ADR
> 0015); clean files (not in Changes) retain their rendered hash…

But the switch never overwrites paths "where the on-disk content
diverges." `ChangeCreate`/`ChangeUpdate` and `ChangeDrift+Overwrite`
all _leave the pre-fill alone_ because their write brings on-disk to
match the rendered body. The _only_ branch that mutates
`recordedHashes` is `ChangeDrift+DriftKeep` — where we substitute the
prior baseline for the rendered hash. The parenthetical about "clean
files" further conflates that with paths absent from `Changes`.

```go
// internal/sync/sync.go:304-315
// Pre-fill the state baseline with the rendered hash of every
// desired file. The switch below only needs to overwrite paths
// where the on-disk content diverges (kept drift preserves the
// prior baseline per ADR 0015); clean files (not in Changes)
// retain their rendered hash so drift detection still works on
// the next plan.
recordedHashes := map[string]string{}
for _, f := range preview.Files {
    bodiesByPath[f.Path] = f
    recordedHashes[f.Path] = utils.HashBytes(f.Body)
}
```

- [x] Reword to name the single mutating branch: e.g. "Pre-fill the
      state baseline with the rendered hash of every desired file. This
      is the correct baseline for clean, created, updated, and
      overwritten paths; only the `ChangeDrift`/`DriftKeep` branch
      below overrides it with the prior baseline (ADR 0015)."
- [ ] Keep the pre-fill comment focused on what the loop does (records
      rendered-hash defaults for every desired file) and move the "why
      Drift+Keep is the sole override" note to the drift branch itself.

## `DriftKeep` / `Apply` doc comments carry transient bug and task numbers

> [!WARNING]
>
> - [CLAUDE.md](../../../CLAUDE.md) — "Don't reference the current task, fix, or callers ('used by X', 'added for the Y flow', 'handles the case from issue #123'), since those belong in the PR description and rot as the codebase evolves."
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Comments #4 / Code Smells #4.

Two code sites mention `bug 0033` (transient) and `task 0035`
(transient) in prose alongside the durable `ADR 0015` pointer:

- `internal/sync/sync.go:105-113` — the `DriftKeep` doc comment. Nine
  lines of prose that duplicate ADR 0015's Decision section plus a
  future-work note about task 0035's Adopt operation.
- `internal/sync/sync.go:336-341` — the block comment inside the
  `Apply` `ChangeDrift` branch. Also cites `bug 0033` alongside the
  `ADR 0015` pointer.

Both are the exact pattern CLAUDE.md forbids: task/bug numbers that
belong in the PR description and the ADR, not in evergreen code.

```go
// internal/sync/sync.go:105-113
// DriftKeep leaves the on-disk content untouched and preserves the
// prior managed baseline, so a kept drift stays classified as drift
// on every subsequent plan until the user overwrites it or the
// profile/file converge. Adopting the on-disk hash as the new
// baseline is intentionally not supported: doing so silently flips
// drift to update on unrelated Applies (bug 0033). "No decision"
// (path absent from Resolutions.Drift) is identical to DriftKeep.
// Promoting local edits into the profile is a separate future
// operation (Adopt, task 0035). See ADR 0015.
DriftKeep DriftDecision = "keep"
```

- [x] Trim `DriftKeep`'s doc to the invariant + one ADR pointer: e.g.
      "`DriftKeep` leaves the on-disk content untouched and preserves
      the prior managed baseline. 'No decision' (path absent from
      `Resolutions.Drift`) is identical. See ADR 0015." Then trim the
      Apply switch block comment (lines 336-341) the same way — keep
      the `ADR 0015` pointer, drop the `bug 0033` reference.
- [ ] Only strip the `bug 0033` / `task 0035` numeric references and
      leave the prose otherwise intact (less aggressive; preserves the
      current level of self-explanatory local context).

## `DriftDecision` zero-value comment says "keep the local edits" without mentioning the baseline

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Understandability #1 ("Make intent visible") + Comments #6 ("Comment non-obvious invariants").
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language" (glossary/code alignment).

`internal/sync/sync.go:97-100` defines `DriftDecision` with the zero-value
gloss "no explicit choice, keep the local edits." That wording survived
from the old semantics where Keep also adopted the on-disk hash. Under
ADR 0015 the crucial fact is that the _baseline_ is also preserved —
that's the whole point of the fix. A reader who lands on `DriftDecision`
first (before `DriftKeep`) gets a subtler picture than the one on
`DriftKeep` itself.

```go
// internal/sync/sync.go:97-100
// DriftDecision is the user's per-file choice for a ChangeDrift entry.
// The zero value (empty string) means "no explicit choice, keep the
// local edits" and is what Apply assumes when a path is missing from
// the resolutions slice.
type DriftDecision string
```

- [x] Align with the `DriftKeep` comment: "The zero value (empty
      string) is identical to `DriftKeep`: leave the on-disk content
      and the prior baseline alone."
- [ ] Drop the semantic gloss and defer to `DriftKeep`'s doc comment:
      "The zero value (empty string) is identical to `DriftKeep`."

## Empty-string in `preview.ManagedState.ManagedFiles[change.Path]` can poison the baseline

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Prefer Explicit Types Over Loose Maps" (sentinel conflated with absence).
> - [docs/guidelines/sync_and_safety.md](../../../docs/guidelines/sync_and_safety.md) — "Keep Managed State Accurate."

`internal/sync/sync.go:342-343` writes
`recordedHashes[change.Path] = preview.ManagedState.ManagedFiles[change.Path]`.
Go returns the zero value `""` if the key is missing. Today
`classifyDesired` at `internal/sync/sync.go:268` protects the invariant

```go
if state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash {
    return FileChange{Path: file.Path, Kind: ChangeDrift, ...}
}
```

so a `ChangeDrift` row implies a non-empty baseline. The code comment
above the Keep branch calls out the `state != nil` invariant but does
not call out the equally-load-bearing "baseline non-empty" invariant.

If that invariant is ever loosened, `""` poisons `recordedHashes`, and
downstream Plan calls quietly misclassify:

- `internal/sync/sync.go:535` — `detectDeletesAndUnknowns` iterates
  map keys, so a key with value `""` still triggers `ChangeDelete` on
  the next Plan (the file gets scheduled for deletion).
- `internal/sync/sync.go:601` — `classifyDeleteOrUnknown` treats
  `state.ManagedFiles[rel] == ""` as absent, so the file surfaces as
  `ChangeUnknown` during the walk.

```go
// internal/sync/sync.go:342-346
if preview.ManagedState != nil {
    recordedHashes[change.Path] = preview.ManagedState.ManagedFiles[change.Path]
    // silently writes "" if the key is missing
} else {
    delete(recordedHashes, change.Path)
}
```

- [x] Guard the write on emptiness so a broken upstream invariant
      degrades to "missing entry" (visible on next Plan as
      `ChangeCreate`) rather than "poisoned empty" (silent delete /
      unknown):
      `go
    if prior := preview.ManagedState.ManagedFiles[change.Path]; prior != "" {
        recordedHashes[change.Path] = prior
    } else {
        delete(recordedHashes, change.Path)
    }
    `
- [ ] Leave the code alone and extend the block comment at lines
      336-341 to explicitly document the "baseline non-empty" invariant
      the Keep branch depends on (in addition to `state != nil`).

## `preview.ManagedState == nil` guard is unreachable — decide silent-drop vs. loud invariant

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "Add an interface only when there is a real boundary, variation, or test need" (restraint / dead defensive paths).
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Return Actionable Errors" (silent fallbacks hide invariant breaks).
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time" (the fallback is untested).

Per `classifyDesired` (`internal/sync/sync.go:254`, `return ChangeCreate
when state == nil`), a `ChangeDrift` row cannot exist with `state == nil`.
The `else` branch at `internal/sync/sync.go:344-346` is therefore dead
code today. It silently `delete`s the pre-filled rendered hash, which is
the exact "silent state loss" failure mode ADR 0015 exists to prevent —
and there is no test covering it.

```go
// internal/sync/sync.go:342-346
if preview.ManagedState != nil {
    recordedHashes[change.Path] = preview.ManagedState.ManagedFiles[change.Path]
} else {
    delete(recordedHashes, change.Path) // unreachable per classifyDesired
}
```

- [x] Return a typed `errs.DomainError` from the nil branch (e.g. a new
      `PreviewInvariantError{Kind: "ChangeDrift", Reason: "ManagedState nil"}`)
      so a future invariant break is surfaced loudly, not silently
      dropped.
- [ ] Drop the `else` entirely and let a nil `ManagedState` panic. The
      invariant is proven at `classifyDesired` and a panic surfaces
      immediately in tests if a future change breaks it.
- [ ] Leave the guard as-is but add a targeted unit test that pins the
      current behaviour — construct a synthetic `Preview{Changes:
    [{Kind: ChangeDrift}], ManagedState: nil}` and assert the state
      entry is absent after Apply. Documents the fallback contract
      even if it stays defensive.

## TUI `onApply` emits drift resolutions asymmetrically vs. unknowns

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — "Stable policy should not import volatile details. Instead, the volatile detail should call stable policy."
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code."

`internal/tui/shell/plan_project.go:831-843` now emits a
`DriftResolution` **only** for `DriftOverwrite` rows, but still emits an
explicit `UnknownKeep` for every `ChangeUnknown` row. The changelog at
`docs/changelog/2026-07-21_0033-apply-clears-pending-drift.md:108-111`
acknowledges that materializing `DriftKeep` "would still be harmless
with the fixed engine, but the code now matches the ADR 0015 contract
exactly." So the asymmetry is stylistic, not correctness-required.

That leaves the TUI as the enforcer of a domain contract ("absence ==
`DriftKeep` == preserve baseline") that lives in `sync.DriftKeep`'s doc
comment. A second caller (a scripted driver, a future UI) that emits
`DriftKeep` explicitly is _safe_ — but a reader of `onApply` has to
know the asymmetry is deliberate.

```go
// internal/tui/shell/plan_project.go:831-843
case app.ChangeDrift:
    if s.driftResolutions[ch.Path] == app.DriftOverwrite {
        drift = append(drift, app.DriftResolution{Path: ch.Path, Decision: app.DriftOverwrite})
    }
case app.ChangeUnknown:
    decision := app.UnknownKeep
    if s.unknownResolutions[ch.Path] == app.UnknownDelete {
        decision = app.UnknownDelete
    }
    unknown = append(unknown, app.UnknownResolution{Path: ch.Path, Decision: decision})
```

- [ ] Keep as-is: the domain (`sync.Apply`) is tolerant of either
      form, the block comment already documents the choice, and the
      test `TestPlanProjectScreen_OnApplyEmptyMapOmitsDriftKeepAndKeepsUnknown`
      pins the current emission contract. The finding stays a
      readability trade-off, not a defect.
- [ ] Make the TUI emit `DriftKeep` explicitly (symmetric with
      `UnknownKeep`) and rely on the sync engine's idempotent Keep
      branch. Delete the ADR-0015 comment in `onApply`; the domain
      enforces the rule. (Same behaviour, fewer moving parts to
      remember.)
- [x] Move the "emit only Overwrite" rule into a small helper in
      `internal/app` (e.g. `app.DriftResolutionsFromMap`) that both
      the TUI and any future caller share, so the encoding of the
      domain contract lives one hop from the domain rather than in
      `internal/tui/shell/plan_project.go`.

## Rendered `af.drift-cleanup` skill glossaries still teach the old `DriftKeep` meaning

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language" ("let UI labels, docs, and code drift into different meanings").

The domain glossary at `docs/glossary.md:232-241` was updated to the
leave-alone semantics. Two rendered twins under managed surfaces were
not:

- `.claude/skills/af.drift-cleanup/docs/glossary.md:143-147`
- `.codex/skills/af.drift-cleanup/docs/glossary.md:143-147`

Both still read:

```md
Drift defaults to _keep_ during apply; the user must explicitly
resolve a drift entry to `DriftOverwrite` to let apply replace the
local edits. Choosing `DriftKeep` adopts the on-disk content as the
new managed baseline so future plans do not flag the same path as
drift again. See ADR 0010.
```

An agent (or human) reading these copies from a consuming repo learns
the pre-fix contract and could reintroduce bug 0033's semantics
elsewhere.

- [x] Update both skill-glossary copies to match `docs/glossary.md:232-241`
      and add the ADR 0015 pointer alongside ADR 0010.
- [ ] Leave the skill copies alone — treat this task as engine-only;
      file a separate cleanup task for the drift-cleanup skill twin.

## `docs/glossary.md` Drift entry cites `ADR 0010` alone

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language" (glossary is canonical; keep ADR pointers in one voice).

`docs/glossary.md:241` still ends the Drift entry with `See ADR 0010.`
even though the entry describes the ADR 0015 semantics and cites
`bug 0033` earlier in the paragraph. The code comment
(`internal/sync/sync.go:113`) and the changelog both point at ADR 0015
as the authority.

```md
// docs/glossary.md:241
the profile is a separate, future operation (_Adopt_), not Keep. See ADR 0010.
```

- [ ] Change the trailing pointer to `See ADR 0010 and ADR 0015.` so
      the citation matches the semantics the paragraph describes.
- [x] Replace with `See ADR 0015.` alone — 0015 supersedes the 0010
      DriftKeep clause and 0015 already points back at 0010 for the
      superseded portion.

## Pure-ignore-set regression test does not actually exercise the ignore-set mutation

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time."

`internal/sync/sync_test.go:530-564`
(`TestApply_PureIgnoreSetChange_LeavesDriftBaselineUntouched`) is named
after the bug-0033 scenario ("un-ignore a persisted folder"), but the
Apply call passes `Resolutions{}` — drift nil, ignored nil. The
regression is still caught because the root cause was the pre-filled
rendered hash in the Keep branch, not the ignore slice. But a future
special case ("skip the drift loop if `Resolutions` is empty") would
keep this test green while corrupting state on a real un-ignore Apply.

```go
// internal/sync/sync_test.go:544
// Un-ignore the persisted folder by sending an empty ignored set; drift
// row gets no resolution, so it must fall through to the preserve default.
if err := Apply(preview, Resolutions{}); err != nil { // sends nil, not "explicit empty"
```

- [x] Rename to `TestApply_NoResolutions_LeavesDriftBaselineUntouched`
      so the name matches what is actually asserted.
- [ ] Keep the name and add a companion test that Applies with
      `Resolutions{IgnoredPaths: []string{}}` (or a differing non-empty
      set) to prove the ignore-set mutation path also preserves the
      drift baseline, matching the bug-0033 title.

## Shell test asserts identity fields unrelated to the drift-emission promise

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time."

`internal/tui/shell/plan_project_test.go:521-523` — inside
`TestPlanProjectScreen_OnApplyEmptyMapOmitsDriftKeepAndKeepsUnknown` —
asserts `in.ProfileRef == "alpha"` and `in.ProjectID == "proj-1"`.
Those checks are unrelated to the drift/unknown emission contract this
test's name promises and are already implicitly proven by the other
`syncInputs` tests in the file. Mixing them dilutes the failure signal.

```go
// internal/tui/shell/plan_project_test.go:521-523
if in.ProfileRef != "alpha" || in.ProjectID != "proj-1" {
    t.Errorf("input ids = (%q, %q), want (alpha, proj-1)", in.ProfileRef, in.ProjectID)
}
```

- [x] Delete the id-assertion block from this test — the id plumbing
      is already covered by `TestPlanProjectScreen_OnApplyWithSelectionsBuildsCorrectSlices`
      and friends.
- [ ] Leave as-is (defensive redundancy is cheap; the test still fails
      the right way when the emission contract regresses).

## `TestApply_DriftKeep_LeavesOnDiskAlone` is largely subsumed

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test One Behavior At A Time" / "Keep Test Code Simple."

`internal/sync/sync_test.go:258-281` asserts only "on-disk file body
unchanged after `DriftKeep`." The newer
`TestApply_DriftKeep_PreservesPriorBaseline` (`sync_test.go:451-480`)
asserts baseline preserved AND next-Plan classification, which is only
reachable if the on-disk body is also unchanged. The older test no
longer describes a promise the codebase makes above and beyond the
newer one, and the fixture setup is nearly identical.

- [x] Delete `TestApply_DriftKeep_LeavesOnDiskAlone` — the fuller
      contract test carries the "Keep leaves everything alone"
      promise.
- [ ] Keep both and add a one-line comment explaining why: e.g.
      "cheap targeted regression guard for on-disk state" vs. "full
      baseline + reclassification contract."
