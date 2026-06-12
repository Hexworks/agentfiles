# Edit Profile screen review

Implementation matches the task description and plan: real `editProfileScreen` replaces the stub, three downstream nav stubs ship for tasks 0027/0028/0029, focus + mnemonic + modal patterns mirror the Profiles screen, and the safety contract ("Delete Project keeps repo files") is encoded both in the prompt and in a sentinel-file test. Build green, tests cover the matrix from the description.

The review found two categories of issues:

1. **Domain rule leaks into the TUI.** `afterEditProject` mutates the loaded `*project.Manifest` pointer in place and only then asks the service to persist; the "which fields are editable, which must be preserved" merge policy lives in shell instead of the project package. Asset ordering is re-implemented at the edge instead of mirroring `Profile.ProjectList()`. The Delete Project prompt invents UX vocabulary for a term the glossary already names ("Orphaned File").
2. **Duplication that will compound as more screens land.** `editProfileMutationCmd[T]` + `editProfileMutationDoneMsg` are byte-for-byte copies of the profiles-screen pair. The cursor-row ANSI-underline trick is open-coded three times across two files. The three navigation stubs and their five-test suites are mechanical clones — six files of mostly-identical Screen plumbing.

The rest of the findings are smaller: a few tests assert on internal modal-ID strings instead of observable behavior, a `prof` field exists only as a nil flag, the `bodyContent` layout math hides magic constants and side effects, and a couple of security-edge defenses (`pendingDelete*` field cleanup, delete-asset cascade language) would tighten the modal lifecycle.

Tick exactly one checkbox per issue and come back.

## In-place mutation of loaded `*project.Manifest` inside the TUI

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Core Rules; Architecture Boundaries)
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) (Put Domain Rules In Domain Code; Model Consistency Boundaries; Keep Application Flow Thin)
> - [`docs/guidelines/tui.md`](../../../docs/guidelines/tui.md) (Core Rule; Render Metadata, Not Policy)
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) (Treat External Input As Untrusted — accept partially valid manifests)

`afterEditProject` reaches into the live `*project.Manifest` returned by `s.selectedProject()` (the same pointer held by `s.prof.Projects[id]` and surfaced through `s.projectsList`), overwrites three fields, and then asks `UpdateProject` to persist it. Two problems flow from this:

- **The merge contract is a domain rule.** "Name / Path / EnabledAgents are editable; ID / SelectedAssetIDs / CreatedAt survive" defines what `EditProject` _means_ at the project-aggregate level. `modals.EditProjectInput` even documents the contract in its godoc, but the rule is then _re-implemented_ in shell. A second caller (a future test fixture, a different screen) would have to replicate it.
- **The in-memory profile becomes dirty before the service confirms.** A user-supplied `Path` is grafted onto the cache pre-validation; on `UpdateProject` failure the on-disk manifest is unchanged but the table briefly renders the rejected values until the next `loadCmd()` tick. The consistency boundary is split across a UI-side partial update and a domain-side persistence step.

```go
// internal/tui/shell/edit_profile.go:681-695
target, ok := s.selectedProject()   // pointer into s.prof.Projects
if !ok { return nil }
target.Name = in.Name                // policy: "Name is editable here"
target.Path = in.Path                // user-supplied path written into live cache pre-validation
target.EnabledAgents = append([]string(nil), in.EnabledAgents...)
return editProfileMutationCmd(
    func() (struct{}, errs.DomainError) {
        return s.actions.UpdateProject(actions.UpdateProjectInput{
            ProfileRef: s.profileID,
            Project:    target,      // mutated domain pointer passed back in
        })
    }, ...)
```

- [x] Move the merge into `app.Service.UpdateProject`: extend `actions.UpdateProjectInput` to take `{ProjectID, Name, Path, EnabledAgents}`, let the service load the manifest, apply the edit, validate, and save atomically. The TUI ships only form values.
- [ ] Keep the current action shape but add `project.ApplyEdit(existing *Manifest, in EditInput) *Manifest` that returns a deep-copied, validated manifest; the screen passes the copy to `UpdateProject` and only swaps it into `s.prof.Projects` on success.

## Duplicated mutation envelope + generic helper across screens

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Reuse/Release Equivalence Principle; Common Closure Principle)
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Code Smells: needless repetition)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) (OCP)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) (Keep Packages Cohesive)

