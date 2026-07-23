# Plan: Adopt — promote local edits into the profile

Task: [description.md](./description.md) · Feature id `0035` · Depends on `0033` ([done](../../done/0033_bug_apply-clears-pending-drift/description.md)).

Related docs the plan will update: [`docs/adr/0015-drift-keep-preserves-baseline.md`](../../../docs/adr/0015-drift-keep-preserves-baseline.md) (Adopt promoted from "future" to "shipped"), new ADR `0020-adopt-as-sanctioned-reverse-flow.md`, `docs/glossary.md`, `docs/architecture/05-building-block-view.md`, `docs/architecture/06-runtime-view.md`, `docs/architecture/08-concepts.md`, `CLAUDE.md` (invariant #6 amendment).

---

## 1. Design summary

Adopt introduces one **repo → profile** write path — the mirror image of Update — under three tight rails:

1. **Reverse-mapping is captured at Apply time**, not inferred later. Every rendered file carries `AssetID + SourceRel`. Apply persists both alongside the hash in `.agentfiles/state.json` (schema v3). A drift row is eligible for Adopt only when its baseline entry carries both fields; v2 entries are legacy and the TUI grays out the Adopt option until the file has been re-applied under v3.
2. **`sync` stays repo-only.** `sync.Apply` classifies `DriftAdopt` / `UnknownAdopt` rows and returns them as an `AdoptRequests` list (asset id + source_rel + repo-relative source path). The actual write into `<asset.Dir>/<source_rel>` and the profile-repo commit live in `app.Service.Apply` alongside the existing target-repo commit.
3. **Adopt is per-file, single-agent.** It writes exactly one asset file. Sibling agent projections (e.g. the `.codex/skills/foo/SKILL.md` mirror of an adopted `.claude/skills/foo/SKILL.md`) surface as ordinary `update` rows on the *next* Plan. Nothing is broadcast in the same Apply.

Adopt is the **single sanctioned exception** to CLAUDE.md invariant #6 (profile is authoritative; render never reads the repo as input). Render itself is untouched — Adopt is a separate write path that happens to feed the profile.

### 1.1 UI cycle

Drift row action cycles `Keep → Overwrite → Adopt → Keep`. Unknown row action cycles `Keep → Delete → (Adopt) → Keep` where the Adopt step only appears when `FileChange.OwningAssetID != ""`. Button labels continue the existing "show the next state" convention (`internal/tui/shell/plan_project.go:630-642`).

### 1.2 Data flow (one adopted path)

```
Plan  ─┐
       │  RenderedFile{Path, Body, AssetID, SourceRel}
       │  → recordedHashes[Path] = hash
       │  → recordedEntries[Path] = {Hash, AssetID, SourceRel}   (v3 shape)
       │
Apply ─┤  ChangeDrift row + DriftAdopt resolution
       │    ← baseline entry AssetID/SourceRel from state
       │    ← re-read on-disk file body
       │  ChangeUnknown row + UnknownAdopt resolution
       │    ← OwningAssetID from FileChange (populated at Plan time)
       │    ← SourceRel derived from asset projection reverse map
       │
       │  sync returns AdoptRequests + writes state.json (v3)
       │
Service.Apply
       │  for each adopt request:
       │    read <projectPath>/<Path>
       │    write <asset.Dir>/<SourceRel>
       │    track for profile-repo commit pathspec
       │  run triggerSyncProject commit (target repo, unchanged)
       │  run triggerAdoptIntoProfile commit (profile repo, new)
       │
Next Plan
       │  adopted path: F==D→Clean (no row)
       │  sibling projections: F!=D→Update
       │  baseline restored on the primary agent's projection
```

---

## 2. Step-by-step execution

Steps ordered so tests can be added incrementally and each step is independently runnable through `make build && make test`.

### Step 1 — Extend `render.RenderedFile` with source provenance

- Add `SourceRel string` to `render.RenderedFile` (`internal/render/render.go:24-33`). It stores the asset-relative path that produced this rendered file.
- Populate it in every writer branch:
  - `addSkillOutputs`: `SourceRel = rel` (the value from `asset.RelativeFiles`).
  - `TypeAgentsDoc`: `SourceRel = config.AgentsDocStarterFileName`.
  - `TypeSettings`: `SourceRel = mapping.Source`.
  - Generic file projection: `SourceRel = projection.Source`.
  - Generic dir projection (`walkProjection`): `SourceRel = filepath.ToSlash(filepath.Join(sourceRel, rel))`.
- `AssetID` already exists on `RenderedFile`; keep as-is.
- No behavioural change for sync/plan — only the struct grew.
- Tests: extend existing `render_test.go` — each render branch's fixture asserts a non-empty `SourceRel` on emitted files; regression coverage for each of the five branches above.

### Step 2 — Bump `state.json` to schema v3 with `{hash, asset_id, source_rel}` entries

- In `internal/sync/sync.go` introduce a value type:
  ```go
  type ManagedFileEntry struct {
      Hash      string `json:"hash"`
      AssetID   string `json:"asset_id,omitempty"`
      SourceRel string `json:"source_rel,omitempty"`
  }
  ```
- Give `ManagedFileEntry` a `UnmarshalJSON` that accepts either a JSON string (v2 legacy: `"deadbeef…"` → `{Hash: "deadbeef…"}`) or the v3 object shape. Marshalling always writes the v3 object.
- Change `ManagedState.ManagedFiles` from `map[string]string` to `map[string]ManagedFileEntry`. Bump `GeneratorVersion` to `"2.0.0"` (the semver-breaking loader change is the schema break — the version field is the existing single-source signal; a separate `schema_version` int would just duplicate it).
- Update every reader in sync (`state.ManagedFiles[path] != ""` → `state.ManagedFiles[path].Hash != ""` etc.) and every writer (Apply populates `Hash + AssetID + SourceRel` from the rendered file map).
- Legacy tolerance: when the loader sees a bare-string entry it fills `Hash` only; `AssetID` and `SourceRel` stay empty. `Adopt` requests against such entries return a new `AdoptUnavailableError` (typed, severity=Error) so the failure surfaces if it ever slips past the TUI gate.
- `loadState` in `internal/sync/sync.go:530` validates unchanged for the hash and path-key rules; new: reject entries where `AssetID != "" && SourceRel == ""` (or vice versa) as `StateCorruptError` — a half-populated v3 entry is a wiring bug worth surfacing loudly.
- Tests:
  - `TestState_LoadV2LegacyEntries_LeavesAdoptDisabled` — write a `map[string]string`-shaped state.json, load, assert entries have `Hash` populated and `AssetID`/`SourceRel` empty; a subsequent Apply of a `DriftAdopt` resolution against that path returns `AdoptUnavailableError`.
  - `TestState_WriteV3_IncludesAssetIDAndSourceRel` — real profile via `profile.Init` + real skill asset, run Plan+Apply, reload the state.json bytes, assert entries carry `{hash, asset_id, source_rel}` and the file is valid JSON in the v3 object form.

### Step 3 — Add `DriftAdopt` + `UnknownAdopt` enums (domain + boundary)

- `internal/sync/sync.go`: add `DriftAdopt DriftDecision = "adopt"` and `UnknownAdopt UnknownDecision = "adopt"`. Update the doc comments on `DriftDecision` and `UnknownDecision` to describe the three-way and three-way (skill-only) semantics.
- `internal/appapi/appapi.go`: mirror both constants (`DriftAdopt`, `UnknownAdopt`).
- Update `appapi.DriftResolutionsFromMap` (`internal/appapi/appapi.go:246-258`) to also emit rows whose decision is `DriftAdopt`; keep the "no-emit on Keep" rule.
- Tests: extend `sync_test.go` and existing appapi tests to cover the new enum round-trips (encode/decode, mirror equivalence).

### Step 4 — `sync.FileChange.OwningAssetID` for unknown rows

- Add `OwningAssetID string` to `sync.FileChange` (`internal/sync/sync.go:151-156`). Also mirror `appapi.FileChange` (`internal/appapi/appapi.go:62-68`) and populate the mirror in `previewFromSync` (`internal/app/service.go:285-303`).
- Populate at Plan time inside `detectDeletesAndUnknowns` (`internal/sync/sync.go:568`). Rule:
  1. Build a per-plan map `assetDirs: map[string]string` (dir → assetID) from `rendered.Files`, keyed by `path.Dir(file.Path)`, valued by `file.AssetID`. Multiple files under the same dir must share the same asset id — if not, the dir is ambiguous and is dropped (no owner, Adopt not offered).
  2. For each unknown path, walk parent directories from `path.Dir(unknown)` upward; the first hit in `assetDirs` supplies `OwningAssetID`. Stop when the parent is `.` or a `surfaces.AssetContainerRoots()` entry (do not cross the container boundary).
- Result: only unknowns *inside* a known asset's rendered folder receive an owner — matches the description's `example-3.md under .claude/skills/foo/` case.
- Tests:
  - `TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset` — real profile+project+skill asset foo with `SKILL.md`, drop a stray `example-3.md` next to it, run Plan, assert the unknown row's `OwningAssetID == "foo"`.
  - `TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForOrphan` — stray file under `.claude/skills/` (no sibling of a known asset), assert empty.
  - `TestPreview_ChangeUnknown_LeavesOwningAssetIDEmptyForAmbiguousDir` — synthesize a directory hosting rendered files from two distinct asset ids, assert empty (ambiguous → no offer).

### Step 5 — `sync.Apply` returns `AdoptRequests`; classifies Adopt rows

- Extend `sync.ApplyResult`:
  ```go
  type AdoptRequest struct {
      Path      string // repo-relative source file, forward-slash
      AssetID   string
      SourceRel string
  }
  type ApplyResult struct {
      Mutated       []string
      StatePath     string
      AdoptRequests []AdoptRequest
  }
  ```
- In the Apply loop:
  - `ChangeDrift` + `DriftAdopt`: look up baseline `AssetID`/`SourceRel` from `preview.ManagedState.ManagedFiles[change.Path]`; if either is empty → append `AdoptUnavailableError` and continue. Otherwise append to `AdoptRequests`, **leave file on disk untouched** (adopt is not a repo-side write), **and preserve the prior baseline** for that path in `recordedHashes` (same rule as Keep — the file stays classified as drift only until the next Plan reruns on the updated profile, at which point F==D and the row disappears).
  - `ChangeUnknown` + `UnknownAdopt`: require `change.OwningAssetID != ""` → else `AdoptUnavailableError`. Derive `SourceRel` via the reverse-mapping rule (see below). Append to `AdoptRequests`. Do not remove the unknown file. Do not add to `recordedHashes` (the next Plan will re-render and pick it up as a regular managed file).
- `SourceRel` derivation for unknowns: given the parent chain that led to `OwningAssetID`, the source_rel is the path relative to the asset container's per-asset root:
  - skill: `<skill_root>/<asset_id>/<rel>` → `source_rel = rel`.
  - cursor commands: single-file layout; no folder shape → skip (unknown-adopt never fires here because the asset's own file already exists as managed).
  - generic dir projection: `source_rel = filepath.ToSlash(filepath.Join(projection.Source, <rel-under-target-dir>))`.
- Implementation detail: compute the reverse mapping once at Plan time (a `sync.assetProjectionRoots` helper that emits `[]{root, assetID, projectionSource}` from `rendered.Files`), stash on the returned Preview via a new unexported field, and read from it in Apply. Keeps the mapping computed with fresh source-of-truth data and avoids re-walking assets in Apply.
- The pre-filled `recordedHashes` map (`internal/sync/sync.go:320`) now stores `ManagedFileEntry{Hash, AssetID, SourceRel}` per file; the drift-Keep and drift-Adopt branches preserve the prior baseline entry (whole `ManagedFileEntry`), not just the hash.
- Tests (all real-stack; each uses `t.TempDir()` and real `sync.Plan` for the setup):
  - `TestApply_DriftAdopt_WritesProfileAndClearsDrift` — set up a real project applied once (v3 state.json written); user edits `.claude/skills/foo/SKILL.md` locally; new Plan classifies it as drift; call Apply with `DriftAdopt`; **verify (a)** the returned `AdoptRequests` contains one entry with the right `AssetID`/`SourceRel`, **(b)** the on-disk repo file body is unchanged (sync doesn't write), **(c)** the recorded baseline is preserved (the drift-classified path stays classified as drift on a plan reload — this is Step 5's behaviour in isolation; only after Step 6 writes the profile and the next Plan re-renders does the row disappear).
  - `TestApply_UnknownAdopt_RejectsOrphanFile` — unknown row with empty `OwningAssetID` + `UnknownAdopt` → `AdoptUnavailableError`; no requests emitted; state.json still rewritten.
  - `TestApply_UnknownAdopt_WritesProfileAssetFile` — unknown row with populated owner + `UnknownAdopt` → `AdoptRequests` contains one entry; unknown file still on disk; state.json rewritten (without the unknown path).
  - `TestApply_DriftAdopt_LegacyV2Entry_ReturnsAdoptUnavailable` — bake a v2-shaped state.json, load, call Apply with `DriftAdopt` → `AdoptUnavailableError`.

### Step 6 — `app.Service.Apply` executes adopt writes + profile commit

- Extend `app.Service.Apply` (`internal/app/service.go:325`) to:
  1. Call `llmsync.Apply` as today.
  2. For each `AdoptRequest`:
     - Read repo file: `os.ReadFile(filepath.Join(proj.Path, filepath.FromSlash(req.Path)))`.
     - Resolve target asset from the loaded profile: `loaded.Profile.Assets[req.AssetID]`. Missing asset → `AdoptUnavailableError` (typed; accumulate, do not abort peer adopts).
     - Validate the source_rel via `asset.ResolveRelative(asset.Dir, req.SourceRel)` — reuses the existing containment safety rail from `internal/asset/files.go:23-48`. Rejection → typed error, accumulate.
     - Write the bytes via a new `asset.WriteFile(dir, rel, body []byte, mode os.FileMode)` helper alongside `AddFile` / `RemoveFile` (mirrors their signature; `os.MkdirAll` intermediate + `os.WriteFile`). Failure → typed error, accumulate.
     - Track absolute profile-relative pathspec for the commit.
  3. Run the existing `triggerSyncProject` commit against the target repo (unchanged).
  4. Run a new `triggerAdoptIntoProfile` commit against the **profile repo** (`loaded.Profile.Root`). Subject: `chore(agentfiles): adopt N file(s) into profile`. Pathspec: the tracked profile-repo pathspec entries. Zero-request Apply → no adopt commit attempted at all (the trigger is only invoked when `len(AdoptRequests) > 0`).
  5. Both commits' outcomes must reach the caller. Extend `Service.Apply`'s return to include the adopt commit outcome as a second `CommitOutcome`, OR (simpler) return the adopt outcome as a warning-severity notification when it Fails and merge with the existing sync outcome for the success toast. Chosen shape: **extend the return** — third return value `adoptOutcome appapi.CommitOutcome`. This mirrors how the discriminated `CommitOutcome` was justified in ADR 0019 (compiler-enforced coverage). Adjust `Actions.SyncProject`, `planProjectActions.SyncProject`, and both existing tests' signatures.
- Tests (parallel to existing `TestService_Apply_*`):
  - `TestService_Apply_AdoptCommitsProfileWhenGitEnabled` — settings.git.enabled=true, real fake committer capturing (dir, pathspec, msg); assert two commit calls: one on target repo dir with `triggerSyncProject` template, one on profile root with the adopt template. Real profile init + real skill asset + real drift toggle.
  - `TestService_Apply_AdoptSkipsCommitWhenGitDisabled` — same fixture, settings.git.enabled=false; assert exactly zero commit calls (both sync + adopt are governed by the same setting). Adopt request still writes the asset file on disk (git off doesn't disable adopt itself, only the commit).
  - `TestService_Apply_AdoptWritesAssetFileWithProvenance` — settings.git.enabled=false; assert `<asset.Dir>/<source_rel>` on disk equals the edited repo body byte-for-byte; assert nothing under `<projectPath>` was modified for the adopted path.
  - `TestService_Apply_AdoptMissingAssetSurfacesError` — Adopt request whose `AssetID` is not in the loaded profile → `AdoptUnavailableError` in the returned domain error; no commit attempted.

### Step 7 — TUI: three-way drift cycle + optional unknown-Adopt

- `internal/tui/shell/plan_project.go`:
  - Extend `driftToggleBtn` (`:630-635`) into a 3-way cycle: Keep → Overwrite → Adopt → Keep. Button label continues to show the *next* state.
  - Extend `unknownToggleBtn` (`:637-642`): when `FileChange.OwningAssetID != ""`, cycle Keep → Delete → Adopt → Keep. When empty, keep the existing 2-way toggle.
  - `onApply` (`:838-884`): emit `DriftAdopt` and `UnknownAdopt` resolutions alongside Overwrite. The `appapi.DriftResolutionsFromMap` change from Step 3 already handles drift.
  - Merge the new adopt commit outcome into the success toast: sync `Committed{sync-sha}` + adopt `Committed{profile-sha}` → `"Project synced (committed <sync-sha>; profile <profile-sha>)"`. A Failed adopt outcome emits a second `SeverityWarning` toast alongside the base success. Add a `commitOutcomeCmd`-style helper if needed (`internal/tui/shell/commits.go:1-` already hosts the pattern).
- Tests (existing `plan_project_test.go` style, using the shell stubs from `stubs_test.go`):
  - `TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt` — start at Keep, press once → Overwrite, twice → Adopt, thrice → Keep; assert the row's actionValue and the emitted resolution slice.
  - `TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset` — build two rows: one with `OwningAssetID="foo"`, one without. Assert the first cycles through Adopt (3-way), the second stays 2-way (Keep ↔ Delete).
  - `TestPlanProjectScreen_ApplyEmitsAdoptResolutions` — mark one drift row Adopt, one unknown row (owned) Adopt, apply; the fake SyncProject records the resolutions containing both Adopt entries.

### Step 8 — Documentation, ADR, glossary, CLAUDE.md, changelog

- New ADR: `docs/adr/0020-adopt-as-sanctioned-reverse-flow.md`. Sections: Status (accepted); Context (invariant #6 was strict; users want a way to promote a local edit without leaving the TUI; existing Overwrite/Register don't cover it); Decision (Adopt is one deliberate exception; the rails above); Consequences (per-file only, sibling projections surface on next plan, legacy v2 entries disable Adopt until re-applied).
- Amend ADR 0015 (`docs/adr/0015-drift-keep-preserves-baseline.md`): rewrite the "future feature" note about task 0035 to point at the new ADR 0020 as delivered.
- `docs/glossary.md`: add **Adopt** entry (definition, position in the drift-decision matrix, cross-link to ADR 0020) and update the **Drift** entry to reference all three decisions.
- `docs/architecture/05-building-block-view.md`: extend the `sync` and `app` package boxes to note the new adopt request return + profile-write responsibility. Update the `render.RenderedFile` field list.
- `docs/architecture/06-runtime-view.md`: add an Adopt sequence next to the existing sync sequence (drift toggle → Apply → sync classifies → app writes → two commits).
- `docs/architecture/08-concepts.md`: extend the drift-decision section to describe the three-way cycle and Adopt.
- `CLAUDE.md`: amend invariant #6 with an explicit Adopt carve-out (one sentence pointing at ADR 0020).
- `docs/changelog/2026-07-23_0035-adopt-drift-into-profile.md`: standard shape (Motivation, What changed, How to use, Risks / follow-ups). Uses today's date per CLAUDE-side note that changelog files are dated.
- No new file in `docs/guidelines/` — Adopt reuses the sync_and_safety, domain_model, and tui guidelines already in play.
- `grep -R "Adopt" docs/ CLAUDE.md` must hit every intended location; the acceptance criterion enumerates this explicitly.

### Step 9 — Baseline gate + smoke

- `make build && make test && make lint` all green.
- Manual smoke (unautomated but recorded in the changelog):
  1. `./bin/af` → open a project → Plan → verify the drift row action cycles Keep → Overwrite → Adopt → Keep.
  2. Edit `.claude/skills/foo/SKILL.md` locally → replan → toggle row to Adopt → Apply → replan → row disappears (F==D); sibling `.codex/skills/foo/SKILL.md` shows as `update`.
  3. With `settings.git.enabled = true`: after step 2, `git log` in the profile repo shows the adopt commit; target repo shows the sync commit.
  4. Legacy state: hand-write a v2-shaped `.agentfiles/state.json`, reopen, replan — drift row toggle no longer offers Adopt (or Apply of an Adopt request surfaces `AdoptUnavailableError`); after one clean apply the Adopt option returns.

---

## 3. Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Asset folder layout is `<profile>/assets/<type>/<id>/` | `internal/asset/asset.go:184` | `dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)` |
| State file lives at `<projectPath>/.agentfiles/state.json` | `internal/config/paths.go:53,57` + `internal/sync/sync.go:404` | `const StateDirName = ".agentfiles"` / `const StateFileName = "state.json"` / `statePath := filepath.Join(preview.ProjectPath, config.StateDirName, config.StateFileName)` |
| `ManagedFiles` on-disk shape today is `map[string]string` (path → hash) | `internal/sync/sync.go:37` | `ManagedFiles map[string]string `json:"managed_files"`` |
| `GeneratorVersion = "1.0.0"` is the only version signal in state.json today | `internal/sync/sync.go:24` | `const GeneratorVersion = "1.0.0"` |
| Drift classification is a pure function of B/D/F hashes; drift means F!=D and F!=B | `internal/sync/sync.go:246-265` (`classifyDesired`) | `if state.ManagedFiles[file.Path] != "" && state.ManagedFiles[file.Path] != currentHash { return ChangeDrift, ... }` |
| Apply pre-fills `recordedHashes` with the rendered hash of every desired file | `internal/sync/sync.go:320-324` | `for _, f := range preview.Files { bodiesByPath[f.Path] = f; recordedHashes[f.Path] = utils.HashBytes(f.Body) }` |
| `DriftKeep` after ADR 0015 preserves the prior baseline (Adopt reuses this branch for the F/B side) | `internal/sync/sync.go:355-374` | `if prior := preview.ManagedState.ManagedFiles[change.Path]; prior != "" { recordedHashes[change.Path] = prior }` |
| `appapi.DriftResolutionsFromMap` emits only Overwrite today; Adopt must extend it | `internal/appapi/appapi.go:246-258` | `if decisions[ch.Path] != DriftOverwrite { continue }` |
| TUI drift toggle currently shows Keep ↔ Overwrite | `internal/tui/shell/plan_project.go:630-635` | `if s.driftResolutions[path] == appapi.DriftOverwrite { return mnemonic.New("Keep", 'p', ...) } return mnemonic.New("Overwrite", 'w', ...)` |
| TUI unknown toggle currently shows Keep ↔ Delete | `internal/tui/shell/plan_project.go:637-642` | `if s.unknownResolutions[path] == appapi.UnknownDelete { return mnemonic.New("Keep", 'p', ...) } return mnemonic.New("Delete", 'd', ...)` |
| `RenderedFile.AssetID` already carries the producing asset id at render time | `internal/render/render.go:24-33` | `AssetID string` |
| Skill projection layout is `<skill_root>/<asset_id>/<rel>` | `internal/render/render.go:231-232` | `target := filepath.ToSlash(filepath.Join(root, a.ID, rel))` |
| `asset.ResolveRelative` is the canonical containment gate for in-asset writes | `internal/asset/files.go:23-48` | `if rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) { return "", FilePathError{..., Reason: "escapes asset folder"} }` |
| Commit-trigger templates live at package scope in `internal/app/triggers.go` and dispatch via `Service.runCommit` | `internal/app/service.go:371-378`, `internal/app/triggers.go:39-46` | `func (s *Service) runCommit(t commitTrigger, ctx commitTriggerCtx, dir string) appapi.CommitOutcome` |
| `GitCommitter.Commit(dir, pathspec, msg, runHooks)` is the sole seam that talks to git | `internal/app/git.go:23-26` | `type GitCommitter interface { Commit(dir string, pathspec []string, msg string, runHooks bool) appapi.CommitOutcome; BinaryAvailable() errs.DomainError }` |
| Every managed pathspec entry is an absolute path (target repo file OR `.agentfiles/state.json`), joined at `track` | `internal/sync/sync.go:329-331` | `mutated = append(mutated, filepath.Join(preview.ProjectPath, filepath.FromSlash(rel)))` |
| Managed-surface asset-container roots come from `surfaces.AssetContainerRoots()` (used as the walk-up terminator for `OwningAssetID`) | `internal/surfaces/surfaces.go:88-127` | `func AssetContainerRoots() []string { return slices.Clone(assetContainerRootsSlice()) }` |
| ADR 0015 explicitly reserves task 0035 for Adopt — this plan is that follow-through | `docs/adr/0015-drift-keep-preserves-baseline.md:47-56` | `Accepting local edits as canonical is split out into a separate, future operation, **Adopt** (task 0035).` |

---

## 4. Acceptance criteria (mirrors `description.md`)

Each item ships in the plan step named in parentheses. Copied verbatim from `description.md` so `af.task.implement`'s DoD gate reads the same list.

- [ ] `internal/sync` exports `DriftAdopt DriftDecision = "adopt"` and `UnknownAdopt UnknownDecision = "adopt"`; `internal/appapi` mirrors both. (Step 3)
- [ ] `sync.Preview.FileChange` (unknown kind) carries `OwningAssetID string` populated when the unknown path sits under a known asset projection dir; verified by `TestPreview_ChangeUnknown_PopulatesOwningAssetIDForKnownAsset`. (Step 4)
- [ ] `.agentfiles/state.json` schema bumps to v3; each entry adds `asset_id` and `source_rel`; loader accepts v2 entries as legacy (Adopt disabled for them); verified by `TestState_LoadV2LegacyEntries_LeavesAdoptDisabled` and `TestState_WriteV3_IncludesAssetIDAndSourceRel`. (Step 2)
- [ ] `sync.Apply` with `Resolutions.Drift = [{Path:P, Decision:DriftAdopt}]` returns P in the adopt reverse-write list and marks P non-drift on next plan; verified by `TestApply_DriftAdopt_WritesProfileAndClearsDrift`. (Steps 5 + 6 — the "non-drift on next plan" half requires Step 6's profile write.)
- [ ] `sync.Apply` with `Resolutions.Unknown = [{Path:P, Decision:UnknownAdopt}]` fails when `OwningAssetID` is empty and succeeds otherwise; verified by `TestApply_UnknownAdopt_RejectsOrphanFile` and `TestApply_UnknownAdopt_WritesProfileAssetFile`. (Step 5)
- [ ] `app.Service.Apply` copies each repo file in the adopt list to `<profile>/assets/<asset_id>/<source_rel>` and invokes `GitCommitter.Commit` on the profile dir when `settings.git.enabled == true`; verified by `TestService_Apply_AdoptCommitsProfileWhenGitEnabled` and `TestService_Apply_AdoptSkipsCommitWhenGitDisabled`. (Step 6)
- [ ] TUI drift row action toggles Keep → Overwrite → Adopt → Keep; verified by `TestPlanProjectScreen_DriftToggleCyclesKeepOverwriteAdopt`. (Step 7)
- [ ] TUI unknown row action includes Adopt option only when `OwningAssetID != ""`; verified by `TestPlanProjectScreen_UnknownAdoptShownOnlyWhenOwnedByAsset`. (Step 7)
- [ ] `docs/architecture/`, `docs/adr/` (new ADR: Adopt as sanctioned reverse flow), `docs/glossary.md`, and the CLAUDE.md source-of-truth invariant updated to describe Adopt as the single sanctioned repo → profile flow; verified by manual review plus `grep -R "Adopt" docs/ CLAUDE.md` producing hits in each. (Step 8)
- [ ] Smoke: `./bin/af → project screen → plan → local edit .claude/skills/foo/SKILL.md → replan → row toggled to Adopt → apply → replan shows no drift/update on that path`. (Step 9)
- [ ] Smoke: same flow with `settings.git.enabled = true` → `git log` in the profile repo shows the Adopt commit; sibling `.codex` skill file shows as `update` on the next plan. (Step 9)

---

## 5. Out of scope (verbatim from description)

- Adopting a fully-unknown file that is not inside a known asset dir — remains the Register flow (`Service.CreateAssetFromFolder`).
- Whole-asset dir sync in a single Adopt (per-file only).
- Auto-propagating an Adopt to sibling agent projections in the same apply (siblings surface as ordinary `update` rows on the next plan).
- Adopting deletes (local delete → profile delete).
- Backfilling `asset_id`/`source_rel` on legacy v2 state entries (Adopt stays disabled until re-apply repopulates them).
- Migration tooling for existing state.json files beyond the loader's v2-legacy tolerance.
