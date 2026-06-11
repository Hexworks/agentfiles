# Plan — 0022 Form Modals

See [description.md](./description.md) for the full task body and modal specs.
Parent UI design: `tasks/done/0015_task_refactor_ui/description.md` (Modals section).

## Goal

Add six `huh.Form`-backed modals under `internal/tui/modals/` plus their typed
result structs. Each modal exposes `New(...) *modal.Modal` and uses the existing
`modal.NewForm` adapter so `*huh.Form` does not leak.

## Design Decisions

1. **One file per modal.** Layout:
   ```
   internal/tui/modals/
     create_profile.go
     register_profile.go
     create_asset.go
     edit_project.go
     register_project.go
     create_file.go
   ```
2. **Result types live next to their modal.** `CreateProfileInput`,
   `RegisterProfileInput`, `EditProjectInput`/`RegisterProjectInput`,
   `CreateFileInput`. The Create Asset modal returns an `asset.Manifest` directly
   (description states this).
3. **Value pointers + initial values.** Each constructor accepts an `initial`
   struct (zero-value when there is no prefill). Fields are bound with
   `huh.NewInput().Value(&local.Field)`, etc. `extract` closes over the same
   locals and returns the typed struct.
   - Allows the **Edit Project** prefill round-trip.
   - Makes tests deterministic: tests pass initial values and then just drive
     `Enter` keys (no need to type each character).
4. **Validators are local input shape only.** "Required" enforced via
   `Validate(func(string) error { … })` returning a small sentinel error.
   Domain validation (path safety, slug collisions, ownership) stays on
   `app.Service` and is invoked by the **screens that open** these modals —
   out of scope here.
5. **Slugging for asset id.** Per description: the asset id is the slug of
   `Name`. Move `slug` from `internal/app/service.go` to `internal/utils/slug.go`
   so the modals can reuse it without importing `internal/app`. Update the
   single existing call site in `service.go`.
6. **Multi-select agents.** Use `huh.NewMultiSelect[string]()` with the four
   options `codex`, `claude-code`, `cursor`, `opencode`. Defined as exported
   constants in a new `internal/tui/modals/agents.go` so the option list is not
   duplicated across `create_asset.go`, `edit_project.go`,
   `register_project.go`. (Domain owns the names already; this is the TUI-side
   option list — see `tui.md` §"Use `huh` For Forms".)
7. **Asset type select.** `huh.NewSelect[asset.Type]()` populated from
   `asset.AllTypes()` so adding a new asset type does not also need a TUI edit.
8. **Tags csv.** A single `huh.NewInput()` field; `extract` splits on `,`,
   trims, drops empties. (No `MultiSelect` because tags are open-ended.)
9. **Modal id strings.** Stable, dash-separated: `"create-profile"`,
   `"register-profile"`, `"create-asset"`, `"edit-project"`,
   `"register-project"`, `"create-file"`. Screens key off these in
   `ResolvedMsg.ID`.

## Step-by-Step Execution

1. **Move `slug` to `internal/utils`.**
   - New file `internal/utils/slug.go` with exported `Slug(string) string`.
   - Update `internal/app/service.go` line 133 caller and delete the private
     `slug` from `service.go`.
   - Add a tiny test `internal/utils/slug_test.go` for the existing rules
     (lowercase, spaces/underscores→dashes, fallback `"item"`).

2. **Shared agents option list.**
   - `internal/tui/modals/agents.go` declares the four agent option labels &
     values. Exposes `AgentOptions() []huh.Option[string]`.

3. **`create_profile.go`.**
   - `type CreateProfileInput struct{ Name, Path string }`
   - `New(initial CreateProfileInput) *modal.Modal`
   - Two `huh.NewInput()` fields with required validators.

4. **`register_profile.go`.**
   - `type RegisterProfileInput struct{ Path string }`
   - `New(initial RegisterProfileInput) *modal.Modal`
   - One `huh.NewInput()` field.

5. **`create_asset.go`.**
   - `New(initial asset.Manifest) *modal.Modal` — returns `asset.Manifest`.
   - Fields:
     - `Name` (Input, req)
     - `Type` (Select[asset.Type], req)
     - `Description` (Text, req)
     - `Tags` (Input, optional csv → `[]string`)
     - `CompatibleAgents` (MultiSelect[string], optional)
     - `ExclusiveGroup` (Input, optional)
   - `extract`:
     - `m.ID = utils.Slug(m.Name)`
     - parses tags
     - returns the `asset.Manifest`

