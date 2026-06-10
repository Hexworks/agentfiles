# Task 0017 — Sync: ChangeUnknown classification + Resolutions API

Cross-links:
- Task body: [`description.md`](./description.md)
- Parent task (UI refactor, "Plan Project" / "Sync Project" sections): [`../0015_task_refactor_ui/description.md`](../0015_task_refactor_ui/description.md)
- Architecture: [`../../../docs/architecture/05-building-block-view.md`](../../../docs/architecture/05-building-block-view.md), [`../../../docs/architecture/08-concepts.md`](../../../docs/architecture/08-concepts.md)

---

## Context

`internal/sync/sync.go` already declares a `ChangeUnknown` constant (line 57, currently untyped — bug) but no code path emits it. `detectDeleteCandidates` lumps every non-desired file inside a managed surface into `ChangeDelete`, regardless of whether the file was ever tracked. `Apply`'s signature is a single `deleteCandidates bool` — all-or-nothing — which cannot express per-file user choices ("keep this drift, delete that unknown").

This task carves the sync-engine slice out of the parent UI refactor (0015). It is **purely** `internal/sync` + the call sites that depend on its signature. The Plan Project Screen and the `[Overwrite]/[Keep]/[Delete]` toggle UI ship in task 0029; for now the existing TUI must just compile.

Why this matters:
- The "unknown" case has a different safety profile than "delete": delete is auto-applied (recognized, intentional removal), unknown requires explicit user opt-in (we never managed it).
- The bool-flag `deleteCandidates` is a SOLID/clean-code violation — a flag arg switching between unrelated behaviors — and blocks per-file UX.
- First-apply must not surface stray `.claude/` files as `ChangeUnknown` — that would flood every new project with noise. Treat first apply as a clean slate.

---

## Files to modify

| File | Why |
| --- | --- |
| `internal/sync/sync.go` | Core refactor: fix `ChangeUnknown` const, split classification, new `Resolution`/`FileResolution` types, new `Apply` signature, drop `Preview.DeleteCandidates`, first-apply clean-slate logic. |
| `internal/sync/sync_test.go` | Replace single existing test with table-driven coverage of the new classification + resolution semantics. |
| `internal/app/service.go` (line 171) | Update `Service.Apply` signature to take `resolutions []llmsync.FileResolution` instead of `deleteCandidates bool`. |
| `internal/tui/forms.go` (lines 193–233, `RunProjectApply`) | Pass an empty `[]sync.FileResolution{}` to `service.Apply`; remove the legacy `DeleteCandidates` confirmation dialog (it can no longer compile since the field disappears). UI redo is task 0029. |
| `internal/tui/render_preview.go` (line 33, `changeStyle`) | Add a `ChangeUnknown` case so preview rendering surfaces unknowns instead of falling through to the muted `?` glyph. |
| `internal/tui/render_preview_test.go` | Add an assertion that `ChangeUnknown` renders with its own glyph. |
| `internal/doctor/doctor.go` (lines 86–87) | Already handles `ChangeDelete`; add a parallel `ChangeUnknown` case in `convertChanges` so doctor reports unknowns instead of dropping them. |

No new files. No changes to `render`, `surfaces`, `errs`, or `utils`.

---

## Execution plan

### Step 1 — Fix the `ChangeUnknown` constant type

`sync.go:57` reads `ChangeUnknown = "unknown"` — missing the `ChangeKind` type, so it's currently a bare `string` constant. Change to:

```go
ChangeUnknown ChangeKind = "unknown"
```

Drive-by; required for any of the new code to compile against it. Also extend the docstring to say "an unrecognized file that lives inside a managed surface but was never tracked in ManagedState".

### Step 2 — Add `Resolution` and `FileResolution` types

In `sync.go`, after the `ChangeKind` block:

