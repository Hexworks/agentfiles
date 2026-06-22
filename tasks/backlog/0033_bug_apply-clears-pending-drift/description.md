---
id: 0033
type: bug
status: backlog
topics: sync, tui, charm, go
---

# Apply silently clears pending drift (drift → update) on an unrelated ignore-set change

## Symptom

On the **Plan Project** screen, a set of files were correctly classified as
`drift`. The user then un-ignored a persisted folder (`ask-matt`) and pressed
**Apply** — a change that should only touch `ignored_paths`. After the Apply:

- those files are now classified as **`update`** instead of `drift`, and
- their baseline hashes in the target repo's `.agentfiles/state.json` were
  rewritten to the **on-disk (drifted)** hashes.

The profile assets were **not** touched (source of truth intact). Only the
state snapshot was corrupted.

## Root cause

An Apply unconditionally bakes a default **DriftKeep** decision into every
pending drift entry, which adopts the on-disk hash as the new baseline:

1. `onApply` (`internal/tui/shell/plan_project.go`, ~L846–861) iterates
   `preview.Changes` and emits an **explicit** `app.DriftKeep` resolution for
   **every** `ChangeDrift` row — including rows the user never touched. The
   comment there states this materialization of the default is intentional.
2. `sync.Apply` `ChangeDrift` + `DriftKeep` branch
   (`internal/sync/sync.go`, ~L323–340) **adopts the on-disk hash** into
   `recordedHashes`, overwriting the prior baseline from `ManagedState`.
3. Next `Plan` (`classifyDesired`, `internal/sync/sync.go` ~L264): on-disk hash
   now equals the recorded baseline, so the drift predicate
   `state.ManagedFiles[path] != currentHash` is false → the file falls through
   to `ChangeUpdate`.

Net effect: any Apply (even one whose only real change is the ignore-set)
silently "resolves" all pending drift by adopting the local edits as baseline,
flipping `drift` → `update` and discarding the drift warning the user had not
acted on.

This is currently the **documented and tested** contract — see the `DriftKeep`
doc comment (`internal/sync/sync.go` ~L105–113) and
`TestApply_DriftKeep_AdoptsCurrentAsBaseline` /
`TestApply_DefaultDrift_LeavesAlone` (`internal/sync/sync_test.go`). So the fix
is a deliberate semantics change, not a one-line patch.

## Proposed fix (recommended: Option A)

Split "user explicitly chose Keep" from "user made no decision":

- **No decision** (path absent from `Resolutions.Drift`): preserve the **prior**
  baseline (`preview.ManagedState.ManagedFiles[path]`) so the file is still
  classified as `drift` on the next Plan. Do **not** adopt the on-disk hash.
- **Explicit `DriftKeep`**: keep today's adopt-on-disk-hash behaviour (clears
  the drift flag, becomes `update` if the rendered body diverges).
- **`DriftOverwrite`**: unchanged.

`indexDriftResolutions` already lets `Apply` tell "present in the map" from
"absent" (zero value), so the two cases are distinguishable.

Then fix `onApply` so it only sends a resolution for drift rows the user
actually acted on (i.e. `DriftOverwrite` selections), letting untouched drift
fall through to the new "no decision = preserve" default.

### Option B (simpler, more aggressive)

Drop adopt-on-keep entirely: `DriftKeep` always preserves the prior baseline,
so drift persists until the file is overwritten or profile/file converge. This
removes the "acknowledge these edits as the new baseline" capability — which has
no dedicated UI affordance today (the row `[Keep]` button just clears an
`[Overwrite]` selection). Pick this only if that capability is unwanted.

## Files

- `internal/sync/sync.go` — `Apply` `ChangeDrift` branch; `DriftKeep` /
  `DriftResolution` / `DriftDecision` doc comments.
- `internal/tui/shell/plan_project.go` — `onApply` drift-resolution emission.
- `internal/sync/sync_test.go` — update `TestApply_DefaultDrift_LeavesAlone` to
  also assert the baseline is preserved and the path stays `ChangeDrift` on the
  next Plan; keep `TestApply_DriftKeep_AdoptsCurrentAsBaseline` for the
  **explicit** Keep path.
- `docs/adr/0010-sync-resolutions-and-first-apply.md` — record the
  no-decision-vs-explicit-Keep distinction.

## Recovery (already-corrupted state)

The agentfiles repo's own `.agentfiles/state.json` is already corrupted
(uncommitted): the `.claude/skills/af.create-task/*` and
`.claude/skills/af.task.review/SKILL.md` baselines were rewritten to their
drifted on-disk hashes while `ask-matt` was dropped from `ignored_paths`.
Restore the prior baselines (the committed `HEAD` values) for those four keys
while keeping `ask-matt` un-ignored, so the files report as `drift` again.

## Tests

- After an Apply with no explicit decision for a drifting path, the next Plan
  still classifies it `ChangeDrift` and `state.json` keeps the prior baseline.
- A pure ignore-set Apply (un-ignore a persisted folder, zero file changes)
  leaves every unrelated drift baseline untouched.
- Explicit `DriftKeep` still adopts the on-disk hash (regression guard).
- `DriftOverwrite` still writes the rendered body and records the rendered hash.

## Verification

```
make build && make test && make lint
./bin/af   # Plan Project on a project with both pending drift and a persisted
           # ignored folder → un-ignore the folder → Apply →
           # reopen Plan → the drifting files are STILL "drift", not "update"
```