`editProfileMutationCmd[T]` (`edit_profile.go:705`) and `mutationCmd[T]` (`profiles.go:482`) have identical signatures, identical bodies, and differ only by the envelope they return (`editProfileMutationDoneMsg` vs `profileMutationDoneMsg`). Both envelopes carry the same `{text string, severity errs.Severity}` pair. The author's own comment cross-references the duplication ("same testability rationale as `profilesScreen.mutationCmd`"). Each new screen that mutates state will fork another copy. As an additional smell, the generic `T` is structurally unused — every call site discards the action's first return value.

```go
// internal/tui/shell/profiles.go:482
func mutationCmd[T any](action func() (T, errs.DomainError), successText string) tea.Cmd { /* identical body */ }

// internal/tui/shell/edit_profile.go:705 — same body, different envelope
func editProfileMutationCmd[T any](action func() (T, errs.DomainError), successText string) tea.Cmd { /* identical body */ }

type profileMutationDoneMsg     struct { text string; severity errs.Severity }
type editProfileMutationDoneMsg struct { text string; severity errs.Severity }
```

- [x] Lift one shared `mutationDoneMsg` + one shared `mutationCmd` (drop the unused generic) into `internal/tui/shell/mutation.go`; both screens consume them. Disambiguate by screen-local context, not by message type.
- [ ] Keep separate envelopes for now but extract a non-generic `runMutation(action func() errs.DomainError, success string) (text string, sev errs.Severity)` helper both screens delegate to. Removes the body duplication without touching the message types.

## Three near-identical navigation stubs

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Code Smells: needless repetition)
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Reuse/Release Equivalence Principle)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) (OCP)

`edit_asset_stub.go`, `select_project_assets_stub.go`, and `plan_project_stub.go` differ only in: type name, title string, body sentence, future task number, and one ID field name. Each implements the same five `Screen` methods with identical bodies. Any change to the Back-key contract or the body layout requires editing six files. The five-test suite per stub is similarly duplicated (next finding).

```go
// All three stubs share this body verbatim:
func (s *editAssetStub) Update(msg tea.Msg) (Screen, tea.Cmd) {
    kp, ok := msg.(tea.KeyPressMsg)
    if !ok { return s, nil }
    if s.back.Matches(kp) { return s, s.back.Trigger() }
    return s, nil
}
```

- [ ] Collapse the three stubs into a single `placeholderScreen{title, taskNumber, body string, back *mnemonic.Button}` plus three thin constructors that format the body string once.
- [x] Keep three files for clarity-of-future-replacement but extract a `backOnlyScreenBase` embedded struct so the boilerplate (Update + StatusKeys + the back wiring) lives once.

## Three near-identical stub test files

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Keep Test Code Simple; Start With The Smallest Useful Test — "duplicate the same behavior at every layer")
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Code Smells: needless repetition)

`edit_asset_stub_test.go`, `plan_project_stub_test.go`, and `select_project_assets_stub_test.go` are clones: five tests each (`BTriggersPop`, `EscTriggersPop`, `UnrelatedKeyIsNoop`, `TitleAndStatusKeysExposeBack`, `BodyIncludes…IDs`), identical bodies, only the constructor name and a title string changed. ~200 LOC restating the same back-key behavior. Each stub is throwaway; the tests should mirror that.

```go
// Same test, three files:
func TestEditAssetStub_BTriggersPop(t *testing.T)            { /* identical */ }
func TestSelectProjectAssetsStub_BTriggersPop(t *testing.T)  { /* identical */ }
func TestPlanProjectStub_BTriggersPop(t *testing.T)          { /* identical */ }
```

- [x] Collapse into one table-driven `stub_screens_test.go` parametrized over `(constructor, title, idLabels)`.
- [ ] Trim each file to one smoke test that asserts the stub satisfies `Screen` and exposes the expected title; rely on the shared placeholder type's tests for back behavior.

## ANSI underline trick triplicated across two files

> [!WARNING]
>
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Reuse/Release Equivalence Principle)
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Code Smells: needless repetition; Naming #4 magic strings)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) (Prefer explicit types over loose strings)

`actionsCellContent` (`profiles.go:284`), `assetActionsCellContent` (`edit_profile.go:455`), and `projectActionsCellContent` (`edit_profile.go:468`) each redeclare the same `"\x1b[4m"` / `"\x1b[24m"` SGR constants — once as `underlineOn`/`underlineOff`, once again the same way, and once renamed to `on`/`off`. The reason for the manual escapes (lipgloss emits a full `\x1b[0m` reset that breaks the surrounding cursor-row highlight) is documented only in `profiles.go`; the edit_profile copies hide the rationale. A fourth and fifth copy is inevitable when tasks 0027/0028 add their own cell renderers.