```go
// Resolution is the per-file decision the user makes when applying a preview.
// It only matters for ChangeDrift (overwrite vs keep) and ChangeUnknown
// (delete vs keep); create/update/delete carry ResolveAuto and are always
// materialized.
type Resolution int

const (
    ResolveAuto      Resolution = iota // create / update / delete — always applied
    ResolveOverwrite                    // drift only
    ResolveKeep                         // drift or unknown — no-op
    ResolveDelete                       // unknown only
)

// FileResolution pairs a target path with the user's per-file choice.
// Paths absent from the resolutions slice keep the default for their ChangeKind.
type FileResolution struct {
    Path       string
    Resolution Resolution
}
```

Note: the TUI already has a `modal.ResolutionState` type for modal lifecycle (Active/Confirmed/Cancelled). Different package, different concept — no collision in practice.

### Step 3 — Split classification in `Plan`

Replace the `detectDeleteCandidates` pass with two separate passes:

1. **State-recorded deletes** — files in `state.ManagedFiles` that are missing from `desired`. Emit `ChangeDelete`, reason "recognized llm file not selected". Only emitted when `state != nil`.
2. **Surface unknowns** — files inside any `surfaces.Roots()` path that are neither in `desired` nor in `state.ManagedFiles`. Emit `ChangeUnknown`, reason "unrecognized file in managed surface". **Only emitted when `state != nil`** — first-apply clean-slate (see Step 4).

Implementation: rename `detectDeleteCandidates` to `detectExtraneous` returning `(deletes []string, unknowns []string, []errs.DomainError)`. Walk identical; only the bucket each file lands in changes, driven by `state.ManagedFiles` membership.

`Plan` then loops both slices and emits `ChangeDelete` / `ChangeUnknown` entries. Sort + return preview as before.

### Step 4 — First-apply policy

When `state == nil` (no `.agentfiles/state.json`):
- Skip the `detectExtraneous` call entirely.
- All desired files classify as `ChangeCreate` even if a file already exists at the path. Short-circuit before the `utils.Exists` check: `if state == nil { changes = append(..., ChangeCreate, reason: "first apply"); continue }`. Existing files at the same path get silently overwritten on `Apply` — documented clean-slate behavior.
- No `ChangeUnknown` entries are emitted; existing files in managed surfaces are ignored.
- The first successful `Apply` writes the initial `ManagedState` (already happens). Subsequent plans then distinguish drift from unknown normally.

### Step 5 — Drop `Preview.DeleteCandidates`

Remove the field from `Preview` (the TODO at sync.go:78 is finally actionable). Remove the assignment at line 138. All delete info now flows through `Changes`.

### Step 6 — New `Apply` signature

```go
func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError
```

Build `map[string]Resolution` from the slice (last entry wins on duplicate paths). Iterate `preview.Changes`:

- `ChangeCreate`, `ChangeUpdate`: always write the rendered body.
- `ChangeDelete`: always remove the file (resolution lookup ignored — `Plan` is the opt-in surface, user already saw the preview).
- `ChangeDrift`: write the rendered body **only** if `resolutions[path] == ResolveOverwrite`. Default `ResolveKeep`: no-op.
- `ChangeUnknown`: remove the file **only** if `resolutions[path] == ResolveDelete`. Default `ResolveKeep`: no-op.

Build a `map[string]render.RenderedFile` keyed by path so the `Changes` iteration can fetch bodies for create/update/overwrite-drift. The `Changes` slice is the authoritative source of truth for what `Apply` does, not the parallel `Files` slice.

Managed-state rewrite: record hashes of every file actually written (Create + Update + Overwrite-Drift). Files that were kept (drift kept or unknown kept) are **not** added to managed state. Files that were deleted (auto delete or unknown delete) drop out naturally.

Errors accumulate via the existing pattern: `[]errs.DomainError`, `errs.Errors(...)` if multiple, single if one, nil if none.

### Step 7 — Update `app.Service.Apply`

`service.go:171` becomes:

```go
func (s *Service) Apply(profileRef, projectID string, resolutions []llmsync.FileResolution) (*llmsync.Preview, errs.DomainError) {
    preview, err := s.Plan(profileRef, projectID)
    if err != nil {
        return nil, err
    }
    if err := llmsync.Apply(preview, resolutions); err != nil {
        return nil, err
    }
    return preview, nil
}
```