6. **`edit_project.go`.**
   - `type EditProjectInput struct{ Name, Path string; EnabledAgents []string }`
     — keeps `ID` and `SelectedAssetIDs` out of the wire format.
   - `New(existing *project.Manifest) *modal.Modal`
     - Prefills `EditProjectInput` from `existing`.
     - extract returns the **modified `*project.Manifest`** (clone of input
       with `Name`/`Path`/`EnabledAgents` overwritten) — description says
       "Result: modified `*project.Manifest`".
   - Required: name, path, EnabledAgents (≥1) via MultiSelect `Validate`.

7. **`register_project.go`.**
   - `type RegisterProjectInput struct{ Name, Path string; EnabledAgents []string }`
   - `New(initial RegisterProjectInput) *modal.Modal`
   - `extract` returns `*project.Manifest{Name, Path, EnabledAgents, SelectedAssetIDs: nil}`.
     ID is left empty — slug is the screen's responsibility when it calls
     `AddProject` (which already slugs); description says the screen calls
     `AddProject` after submit.
   - Required: name, path, EnabledAgents (≥1).

8. **`create_file.go`.**
   - `type CreateFileInput struct{ Path string }`
   - `New(initial CreateFileInput) *modal.Modal`
   - One `huh.NewInput()` field.

9. **Tests** (`internal/tui/modals/*_test.go`).
   - For each modal, drive submission and confirm `ResolvedMsg.Value`
     carries the typed payload with the prefilled fields:
     1. Build with non-zero initial values that already satisfy required.
     2. Loop sending `tea.KeyPressMsg{Code: tea.KeyEnter}` to advance through
        fields until the form reports `huh.StateCompleted`.
     3. The last `Update` returns a command; drain it (helper modeled on
        `modal/modal_test.go::drainResolved`) and assert on the typed payload.
   - Cancel test (one shared helper exercised against `create_file` is
     sufficient since cancel is implemented entirely inside `modal/form.go`
     and that path is already covered there — but description asks per-modal,
     so add a thin cancel test for each modal that injects
     `tea.KeyPressMsg{Code: tea.KeyEscape}` and asserts
     `ResolvedMsg{Confirmed: false, Value: nil}`).
   - Edit Project: round-trip — build with an existing `*project.Manifest`,
     submit without changes, assert returned manifest equals the input
     (except potential `EnabledAgents` order which we normalize via
     `slices.Sort` to match `project.Manifest.Normalize`).
   - Create Asset: assert `Name → ID` slug works (e.g.
     `Name: "Hello World" → ID: "hello-world"`) and csv tag parsing
     (`"a, b ,c" → ["a","b","c"]`).

10. **Verification.** `make build && make test && make lint`.

## Edge Cases / Notes

- Keyboard simulation through `huh` requires fields to be already
  satisfied (prefilled or pressing keys that satisfy validators) before
  `Enter` advances. Tests therefore use initial values that already pass
  validators.
- `huh.NewMultiSelect[string]().Value(*[]string)` — the slice's existing
  contents become the preselected options; this is how prefill works.
- Cancel via `<esc>`: `huh.Form` flips to `huh.StateAborted` on Esc, the
  modal adapter (`form.go`) translates to `Cancelled`, the modal emits
  `ResolvedMsg{Confirmed: false, Value: nil}` — already covered behavior.
- `*huh.Form` must not leak: every `extract` returns a typed value.

## Documentation Touched

- None of the architecture docs / ADRs require updates. This task only adds
  TUI components that follow established patterns (form adapter, modal,
  `huh`, `slug` reuse). The parent design (`0015_task_refactor_ui`) already
  documents the modal specs.
- Possibly extend `docs/glossary.md` if a reviewer wants "form modal"
  defined; skipped by default to avoid noise.

## Files

| File                                       | Change |
| ------------------------------------------ | ------ |
| `internal/utils/slug.go`                   | new    |
| `internal/utils/slug_test.go`              | new    |
| `internal/app/service.go`                  | edit (use `utils.Slug`, drop private) |
| `internal/tui/modals/agents.go`            | new    |
| `internal/tui/modals/create_profile.go`    | new    |
| `internal/tui/modals/register_profile.go`  | new    |
| `internal/tui/modals/create_asset.go`      | new    |
| `internal/tui/modals/edit_project.go`      | new    |
| `internal/tui/modals/register_project.go`  | new    |
| `internal/tui/modals/create_file.go`       | new    |
| `internal/tui/modals/*_test.go`            | new    |