```go
// profiles.go:284
const ( underlineOn = "\x1b[4m"; underlineOff = "\x1b[24m" )
// edit_profile.go:455 — same constants, same name
const ( underlineOn = "\x1b[4m"; underlineOff = "\x1b[24m" )
// edit_profile.go:468 — same constants, different name
const ( on = "\x1b[4m"; off = "\x1b[24m" )
```

- [ ] Add `mnemonic.UnderlineRune(r rune) string` (or `mnemonic.ActionsCell(labels ...string) string`) so the SGR codes live in one place and the three screens become one-liners.
- [x] Lift `underlineOn` / `underlineOff` plus a small `underline(s string) string` helper into `internal/tui/shell/cellrender.go` and call it from all three sites.

## `prof` field exists only as a nil flag

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Data And Objects #5 keep structs focused; Functions #5 hidden state)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) (Keep Packages Cohesive)

`editProfileScreen.prof` is written in the `editProfileLoadedMsg` branch and read **only** by `rebuildLists` (as a `s.prof == nil` check). The two derived slices `assetsList` / `projectsList` are the actual source of truth everywhere else. Two pointers to the same data invite drift: a future caller that pokes `s.prof.Projects` between `loadCmd` and `rebuildLists` sees stale state.

```go
type editProfileScreen struct {
    ...
    prof         *profile.Profile   // only used as a nil flag in rebuildLists
    assetsList   []*asset.Asset
    projectsList []*project.Manifest
    ...
}
```

- [x] Drop the field. Have `rebuildLists(prof *profile.Profile)` take the loaded profile as a parameter; the `editProfileLoadedMsg` handler passes `m.prof` directly.
- [ ] Keep `prof` but make `rebuildLists` and all readers consult it (not the derived slices), so the field becomes load-bearing instead of redundant.

## Asset ordering reinvented at the TUI edge; `Profile.AssetList()` missing

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) (Put Domain Rules In Domain Code; Use The Project Language)
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) (Keep Packages Cohesive)

`profile.Profile.ProjectList()` exists with a "stable TUI presentation" contract. Asset ordering follows the same shape — map → slice, sort by `Name` — but lives in the screen (`rebuildLists`, `edit_profile.go:358`) instead of the domain. The next screen that lists assets must either re-implement the rule or import this one. The `sort.Slice` form also disagrees with the codebase trend toward `slices.SortFunc`.

```go
// internal/tui/shell/edit_profile.go:358 — domain ordering rule re-implemented at the edge
assets := make([]*asset.Asset, 0, len(s.prof.Assets))
for _, a := range s.prof.Assets {
    assets = append(assets, a)
}
sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
s.assetsList = assets
s.projectsList = s.prof.ProjectList() // symmetric, but defined in the domain
```

- [x] Add `(*Profile).AssetList() []*asset.Asset` next to `ProjectList()` with the same sort-by-Name guarantee; the TUI becomes a one-liner using `slices.SortFunc` internally.
- [ ] Keep the sort in the screen but switch `sort.Slice` to `slices.SortFunc` for consistency with the rest of the codebase.

## Delete Project prompt invents UX vocabulary already in the glossary

> [!WARNING]
>
> - [`docs/guidelines/domain_model.md`](../../../docs/guidelines/domain_model.md) (Use The Project Language; Preserve Source-Of-Truth Semantics)
> - [`docs/glossary.md`](../../../docs/glossary.md) (Orphaned File)

The Delete Project confirmation says "only metadata — repo files are kept". The glossary already names the residual state "Orphaned File" and `Service.DeleteProject`'s godoc uses the same phrase ("remain on disk as orphaned files"). Three vocabularies for the same domain term: the prompt, the godoc, and the glossary. The word "metadata" also obscures _which_ artifact is being deleted (the project manifest).

```go
// edit_profile.go:555-559
prompt := fmt.Sprintf(
    "Are you sure you want to delete project %q? (only metadata — repo files are kept)",
    p.Name,
)
```

