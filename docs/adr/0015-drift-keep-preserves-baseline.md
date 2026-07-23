# Drift Keep Preserves The Baseline; Adopt Is The Explicit Promote

## Status

accepted

Supersedes the `DriftKeep` clause of ADR 0010 (the rest of 0010 stands).

## Context

ADR 0010 defined the drift default as `DriftKeep`, and the engine implemented
Keep as "leave the file on disk, but **adopt the on-disk hash** as the new
managed baseline." The intent was that acknowledging a drift would stop it from
being reported again.

That coupling produced bug 0033. `onApply` materialized an explicit `DriftKeep`
for **every** drift row — including rows the user never touched — so any Apply
(even one whose only real change was the ignore-set) silently rebaselined all
pending drift to its on-disk content. On the next `Plan` the file's on-disk hash
equalled the recorded baseline, so it was no longer `drift`; because the rendered
body still differed it surfaced as `update`. The user's un-acknowledged drift
warning was discarded, and the state snapshot was rewritten to the local edits.

Classification is a pure function of three hashes for a path — `B` (baseline in
`state.json`), `D` (desired render output), `F` (on-disk file):

- `F==D` → Clean
- `F!=D` and `F==B` → Update
- `F!=D` and `F!=B` → Drift

The bug was Apply mutating `B := F` on Keep, which turns the intended
`Drift → Drift` self-loop into `Drift → Update`.

The deeper problem was conceptual: one word, "Keep", carried two distinct
meanings — *leave the file alone* and *accept the local edits as canonical*. The
second meaning had no UI affordance (the row toggle only switches Overwrite ↔
Keep, where Keep just clears an Overwrite selection), so it existed only as an
accidental default.

## Decision

`DriftKeep` means **leave alone**: the file is untouched **and the prior managed
baseline is preserved**. A kept drift stays classified as `drift` on every
subsequent plan until the user overwrites it, or the profile/file converge.
"No decision" (path absent from `Resolutions.Drift`) is identical to `DriftKeep`.
Apply never adopts the on-disk hash as the new baseline.

`DriftOverwrite` is unchanged: it writes the rendered body and records the
rendered hash.

Accepting local edits as canonical is now shipped as **Adopt** — see
[ADR 0020](./0020-adopt-as-sanctioned-reverse-flow.md). Adopt is not a
baseline rewrite: it promotes the local edit *into the profile* (the
source of truth) and lets sibling agent projections catch up as ordinary
`update` rows on the next plan. Because it reverses the profile → repo
flow, it gets its own `DriftDecision` value (`DriftAdopt`) and its own
UI button.

We considered dropping the `DriftKeep` enum entirely (since Keep and "no
decision" are now identical) but kept it as the explicit, named default — it
documents intent and stays symmetric with `UnknownKeep`.

## Consequences

- `sync.Apply`'s `ChangeDrift` Keep branch records the prior baseline
  (`preview.ManagedState.ManagedFiles[path]`), not the on-disk hash.
- `onApply` emits a drift resolution only for `DriftOverwrite` rows; untouched
  rows fall through to the preserve-baseline default.
- The "acknowledge these edits as the new baseline" capability is now
  provided deliberately by Adopt (ADR 0020).
- Tests change accordingly: `TestApply_DriftKeep_AdoptsCurrentAsBaseline` is
  replaced by a preserves-prior-baseline assertion; a pure ignore-set Apply must
  leave unrelated drift baselines untouched.
- The glossary Drift entry is updated to the leave-alone meaning.
