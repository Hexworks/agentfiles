---
id: 0035
type: feature
status: in-progress
topics: sync_and_safety, domain_model, tui
depends_on: 0033
---

# Adopt: promote a local drift edit into the profile

A third drift resolution alongside `DriftKeep` (leave alone) and
`DriftOverwrite` (replace local with rendered): **Adopt** takes the user's
**local edit** and makes it the *canonical* representation by writing it back
into the **profile** (source of truth). Adopt is the mirror of Update:

- **Update** — profile changed, local rendered file is stale → rewrite local.
- **Adopt** — local file changed, profile is stale → rewrite profile from
  local.

A companion **UnknownAdopt** covers the related case where an untracked file
sits *inside* a known asset's rendered folder (e.g. a new `example-3.md`
under `.claude/skills/foo/`). Untracked *folders* remain Register territory
and are out of scope here.

## Use case

1. User edits a managed file locally in a project to try something out.
2. User decides the change is good and wants it to become canonical.
3. User picks **Adopt** on that drift row (or unknown row when the file
   lives inside a known asset dir).
4. On apply, the local content is copied into the profile asset it came
   from. When git integration is enabled, a commit is recorded in the
   profile repo.
5. On the next plan the adopted path shows neither drift nor update.
   Sibling agent projections (e.g. the `codex` mirror of a `claude` skill)
   surface as **`update`** rows — the user applies them separately in a
   follow-up plan/apply cycle.

## Key constraint — the one sanctioned repo → profile flow