- [x] Rephrase: `"Delete project manifest %q? Files in the target repo become orphaned files and are kept on disk."` so the prompt, the service godoc, and the glossary share one vocabulary.
- [ ] Leave the prompt phrasing as-is, but add a glossary cross-reference comment above the constant so a future contributor reaches for the canonical term first.

## Delete Asset prompt understates the cascading effect across projects

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) (Treat External Input As Untrusted — make risky operations visible before they happen)
> - [`docs/guidelines/tui.md`](../../../docs/guidelines/tui.md) (put warnings before the irreversible action)

The Delete Project prompt explains its scope; the Delete Asset prompt does not. `Service.DeleteAsset` (`service.go:462`) deletes the asset folder under the profile root **and** calls `loaded.UnselectAsset(assetID)` which strips the id from every project's `SelectedAssetIDs`. Confirming the dialog quietly mutates every project in the profile.

```go
// edit_profile.go:515 — undersells the cascade
prompt := fmt.Sprintf("Are you sure you want to delete asset %q?", a.Name)
// Service.DeleteAsset also does:
//   - loaded.UnselectAsset(assetID) across every project
//   - asset.Delete(target.Dir) on disk
```

- [x] Extend the prompt: `"Delete asset %q? It will also be removed from every project's selection."`
- [ ] Inspect `s.prof.Projects` for usages of `a.ID` before opening the modal and surface the affected count in the prompt body.

## `pendingDelete*` fields are not cleared uniformly across modal lifecycles

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) (Keep File Access Inside Intended Roots — write only the files identified by the preview or apply plan)
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Common Closure Principle — modal sub-state)

`onDeleteAsset` sets `s.pendingDeleteAsset = a.ID`; `afterDeleteAsset` clears it. But `handleResolved` clears `s.modal` for **every** modal-id case, not just the matching delete case. If a future contributor opens a delete-asset modal, then somehow opens another modal that resolves first (today not reachable; tomorrow easy to regress when the modal sub-state grows), the stale `pendingDeleteAsset` is still live. Same shape for `pendingDeleteProj`. Two parallel pending-id fields plus an `s.modal.ID()` string dispatcher is the broader smell: modal sub-state is encoded across three places.

```go
// edit_profile.go:581 — clears modal but leaves stale pending* if a non-matching arm fires
func (s *editProfileScreen) handleResolved(msg modal.ResolvedMsg) tea.Cmd {
    s.modal = nil
    switch msg.ID {
    case "delete-asset":   return s.afterDeleteAsset(msg)   // clears pendingDeleteAsset
    case "create-asset":   return s.afterCreateAsset(msg)   // pendingDeleteAsset stays set
    ...
    }
}
```

- [x] Reset `pendingDeleteAsset` and `pendingDeleteProj` at the top of `handleResolved` before the switch.
- [ ] Replace both fields with a single tagged sub-state interface (`modalCtx interface{ resolve(*editProfileScreen, modal.ResolvedMsg) tea.Cmd }`) set by `openModal`; `handleResolved` becomes one line and the dispatch lives next to the open.

## `Update` switch is long and the modal-guard branch is repeated three times

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Functions #1 small/obvious, #2 one coherent job)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) (SRP)

`Update` handles six message families inline and the `if s.modal != nil { return s.forwardToModal(msg) }` check appears three times (once in `WindowSizeMsg`, once at the top of the `tea.KeyPressMsg` arm, once at the fall-through tail). Each branch carries layout, data-ingestion, focus, mnemonic, and table-routing logic.

```go
case tea.KeyPressMsg:
    if s.modal != nil { return s.forwardToModal(msg) }   // guard #1
    if handled, cmd := s.handler.Update(msg); handled {
        s.rebuildSet()
        return s, cmd
    }
    if btn := s.set.Match(m); btn != nil { return s, btn.Trigger() }
    return s, s.routeToFocusedTable(m)
}
if s.modal != nil { return s.forwardToModal(msg) }       // guard #2 — same call
```

- [x] Extract `handleKey(msg tea.KeyPressMsg) (Screen, tea.Cmd)` and `handleResize(msg tea.WindowSizeMsg) (Screen, tea.Cmd)`; collapse the three modal-guard branches into one entry-level check (with an allow-list for the message types that must still reach the screen).
- [ ] Lift only the modal-guard refactor (keep the switch shape) so the duplicate `if s.modal != nil` lives in one place.

## `bodyContent` mixes layout math, side-effect resize, and rendering behind a "Content" name

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Functions #2 one coherent job, #5 surprising side effects; Naming #4 magic numbers)

