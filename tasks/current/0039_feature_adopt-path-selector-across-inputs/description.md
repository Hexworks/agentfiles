---
id: 0039
type: feature
status: pending
topics: tui, go
depends_on: 0037
---

# Adopt path selector modal across all path inputs

Roll out the `pathselector` modal introduced by task 0037 to the three
remaining TUI flows that still collect a filesystem path via a plain
`huh.Input`:

- `internal/tui/modals/create_profile.go`
- `internal/tui/modals/register_profile.go`
- `internal/tui/modals/register_project.go`

Each flow becomes two steps: (1) pathselector opens first, dirs only,
no constraint, start folder `$HOME`; (2) on confirm, the existing form
opens with the selected path shown as a read-only field (same pattern
as the `Type` field in the Edit Asset screen — `huh.NewInput` with
value pre-filled and Description marking it read-only). Cancelling the
pathselector aborts the whole flow. Retry-after-error re-launches the
pathselector seeded with `StartFolder = filepath.Dir(previousPath)`.

## Acceptance Criteria

- [ ] Create Profile action opens pathselector (dirs only, no constraint, start=$HOME); confirming a path opens the profile form with Name editable and Path shown as a read-only field pre-filled with the selected path.
- [ ] Register Profile action mirrors the same two-step flow.
- [ ] Register Project action mirrors the two-step flow, and the form still carries the `EnabledAgents` multi-select.
- [ ] Cancelling the pathselector aborts the flow — the follow-on form is not opened.
- [ ] Retry-after-error (validation fails, form re-opened) re-launches the pathselector with `StartFolder = filepath.Dir(previousPath)`.
- [ ] Unit tests for `buildCreateProfile`, `buildRegisterProfile`, and `buildRegisterProject` assert the resulting form has a `path` field pre-filled with the input path and whose Description marks it read-only.

## Out of scope

- `internal/tui/modals/edit_project.go` — project path should not be editable at all; tracked separately as a bug task.
- `internal/tui/modals/create_file.go` — path there is an asset-relative slug, not a filesystem path.
- Changes to `internal/tui/modals/pathselector` (owned by task 0037).

## Verification

- Baseline: `make build && make test && make lint` pass.
- `go test ./internal/tui/modals -run 'TestBuildCreateProfile|TestBuildRegisterProfile|TestBuildRegisterProject'` — asserts each build helper produces a form whose `path` field is pre-filled with the supplied path and is marked read-only in its Description.
- Smoke: `./bin/af` → Profiles → Create Profile → pathselector opens → select a folder → confirm → form opens with Path pre-filled and marked read-only → submit → new profile appears in the profile list. Repeat for Register Profile and Register Project.