Normal flow is profile → repo; render never reads the repo as input
(CLAUDE.md invariant #6, ADR 0001). Adopt is the single deliberate reverse
flow: repo → profile. To keep the exception well-scoped:

- `sync` stays repo-only. `sync.Apply` classifies Adopt resolutions and
  returns the reverse-write list; the actual profile write lives in
  `app.Service` alongside the git commit.
- Reverse-mapping (which profile asset file does a rendered path map back
  to?) is persisted per-entry in `.agentfiles/state.json` (schema v3:
  `{asset_id, source_rel}`). Legacy v2 entries render Adopt disabled until
  the file is next re-applied.
- Adopt writes the profile asset file only; sibling agent projections
  surface as ordinary `update` rows on the next plan. Adopt is not a
  multi-agent broadcast.
- A new ADR records Adopt as the single sanctioned exception to
  profile-is-authoritative; CLAUDE.md invariant #6 is amended in the same
  change.

## Notes

- Builds on 0033, which fixes `DriftKeep` to mean "leave alone, preserve
  baseline" and removes the accidental adopt-on-keep behavior. Adopt is the
  intentional successor capability with its own enum value (`DriftAdopt`)
  and a matching `UnknownAdopt`.
- TUI: the drift row action cycles Keep → Overwrite → Adopt → Keep. The
  unknown row action exposes Adopt only when the classifier populated
  `OwningAssetID` on the change.
- Reverse map lives in `sync.Preview.FileChange.OwningAssetID` (unknown
  kind) and in the new state.json fields (drift kind).

## Acceptance Criteria

- [ ] `internal/sync` exports `DriftAdopt DriftDecision = "adopt"` and
  `UnknownAdopt UnknownDecision = "adopt"`; `internal/appapi` mirrors both.
- [ ] `sync.Preview.FileChange` (unknown kind) carries `OwningAssetID string`
  populated when the unknown path sits under a known asset projection dir;
  verified by `TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset`.
- [ ] `.agentfiles/state.json` schema bumps to v3; each entry adds
  `asset_id` and `source_rel`; loader accepts v2 entries as legacy (Adopt
  disabled for them); verified by
  `TestState_LoadV2LegacyEntries_LeavesAdoptDisabled` and
  `TestState_WriteV3_IncludesAssetIDAndSourceRel`.
- [ ] `sync.Apply` with `Resolutions.Drift = [{Path:P, Decision:DriftAdopt}]`
  returns P in the adopt reverse-write list and marks P non-drift on next
  plan; verified by `TestApply_DriftAdopt_WritesProfileAndClearsDrift`.
- [ ] `sync.Apply` with `Resolutions.Unknown = [{Path:P, Decision:UnknownAdopt}]`
  fails when `OwningAssetID` is empty and succeeds otherwise; verified by
  `TestApply_UnknownAdopt_RejectsOrphanFile` and
  `TestApply_UnknownAdopt_WritesProfileAssetFile`.
- [ ] `app.Service.Apply` copies each repo file in the adopt list to
  `<profile>/assets/<asset_id>/<source_rel>` and invokes
  `GitCommitter.Commit` on the profile dir when `settings.git.enabled ==
  true`; verified by `TestService_Apply_AdoptCommitsProfileWhenGitEnabled`
  and `TestService_Apply_AdoptSkipsCommitWhenGitDisabled`.
- [ ] TUI drift row action toggles Keep → Overwrite → Adopt → Keep;
  verified by `TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt`.
- [ ] TUI unknown row action includes Adopt option only when
  `OwningAssetID != ""`; verified by
  `TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset`.
- [ ] `docs/architecture/`, `docs/adr/` (new ADR: Adopt as sanctioned
  reverse flow), `docs/glossary.md`, and the CLAUDE.md source-of-truth
  invariant updated to describe Adopt as the single sanctioned repo →
  profile flow; verified by manual review plus
  `grep -R "Adopt" docs/ CLAUDE.md` producing hits in each.
- [ ] Smoke: `./bin/af → project screen → plan → local edit
  .claude/skills/foo/SKILL.md → replan → row toggled to Adopt → apply →
  replan shows no drift/update on that path`.
- [ ] Smoke: same flow with `settings.git.enabled = true` → `git log` in
  the profile repo shows the Adopt commit; sibling `.codex` skill file
  shows as `update` on the next plan.

## Out of scope

- Adopting a fully-unknown file that is not inside a known asset dir —
  that remains the Register flow (`Service.CreateAssetFromFolder`).
- Whole-asset dir sync in a single Adopt (per-file only; multi-file assets
  need multiple Adopt toggles).
- Auto-propagating an Adopt to sibling agent projections in the same apply
  (siblings surface as ordinary `update` rows on the next plan).
- Adopting deletes (local delete → profile delete) — separate flow.
- Backfilling `asset_id`/`source_rel` on legacy v2 state entries (Adopt
  stays disabled until the next re-apply repopulates them).
- Migration tooling for existing state.json files beyond the loader's
  v2-legacy tolerance.

## Verification

- `make build && make test && make lint` (baseline gate).
- `go test ./internal/sync -run TestApply_DriftAdopt_WritesProfileAndClearsDrift`.
- `go test ./internal/sync -run TestApply_UnknownAdopt_WritesProfileAssetFile`.
- `go test ./internal/sync -run TestApply_UnknownAdopt_RejectsOrphanFile`.
- `go test ./internal/sync -run TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset`.
- `go test ./internal/sync -run TestState_LoadV2LegacyEntries_LeavesAdoptDisabled`.
- `go test ./internal/sync -run TestState_WriteV3_IncludesAssetIDAndSourceRel`.
- `go test ./internal/app -run TestService_Apply_AdoptCommitsProfileWhenGitEnabled`.
- `go test ./internal/app -run TestService_Apply_AdoptSkipsCommitWhenGitDisabled`.
- `go test ./internal/tui/shell -run TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt`.
- `go test ./internal/tui/shell -run TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset`.
- Smoke: `./bin/af → project screen → plan → edit
  .claude/skills/foo/SKILL.md locally → replan → toggle row to Adopt →
  apply → replan → no drift/update on that path`.
- Smoke: same flow with `settings.git.enabled = true` → `git log` in the
  profile repo shows the Adopt commit; the sibling `.codex` file appears
  as `update` on the next plan.