`bodyContent` (`edit_profile.go:305`) does four things: computes the table-height split, mutates both tables via `applyTableSize` (a side effect from a function named "content"), renders two bold headers, and assembles the join. The constant `fixedLines = 5` is explained only in a multi-paragraph doc comment ("assetsHeader (1) + table (N) + spacer (1) + projectsHeader (1) + spacer (1) + buttons (1)"). A reader has to reverse-engineer the arithmetic. The accompanying `cellPadding = 8` constant is redeclared in `assetsColumns` and `projectsColumns`, and `minNameColW` (in edit_profile.go) sits next to a `minPathColW` (in profiles.go) — a hidden cross-file dependency.

```go
func (s *editProfileScreen) bodyContent(width, height int) string {
    const fixedLines = 5         // magic; meaning explained only in doc comment
    tableTotal := height - fixedLines
    if tableTotal < 2 { tableTotal = 2 }
    assetsH := tableTotal / 2
    projectsH := tableTotal - assetsH
    s.applyTableSize(s.assetsTable, ...)  // side effect hidden inside *Content
    s.applyTableSize(s.projectsTable, ...)
    // ...
}
```

- [x] Split into `layoutTables(height) (assetsH, projectsH int)` (pure), `resizeTables(width, assetsH, projectsH)` (side-effect), and `renderBody()` (pure assembly). Promote `fixedLines` into named parts (`headersRows + spacersRows + buttonsRow`).
- [ ] Rename `bodyContent` to `renderAndResizeBody` so the side effect is in the name, and add a one-line `const tableChromeRows = 5 // 2 headers + 2 spacers + 1 button row` comment in place of the long doc block.

## Tests assert on internal modal-ID strings and private pending fields

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Assert Behavior, Not Mock Mechanics)

Five tests (`AssetsDKeyOpensDeleteAssetConfirm`, `ProjectsEKeyOpensEditProjectModal`, `ProjectsDKeyOpensDeleteProjectConfirm`, `CKeyOpensCreateAssetModal`, `RKeyOpensRegisterProjectModal`) assert against literal modal IDs (`"delete-asset"`, `"create-asset"`, …) and private fields (`s.pendingDeleteAsset`, `s.pendingDeleteProj`). Renaming `"delete-asset"` to `"asset-delete"` would break every test even though the user-visible behavior is unchanged. The delete-confirm tests already cover the same flow end-to-end via the service.

```go
// edit_profile_test.go:309
if got := s.modal.ID(); got != "delete-asset" { ... }   // internal token
if s.pendingDeleteAsset != "skill-1" { ... }            // private field
```

- [ ] Drop the opener tests where a confirm-path test already asserts the end-to-end flow. For the openers that have no confirm-path test (Create Asset, Register Project), render the modal and assert the body contains the expected heading or label.
- [x] Expose a `ModalKind` enum on the screen and assert against the typed value rather than the magic ID string.

## `labelsEqual` is order-sensitive but the contract is uniqueness

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Test One Behavior At A Time)

Tests like `AssetsFocusedMnemonicSet_PopulatedIsUnique` hard-code an exact sequence of seven labels (`"1","2","Edit","Delete","Create Asset","Register Project","Back"`). The test name says "Unique" — the rule under test is uniqueness — but the assertion locks in order plus content, two behaviors at once. Reordering the buttons (e.g. to match the rendered status-bar priority) breaks every such test even though the uniqueness invariant still holds.

```go
want := []string{"1", "2", "Edit", "Delete", "Create Asset", ...}
if !labelsEqual(got, want) { ... }   // order-sensitive; name says "Unique"
```

- [ ] Replace `labelsEqual` with a set-equality helper (assert membership, then assert no duplicate runes). Split the ordering assertion into its own test if a stable order is actually a user-visible contract.
- [x] Document the ordering contract on `mnemonic.Set.Buttons()` and rename the affected tests to `…HasExpectedMnemonicsInOrder` so the rule under test is unambiguous.

## Mutation-error branch is not exercised

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Test One Behavior At A Time)

`LoadErrorEmitsNotification` covers the load-error branch by corrupting the manifest on disk. There is no symmetric test for the mutation-error branch: `editProfileMutationDoneMsg` carries `severity` and the screen's `SeverityError` code path is untested. A regression that drops the notification or swallows the error would not be caught.