### Step 8 — Update TUI to compile

`internal/tui/forms.go` (`RunProjectApply`, lines 193–233):
- Delete the `if len(preview.DeleteCandidates) > 0 { ... }` confirmation dialog (lines 204–215). Field no longer exists.
- Change final `service.Apply(profileID, projectID, deleteCandidates)` to `service.Apply(profileID, projectID, []llmsync.FileResolution{})`. Add the `llmsync` import alias if needed.
- This loses the "delete recognized unmanaged files?" prompt entirely — that's correct per the task. Existing flow is retired in 0021.

`internal/tui/render_preview.go` (`changeStyle`, line 33): add `case llmsync.ChangeUnknown: return "?", driftStyle`.

### Step 9 — Update `doctor`

`internal/doctor/doctor.go:86–87`: `convertChanges` switch handles `ChangeDelete`. Add parallel case for `ChangeUnknown`.

### Step 10 — Rewrite tests

`internal/sync/sync_test.go` — reshape into focused per-behavior tests:

- `TestPlan_FirstApply_NoStateNoUnknowns` — state.json absent, desired = `[AGENTS.md]`, a stray `.codex/old.txt` exists. Expect: one `ChangeCreate` for `AGENTS.md` (reason "first apply"), zero unknowns, zero deletes.
- `TestPlan_FirstApply_DesiredFileOverwritesExisting` — state.json absent, desired = `[AGENTS.md]`, `AGENTS.md` already exists with different content. Expect: `ChangeCreate`.
- `TestPlan_SubsequentApply_StateRecordedDelete` — state.json lists `.codex/old.txt`, desired empty for that path, file exists. Expect: `ChangeDelete`.
- `TestPlan_SubsequentApply_UnknownFile` — state.json lists `AGENTS.md` only, desired = `AGENTS.md` only, `.codex/stray.txt` exists in project. Expect: `ChangeUnknown` for `.codex/stray.txt`.
- `TestPlan_SubsequentApply_DriftDetected` — existing drift assertion preserved.
- `TestApply_ResolveKeep_LeavesDriftAlone` — drift with `ResolveKeep`: on-disk content unchanged.
- `TestApply_ResolveOverwrite_RewritesDrift` — drift with `ResolveOverwrite`: on-disk content matches rendered body.
- `TestApply_ResolveDelete_RemovesUnknown` — unknown with `ResolveDelete`: file removed.
- `TestApply_DefaultUnknown_LeavesAlone` — empty resolutions: file still exists.
- `TestApply_StateRewritten` — after Apply with mix of changes, `.agentfiles/state.json` contains only files actually written.

`render_preview_test.go` needs a `ChangeUnknown` rendering case.

---

## ADR / docs / guidelines impact

- **ADR**: Create `docs/adr/0011_sync_resolutions.md` (verify next sequential number). Title: "Sync resolution model: explicit per-file user decisions". Covers: (a) four-resolution enum + rationale, (b) first-apply clean-slate, (c) why `ChangeDelete` auto-applies while `ChangeUnknown` requires opt-in.
- **Architecture**: update `docs/architecture/08-concepts.md` — rewrite "Safety-First Deletion" section. Add a brief "First-Apply Clean Slate" subsection.
- **Building block view**: update `docs/architecture/05-building-block-view.md` `sync` paragraph (lines 102–107) — replace "detects recognized delete candidates" with "detects recognized delete candidates and unknown files, applies per-file resolutions".
- **Guidelines**: update `docs/guidelines/sync_and_safety.md` Rule 3 — opt-in is now per-file via `ResolveDelete`.
- **No new guideline files.**

---

## Verification

```
make build && make test && make lint
```

End-to-end resolution UX (toggle buttons) lands in task 0029; manual verification of `ResolveOverwrite` / `ResolveDelete` is via the unit tests in step 10.
