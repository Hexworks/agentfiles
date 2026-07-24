# 0045 changes

Added a read-only `[Diff]` (`d`) row action to the Plan Project screen for
`~ update` and `* drift` file rows. Pressing it opens a scrollable, viewport-
backed modal (the same frame as the `?` help dialog) showing a colored unified
diff between the on-disk file and the managed/rendered content. The direction
flips by kind: update shows current-on-disk vs. incoming-managed (what Apply
would change); drift shows managed-baseline vs. the drifted local edit (how the
file diverged). The action is absent on `+ add`, `- delete`, and `? unknown`
rows because each lacks one side of the comparison.

The feature keeps the `tui/shell → actions → app → domain` dependency direction
intact. `app.Service.DiffFile` re-renders read-only (no bodies cached in
`Preview`) and reads the local file, returning both as the new
`appapi.DiffBodies`. A new leaf component `internal/tui/components/diffview`
owns all presentation: it builds the unified-diff text with `go-udiff`, picks
the `-`/`+` direction and colors the lines, and wraps a `viewport.Model` as a
`modal.Content`. The diff is read-only — resolution still lives on the existing
Keep/Overwrite/Adopt/Delete toggles.

## Decisions

- Domain returns raw bytes (`DiffBodies`), TUI formats the diff — **Why:**
  matches the "render metadata, not policy" rule; the `-`/`+` direction and
  coloring are presentation and belong in `internal/tui`, so `go-udiff` and
  lipgloss stay out of the app/domain layers.
- `[Diff]` is appended after the drift toggles (order `Open, <toggles>, Diff`)
  — **Why:** keeps the existing matrix-test press indices (`fn(n)[i+1]`) stable
  and satisfies the acceptance criterion's `o, p/w/t, d` layout; the mnemonic
  `d` is free on update (`o` only) and drift (`o`, `p`/`w`/`t`) rows, and never
  collides with the unknown row's `[Delete]/d` because unknown rows are never
  offered a `[Diff]`.
- `DiffFile` re-renders lazily rather than caching bodies on `Preview`/
  `FileChange` — **Why:** the plan can be large and most rows are never diffed;
  re-rendering one path on demand keeps the preview lean and always current.

Not done (out of scope): editing/applying from the diff view, binary-file
detection, syntax highlighting, size caps, side-by-side layout, and diffs for
create/delete/unknown rows.

## Assumptions

- The desired body of a path equals the render output `RenderedFile.Body` for
  that same forward-slash target path — **Why:** render keys every desired file
  by that path (`render.go`), so a linear scan is the exact reverse of the
  `FileChange` the row was built from.
- Showing a diff of a symlinked local file needs no symlink guard — **Why:**
  unlike Adopt (which writes repo bytes back into the profile and can push them
  to a remote), the diff is display-only and never propagates content, so the
  Adopt-style `Lstat` refusal is unnecessary here.

## Other Notes

- Promoted `github.com/aymanbagabas/go-udiff v0.4.1` from a `go.sum`-only
  transitive dependency (via glamour) to a direct `require` in `go.mod`.
- Docs: documented the action in `docs/manual/plan_project.md` (backs the `?`
  help topic) and added the `diffview` component + `DiffFile` seam to
  `docs/architecture/05-building-block-view.md`.
- No ADR — the feature follows established patterns (read-only render reuse,
  the existing modal/actions/seam conventions); promoting an already-resolved
  transitive dependency is not a durable architectural decision.
- Tests: real-stack `TestDiffFile` (real profile/asset/project in `t.TempDir()`,
  real `render.Build` + `os.ReadFile`, one drifted + one updated file) and
  `TestDiffFile_LocalReadFailureReturnsTypedError`; `diffview` builder unit
  tests for both directions and the equal-bodies message; shell `TestDiffButton`
  plus dispatch/error-render tests; an actions forwarding test. Updated the two
  existing plan-project tests that assumed update rows carry only `[Open]` and
  that drift rows carry no trailing button.

## `app.Service.DiffFile`

New read-only service method: resolve the project, re-render, match the desired
body by path, read the local body.

```go
// after — re-renders read-only; typed errors for missing-from-plan and local read failure
func (s *Service) DiffFile(profileRef, projectID, path string) (appapi.DiffBodies, errs.DomainError) {
	loaded, proj, err := s.resolveProject(profileRef, projectID)
	if err != nil {
		return appapi.DiffBodies{}, err
	}
	plan, buildErrs := render.Build(loaded.Profile, proj)
	if len(buildErrs) > 0 {
		return appapi.DiffBodies{}, errs.Errors(buildErrs)
	}
	desired, found := desiredBody(plan, path)
	if !found {
		return appapi.DiffBodies{}, DiffDesiredMissingError{Path: path}
	}
	local, readErr := os.ReadFile(filepath.Join(proj.Path, filepath.FromSlash(path)))
	if readErr != nil {
		return appapi.DiffBodies{}, DiffLocalReadError{Path: path, Err: readErr}
	}
	return appapi.DiffBodies{Local: local, Desired: desired}, nil
}
```

## Plan Project `[Diff]` row action

The file-row action factory now offers `[Diff]` on update and drift rows, and a
new `diffReadyMsg` carries the fetched bodies (or a typed error) back to
`handleDiffReady`, which opens the diff modal.

```go
// before
btns := []*mnemonic.Button{s.openFileBtn(d.path)}
switch d.change.Kind {
case appapi.ChangeDrift:
	btns = append(btns, s.driftToggleButtons(d.path, d.change.AdoptProvenance.Available())...)
case appapi.ChangeUnknown:
	btns = append(btns, s.unknownToggleButtons(d.path, d.change.OwningAssetID != "")...)
}
```

```go
// after — update and drift are the only kinds with both a desired and a local body
btns := []*mnemonic.Button{s.openFileBtn(d.path)}
switch d.change.Kind {
case appapi.ChangeUpdate:
	btns = append(btns, s.diffFileBtn(d.path, d.change.Kind))
case appapi.ChangeDrift:
	btns = append(btns, s.driftToggleButtons(d.path, d.change.AdoptProvenance.Available())...)
	btns = append(btns, s.diffFileBtn(d.path, d.change.Kind))
case appapi.ChangeUnknown:
	btns = append(btns, s.unknownToggleButtons(d.path, d.change.OwningAssetID != "")...)
}
```