```go
// covered: SeverityInfo branch
if done.severity != errs.SeverityInfo { ... }
// missing: a test that triggers UpdateProject/DeleteAsset failure and
// asserts NotificationMsg{Severity: SeverityError}.
```

- [x] Add `TestEditProfileScreen_MutationErrorEmitsErrorNotification` that triggers a service failure (e.g. delete a profile path mid-flow, or pass a malformed manifest) and asserts the notification severity.
- [ ] Unit-test `editProfileMutationCmd` directly against a stub action returning a domain error; isolates the branch without registry I/O.

## `EditProjectModalPreservesNonEditableFields` fabricates its precondition

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Assert Behavior, Not Mock Mechanics; Separate Given, When, Then)

The test writes `manifest.SelectedAssetIDs = []string{"some-asset"}` on the seeded reference, then writes the same value **again** on the screen's loaded copy via `s.selectedProject()`, because the first write was lost on `Init`'s reload. The "preservation" being asserted is on a precondition the production code path can never produce — nothing in `AddProject` writes `SelectedAssetIDs`. If the merge ever silently drops the field, the test would still pass for the wrong reason.

```go
manifest := f.seedProject(t, "Proj", projectPath)
manifest.SelectedAssetIDs = []string{"some-asset"}   // (1) lost on reload
...
if proj, ok := s.selectedProject(); ok {
    proj.SelectedAssetIDs = []string{"some-asset"}   // (2) test re-writes by hand
}
```

- [x] Seed `SelectedAssetIDs` through a real domain entry point (e.g. a `Service.SelectAsset` call) so the precondition is produced by the same code that produces it in production.
- [ ] Replace this test with a pure unit test of `project.ApplyEdit(existing, in)` (or the merge function pulled out per "In-place mutation" finding above) — assert on a known input/output pair.

## `BodyExactlyMatchesRequestedHeight` misses the genuine edge cases

> [!WARNING]
>
> - [`docs/guidelines/testing.md`](../../../docs/guidelines/testing.md) (Test One Behavior At A Time)

The matrix covers `{24, 24, 40, 14}`. 14 is not actually small for the two-pane layout. The bug class the test exists to catch (height overshoot pushing the status bar off-screen) bites at heights below the panes' minimum render (`fixedLines + 2`, i.e. ~7). Three of the four cases are near-duplicates of the happy path.

```go
cases := []struct{ ... }{
    {"empty state", nil, nil, 120, 24},
    {"populated",   ..., 120, 24},
    {"tall window", nil, nil, 120, 40},
    {"short window", nil, nil, 120, 14},   // not actually short
}
// missing: height = 1, 2, 6, 7 — the genuine boundaries.
```

- [ ] Add `height = 1` and `height = 7` cases asserting the body still returns exactly the requested height (clamped behavior).
- [x] Split into two tests: one happy-path size, one `…ClampsAtMinimumHeight` for the boundary, so the intent of each is visible.

## Stub constructors and tests reach across the eventual real-screen boundary

> [!WARNING]
>
> - [`docs/guidelines/security.md`](../../../docs/guidelines/security.md) (Treat External Input As Untrusted)
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Stable Abstractions Principle)
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) (LSP)

Three smaller issues bundled because they live on the same boundary:

1. `newEditAssetStub`, `newSelectProjectAssetsStub`, `newPlanProjectStub` accept ids without the non-empty checks `newEditProfileScreen` panics on (`edit_profile.go:84-89`).
2. Tests reach into the stub's private `profileID` / `assetID` / `projectID` fields. When tasks 0027/0028/0029 land the real screens, those assertions either rewrite or force the real screens to keep the exact same unexported names.
3. Stub `Body(width, _ int)` discards `height`; `Update` ignores `WindowSizeMsg`. The `Screen` contract advertises both. Callers that assume screens self-size to the rectangle get different behavior depending on which Screen is on the stack.

```go
func newEditAssetStub(profileID, assetID string) *editAssetStub {
    return &editAssetStub{
        profileID: profileID,   // no non-empty check
        assetID:   assetID,
        ...
    }
}
// edit_profile_test.go:287 — asserts private fields directly
if stub.profileID != f.Profile.ID || stub.assetID != "skill-1" { ... }
// edit_asset_stub.go:57 — discards height
func (s *editAssetStub) Body(width, _ int) string { ... }
```

