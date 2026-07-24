# Adopt Is The Single Sanctioned Repo → Profile Flow

## Status

accepted

Follows [ADR 0015](./0015-drift-keep-preserves-baseline.md) (Keep
preserves the baseline) and refines invariant #6 in `CLAUDE.md`
("Profile is authoritative; render never reads the repo as input").

> **Superseded in part by [ADR 0021](./0021-render-reverse-strategy-table.md).**
> The reverse-mapping mechanics this ADR describes as living in
> `internal/sync` (`assetProjectionDirs` / `owningAssetSourceRelFor`) now
> live in `render` behind `ProjectPlan.ReverseLookup`, owned by the same
> `(agent, type)` strategy that renders forward. The Adopt *policy* below
> is unchanged.

## Context

`agentfiles` treats a profile folder as the source of truth. Every
managed surface file in a project repo is an **output** produced by the
render pipeline from the profile. Invariant #6 (`CLAUDE.md`) forbids
render from reading repo state as input; the sync engine mediates all
writes and never sends bytes the other way.

Users have a legitimate need this invariant did not cover: quickly
promote a local repo edit into the profile so it becomes the canonical
version. Today they must:

1. Copy the file back into the profile asset folder by hand.
2. Re-run `plan`/`apply` for the project so the copy is picked up.
3. Manually replicate the edit for every sibling agent projection
   (claude → codex → cursor).

Two capabilities the TUI already ships do NOT cover this:

- `DriftOverwrite` throws the local edit away and rewrites the repo from
  the profile.
- `Register-as-Asset` (`CreateAssetFromFolder`) turns a folder full of
  untracked files into a *new* asset. It is the wrong tool for a
  one-file edit inside an already-managed asset.

## Decision

Introduce **Adopt**: the single sanctioned repo → profile flow.

- `sync.DriftDecision` gains a third value, `DriftAdopt`. On a drift
  row, the TUI cycles Keep → Overwrite → Adopt → Keep. Adopt leaves
  the on-disk repo body as-is and returns an `AdoptRequest{Path,
  AssetID, SourceRel}` in `sync.ApplyResult.AdoptRequests`.
- `sync.UnknownDecision` gains `UnknownAdopt`, offered by the TUI only
  when the change row carries a non-empty `OwningAssetID` (populated at
  Plan time for unknown files that sit inside a known asset's
  projection dir).
- Reverse-mapping is captured at Apply time. `state.json` bumps to
  schema v3 whose `managed_files` entries carry `{hash, asset_id,
  source_rel}`. Legacy v2 entries (bare hash) decode cleanly for
  backwards compatibility but Adopt is disabled for them until the
  next re-apply repopulates the provenance keys.
- `app.Service.Apply` executes the reverse-write: read the repo file,
  write the body into `<profile>/assets/<type>/<asset_id>/<source_rel>`
  through `asset.WriteFile` (same containment rail as `AddFile` /
  `RemoveFile`), then, when git integration is enabled, record a
  scoped commit on the profile repo (`chore(agentfiles): adopt N
  file(s) into profile`). The primary target-repo commit still runs
  first; the profile-repo commit rides on a second discriminated
  `CommitOutcome` returned from `Service.Apply`.

Adopt is deliberately narrow:

- **Per-file, single-agent.** Sibling projections (e.g. the `.codex`
  mirror of an adopted `.claude` skill file) surface as ordinary
  `update` rows on the next plan and apply. Adopt is not a
  multi-agent broadcast.
- **`sync` stays repo-only.** Classification lives in `sync.Apply`
  but the profile-side write and its git commit live in
  `app.Service.Apply` so `sync` never touches the profile folder.
- **Not an escape hatch.** Adopt is the *only* sanctioned repo →
  profile write path. Render still refuses to read the repo as input.

`CLAUDE.md` invariant #6 is amended in the same change to acknowledge
Adopt as the sole exception.

## Consequences

- `state.json` schema v3 is a breaking on-disk change with a
  backwards-compatible loader. Existing v2 files continue to load;
  their entries are Adopt-disabled until one clean apply re-writes
  them under v3.
- Two typed errors surface Adopt-specific failures: `sync.AdoptUnavailableError`
  (v2 legacy entry, orphan unknown, missing reverse-mapping) and
  `app.AdoptReadError` (repo-side file unreadable). Both carry the
  offending path and follow the domain-error contract.
- The `Apply` return signature grows to include a second
  `CommitOutcome` for the profile-repo commit. Callers pattern-match
  on the concrete type (`Committed` / `Skipped` / `Failed`) the same
  way they already do for the sync commit. Downstream call sites
  (actions layer, TUI, tests) update together.
- Two commits happen on a git-enabled apply that adopts anything: one
  on the target repo (`chore(agentfiles): sync project X (N files)`),
  one on the profile repo (`chore(agentfiles): adopt N file(s) into
  profile`). The TUI merges both outcomes into one info toast plus any
  warn toasts their `Failed` variants demand.
- Documentation follows: glossary gains an **Adopt** entry, arc42
  building-block / runtime / concepts chapters describe the flow, the
  drift-decision chapter switches to a three-way cycle.
- Schema v3 rows must carry `asset_id` and `source_rel` together or
  not at all. A half-populated entry (only one of the two set) is a
  wiring bug and `loadState` rejects the whole file with
  `StateCorruptError` instead of silently disabling Adopt. Legacy v2
  rows (both fields absent) remain valid input and load Adopt-disabled
  until re-applied.
