# Plan — task#0005: Move rendering logic to TUI + structured errors

Cross-links:
- Task: `tasks/current/0005_task_move-rendering-logic-to-tui/description.md`
- New ADR: `docs/adr/0007-rendering-belongs-to-tui.md`
- Changelog: `docs/changelog/2026-04-27_0005-move-rendering-logic-to-tui.md`

## Context

Domain packages (`app`, `render`, `sync`, `doctor`, `asset`, `profile`) currently produce user-facing strings:

1. `internal/sync/FormatPreview` is a pure rendering function living in `sync` (sync.go:236).
2. `internal/doctor/CheckProfile` returns `(string, error)` — builds a human-facing health report with `fmt.Fprintf` (doctor.go:18).
3. Eleven `fmt.Errorf` sites across domain packages produce hard-coded strings, three of them inside loops that short-circuit on the first failure.

This couples domain code to presentation, blocks consistent styling (icons/colors/severity) in the TUI, and prevents callers from inspecting error metadata. Refactor pushes all formatting into `internal/tui/` and replaces string errors with typed structs that the TUI renders with lipgloss.

## Approach

### A. Typed errors, co-located with their domain

Each domain package gets an `errors.go` file with explicit error types implementing `Error()`. Loops accumulate via `errors.Join` (Go 1.20+). TUI introspects with `errors.As`.

| Package | New error types (file `errors.go`) |
|---|---|
| `internal/asset` | `ErrAssetIDNameRequired`, `UnsupportedAssetTypeError{Type}` |
| `internal/profile` | `DuplicateAssetIDError{ID}`, `DuplicateProjectIDError{ID}` |
| `internal/render` | `ExclusiveGroupConflictError{Group, AssetIDs []string}`, `AssetNotFoundError{ID}`, `AssetRenderError{AssetID, Err error}`, `TargetOutsideSurfacesError{Target}` |
| `internal/app` | `UnknownAssetError{AssetID}`, `AssetExistsError{AssetID}`, `ProjectNotFoundError{ProjectID}`, `ProjectPathOwnedError{Path, ProfileName}` |

Each `Error()` returns the previous string verbatim so existing CLI output doesn't regress when an error is printed without TUI introspection.

### B. Loop accumulation

Three sites must collect errors instead of short-circuiting; they all return `errors.Join(errs...)` when `len(errs) > 0`:

1. `render.Build` — exclusive-group check (render.go:53). Collect all conflict groups, then all asset-render errors.
2. `render.resolveAssets` — collect every missing asset id (render.go:86).
3. `app.AddProject` — collect every unknown asset id (service.go:126).
4. `app.ensureProjectPathAvailable` — collect every conflict instead of returning on first (service.go:194).

`render.addAssetOutputs` keeps fail-fast on per-asset I/O errors (loop body invokes filesystem). Aggregation happens at the level above (`render.Build`).

### C. Move rendering out of `sync` and `doctor`

1. **`sync.FormatPreview` → `tui.RenderPreview(*llmsync.Preview) string`.** New file `internal/tui/render_preview.go`. Style it with lipgloss: a colored bullet per change kind (create=green, update=yellow, drift=magenta, delete_candidate=red), icon prefix, project header in bold. Delete `FormatPreview` from `sync.go`. Update both call sites in `internal/tui/forms.go` (RunProjectPlan:188, RunProjectApply:203).

2. **`doctor.CheckProfile` → returns `*Report`.** New types in `internal/doctor/doctor.go`:
   ```go
   type Report struct {
       ProfileName string
       Projects    []ProjectStatus
   }
   type ProjectStatus struct {
       Name    string
       Changes []llmsync.FileChange   // empty == clean
   }
   ```
   `CheckProfile` builds the struct; no `fmt.Fprintf` left.
   New `internal/tui/render_report.go` with `RenderReport(*doctor.Report) string`. `RunDoctor` (forms.go:235) calls it.

### D. TUI error rendering

New file `internal/tui/render_errors.go`:

```go
func RenderError(err error) string  // unwraps errors.Join, dispatches by type
```

Behavior:
- Single typed error → one-line lipgloss-styled message: icon + colored severity + message.
- Joined errors (`errors.As(err, &interface{ Unwrap() []error })`) → list each on its own line, sorted by severity descending.

Severity assignment lives in `render_errors.go`:
- `error` (red, ✗): Unsupported/missing/conflict/already-exists.
- `warning` (yellow, ⚠): TargetOutsideSurfacesError, ProjectPathOwnedError.
- `info` (cyan, ℹ): default fallback.

Wire into `tui.reportAction` (tui.go:89) so error printing routes through `RenderError(err)` rather than `fmt.Printf("\nerror: %v\n", err)`.

### E. Lipgloss styles

New file `internal/tui/styles.go`: `var (errorStyle, warnStyle, ...)` — single import point so future styling changes stay local. lipgloss is already a transitive dep via huh; promote it to a direct require in `go.mod`.

### F. Tests

Use `t.TempDir()` and stable ids per testing guidelines. New / extended tests:

- `internal/render/render_test.go` (extend) — `TestBuild_AccumulatesExclusiveGroupConflicts`, `TestBuild_AccumulatesMissingAssets`, `TestResolveAssets_AccumulatesMissingIDs`. Use `errors.As` to assert typed errors are present in the joined result.
- `internal/profile/profile_test.go` (new) — `TestScanAssets_DuplicateIDReturnsTypedError`, same for projects.
- `internal/asset/asset_test.go` (new) — `TestValidate_UnsupportedTypeReturnsTypedError`, `TestValidate_MissingIDOrName`.
- `internal/app/service_test.go` (extend project_ownership_test.go) — `TestAddProject_AccumulatesUnknownAssets`, `TestEnsureProjectPathAvailable_AccumulatesConflicts`.
- `internal/doctor/doctor_test.go` (new) — `TestCheckProfile_ReturnsStructuredReport`, `TestCheckProfile_CleanProject`.
- `internal/tui/render_preview_test.go` — table-driven: feed Preview fixtures, assert output substrings (icon + colored kind label) without coupling to ANSI byte sequences (use `lipgloss.SetColorProfile(termenv.Ascii)` in test setup so styles strip to plain text).
- `internal/tui/render_report_test.go` — same pattern.
- `internal/tui/render_errors_test.go` — single typed error + joined errors + unknown error fallback.