- [x] Address all three via the shared `placeholderScreen` proposed in "Three near-identical navigation stubs": one type with panic-guards on the ids, `ProfileID() / TargetID()` accessors for tests, and a real `WindowSizeMsg` + `height`-honoring `Body`.
- [ ] Apply only the panic guards now and rely on tasks 0027/0028/0029 to fix the LSP gap when they swap in real screens.

## Hard dependency on `*actions.Actions` instead of a screen-local interface

> [!WARNING]
>
> - [`docs/guidelines/solid.md`](../../../docs/guidelines/solid.md) (DIP, ISP)
> - [`docs/guidelines/clean_architecture.md`](../../../docs/guidelines/clean_architecture.md) (Stable Dependencies Principle)

`editProfileScreen` takes a concrete `*actions.Actions` and calls six methods on it. The TUI/app boundary is the documented inversion point (per ADR 0011 and tui.md). A screen-local interface listing only those six methods would let the screen be tested without standing up the whole `Actions` graph and would surface — at compile time — when the screen quietly starts using a seventh method. `profilesScreen` carries the same shape, so this is a screen-base pattern decision, not a one-off.

```go
type editProfileScreen struct {
    actions   *actions.Actions   // full surface; only 6 methods used
}
```

- [x] Declare `editProfileActions interface { LoadProfile(...) ...; CreateAsset(...) ...; DeleteAsset(...) ...; AddProject(...) ...; UpdateProject(...) ...; DeleteProject(...) ... }` in shell and accept it in `newEditProfileScreen`. `*actions.Actions` satisfies it implicitly.
- [ ] Defer until a pattern emerges across multiple screens: keep the concrete dependency today, plan a single shell-wide `Service` interface in a follow-up task once two more screens land.

## `AddMnemonic`'s nil-return contract is not honored

> [!WARNING]
>
> - [`docs/guidelines/go.md`](../../../docs/guidelines/go.md) (Return Actionable Errors)
> - [`docs/guidelines/errors.md`](../../../docs/guidelines/errors.md)

`focus.Handler.AddMnemonic` documents that it returns `nil` when the digit is invalid or already bound (it logs a warning instead of erroring). The constructor stores the returned `*mnemonic.Button` into `s.assetsPanel` / `s.projectsPanel` and then dereferences both unconditionally in `rebuildSet`, `View()`, and `Binding()`. With the current literals `'1'` and `'2'` this is dead code, but a refactor to a dynamic rune would turn a domain error into a nil-pointer panic in `rebuildSet`.

```go
s.assetsPanel = s.handler.AddMnemonic(s.assetsTable, '1') // can return nil per docs
s.projectsPanel = s.handler.AddMnemonic(s.projectsTable, '2')
s.rebuildSet()  // dereferences both unconditionally
```

- [x] Panic in `newEditProfileScreen` when either returned button is nil, matching the existing `nil actions` / empty `profileID` precondition pattern.
- [ ] Change `AddMnemonic` to return `(*mnemonic.Button, errs.DomainError)` and propagate a typed `MnemonicBindError` per `errors.md`. Out of scope for this task; tracked separately if chosen.

## Naming inconsistencies on the screen struct

> [!WARNING]
>
> - [`docs/guidelines/clean_code.md`](../../../docs/guidelines/clean_code.md) (Naming #2 meaningful distinctions; Understandability #2 consistency)

A small cluster of naming drift on the `editProfileScreen` fields:

- `pendingDeleteAsset` (spelled out) vs `pendingDeleteProj` (abbreviated).
- `assetsPanel` / `projectsPanel` are typed `*mnemonic.Button`, not panels — the name lies about the type.
- `assetsList`, `projectsList` use Go-discouraged `List` suffixes when `assets`, `projects` would do.

```go
type editProfileScreen struct {
    ...
    assetsPanel   *mnemonic.Button // [1] — named "Panel", is a Button
    projectsPanel *mnemonic.Button // [2]
    ...
    pendingDeleteAsset string       // full word
    pendingDeleteProj  string       // abbreviated
    ...
}
```

- [x] Rename to `pendingDeleteAssetID` / `pendingDeleteProjectID`; rename `assetsPanel` / `projectsPanel` to `assetsFocusButton` / `projectsFocusButton`; drop `List` suffix on slices.
- [ ] Apply only the `pendingDelete*` consistency fix; leave the slice and panel names since the cost-of-change is low and the bigger refactors (modal sub-state, AssetList) above will touch those anyway.