## Step-by-step execution

1. **Add ADR 0007** `docs/adr/0007-rendering-belongs-to-tui.md` declaring "domain returns metadata, TUI renders". Status: accepted.
2. **Add error types** in five new `errors.go` files (asset, profile, render, app — doctor doesn't need its own, only `Report`).
3. **Refactor render.go**: replace `fmt.Errorf` with typed errors, accumulate in `Build` and `resolveAssets`.
4. **Refactor app/service.go**: replace four `fmt.Errorf` sites with typed errors, accumulate in `AddProject` and `ensureProjectPathAvailable`.
5. **Refactor asset.go + profile.go**: replace `fmt.Errorf` sites.
6. **Refactor doctor.go**: change signature to return `(*Report, error)`. Remove all `fmt.Fprintf`.
7. **Add `internal/tui/styles.go`**: lipgloss style constants.
8. **Add `internal/tui/render_preview.go`**: replaces `sync.FormatPreview`. Delete the function from `sync.go`.
9. **Add `internal/tui/render_report.go`**.
10. **Add `internal/tui/render_errors.go`** + wire into `tui.reportAction`.
11. **Update `internal/tui/forms.go`** call sites (RunProjectPlan, RunProjectApply, RunDoctor).
12. **Promote lipgloss in go.mod** from indirect to direct (it becomes a direct import).
13. **Write/extend all tests listed in §F.** Run `make test`; iterate until green.
14. **Run `make lint && make fmt && make build`** to confirm.
15. **Update docs**:
    - `docs/architecture/05-building-block-view.md` — note that TUI owns all rendering and that domain packages return typed errors.
    - `docs/architecture/08-concepts.md` (if it covers errors/output) — append error-metadata convention.
    - `docs/glossary.md` — add `Report`, `Render Error` if helpful.
    - `CLAUDE.md` — drop the "FIX: task#0005 markers" line under "Open refactor markers" (replaced by this completed work) and update its bullet to reflect the new convention.
16. **Add `docs/guidelines/errors.md`** — a short guideline file documenting the typed-error + `errors.Join` accumulation convention. Cross-link from `go_guidelines.md` and `clean_architecture.md`.
17. **Move task to in-review status**, write changelog at `docs/changelog/2026-04-27_0005-move-rendering-logic-to-tui.md`.

## Files modified / created

**Modified:**
- `internal/sync/sync.go` (delete `FormatPreview`)
- `internal/doctor/doctor.go` (`Report` types + struct return)
- `internal/render/render.go` (accumulate, typed errors)
- `internal/app/service.go` (accumulate, typed errors)
- `internal/asset/asset.go` (typed errors)
- `internal/profile/profile.go` (typed errors)
- `internal/tui/tui.go` (route errors through `RenderError`)
- `internal/tui/forms.go` (replace `FormatPreview` + `CheckProfile` string consumers)
- `go.mod` (promote lipgloss)
- `CLAUDE.md` (remove FIX-marker note)
- `docs/architecture/05-building-block-view.md`
- `docs/architecture/08-concepts.md` (if relevant)

**Created:**
- `internal/asset/errors.go`
- `internal/profile/errors.go`
- `internal/render/errors.go`
- `internal/app/errors.go`
- `internal/tui/styles.go`
- `internal/tui/render_preview.go`
- `internal/tui/render_report.go`
- `internal/tui/render_errors.go`
- `internal/tui/render_preview_test.go`
- `internal/tui/render_report_test.go`
- `internal/tui/render_errors_test.go`
- `internal/profile/profile_test.go`
- `internal/asset/asset_test.go`
- `internal/doctor/doctor_test.go`
- `docs/adr/0007-rendering-belongs-to-tui.md`
- `docs/guidelines/errors.md`
- `docs/changelog/2026-04-27_0005-move-rendering-logic-to-tui.md`
- `tasks/current/0005_task_move-rendering-logic-to-tui/plan.md` (mirror of this file)

## Verification

```bash
make fmt
make lint
make test
make build
make run ARGS="--registry /tmp/agentfiles-smoke.json"
```

Manual TUI smoke (after `make build`):
1. Create profile → add asset (skill) → add project pointing at a temp repo with two intentionally-conflicting exclusive_group assets → run "project plan". Confirm preview shows colored bullets + icons; confirm rendered error lists *all* conflicts.
2. Run "doctor" against a profile with one clean and one drifted project. Confirm Report renders with project headers, "clean" or change list, and color severity.
3. Force `unknown asset` and `project not found` paths via the TUI. Confirm `RenderError` produces the expected single-line styled output (not the raw `fmt.Errorf` string).

## Notes on scope

- `Open refactor markers` in CLAUDE.md narrowly framed task#0005 as "structured error types only". The description.md (authoritative) widens it to also moving rendering. Plan executes the wider scope.
- `WARN`-tagged comment in profile.go:138 (extract small helper) is **out of scope** — different cleanup, no `task#0005` link.
- `TODO` blocks in `render.go:102` and `asset.go:166` reference future work, not task#0005, and stay untouched.
