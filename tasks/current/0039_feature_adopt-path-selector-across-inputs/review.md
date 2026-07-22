# Adopt path selector across all path inputs — review

The two-step pathselector rollout meets every acceptance criterion, all
required sections are present, and the code compiles as a coherent
extension of task 0037. The DoD gate passed: each AC maps to concrete
diff evidence and every hunk resolves against the plan.

The findings below cluster around three themes:

1. **The "read-only" contract is cosmetic-only** — `huh` v2 has no
   runtime read-only mode, no validator was added, and there is no
   regression test that would catch a `readOnlyPathInput → pathInput`
   swap. Combined with the removed `TestRegisterProfile_RejectsEmpty…`
   test, an empty or user-edited path can reach the action layer.
2. **Retry-loop scaffolding is duplicated three times** — three
   near-identical `*FailedMsg` structs, three copies of the picker-open
   helper, and a hidden "clear-on-next-open" invariant that lives across
   four call sites. The changelog justifies distinct *types*, not
   identical *shapes*.
3. **Inconsistency between sibling screens** — `profiles.go` dispatches
   by raw modal-id string, `edit_profile.go` dispatches by typed
   `modalKind`; three canonical docs (arch, glossary, `CLAUDE.md`)
   still show the old `NewSelectPath(opts)` signature.

Read every issue below and tick exactly one checkbox per issue for the
solution you want applied, then run `af.task.review-apply 0039` in a
fresh session.

## Read-only path field is cosmetic — value is still editable, validator dropped

> [!WARNING]
> - [Security](docs/guidelines/security.md) — "Treat External Input As Untrusted", "Keep File Access Inside Intended Roots"
> - [SOLID](docs/guidelines/solid.md) — LSP: `readOnlyPathInput` and `pathInput` share `*huh.Input` return type but not contract

`readOnlyPathInput` builds an ordinary `huh.NewInput()` bound to
`&state.Path`, differing from `pathInput` only by appending
`" (read-only)"` to the description and dropping the
`Validate(requiredString)` gate. `huh` v2 has no runtime read-only mode
— the `textinput` widget still receives every keypress that reaches
focus. The changelog itself acknowledges this ("the marker is the only
user-visible signal"), yet three flows (Create Profile / Register
Profile / Register Project) now rely on the two-step contract to keep
the pathselector's safety gates load-bearing.

A user who Tabs onto the Path field can overwrite the seeded value with
any string — including a path they were never able to reach through the
pathselector, or an empty string. The submitted value flows into
`Service.CreateProfile` / `Service.RegisterProfile` / `Service.AddProject`
where `utils.ToAbsolute(path)` handles the empty case but downstream
error shapes differ per flow. The safety invariants the picker enforces
(constraint containment, symlink resolution via `utils.ResolveAbs`)
apply only when the read-only contract holds.

`TestRegisterProfile_RejectsEmptyRequiredField` was removed because
"Register Profile form now has zero editable fields". That is the
symptom, not the fix — the deletion removes the only guard that would
have caught a `readOnlyPathInput → pathInput` regression.

```go
// internal/tui/modals/fields.go
func readOnlyPathInput(value *string, description string) *huh.Input {
    return huh.NewInput().Key("path").Title("Path").
        Description(description + " (read-only)"). // cosmetic
        Value(value)                               // bound pointer, editable
    // no .Validate — empty or edited strings reach the action layer
}
```

- [ ] Enforce read-only via a custom `huh.Accessor` whose `Set` is a no-op, then add a test that types characters into the field and asserts `state.Path` is unchanged.
- [ ] Replace the field with `huh.NewNote` (or a caption line rendered above the group) so no `textinput` is focusable at all — for Register Profile this collapses the form to zero fields, which is the underlying design pressure.
- [ ] Keep the widget but re-add `.Validate(requiredString)` plus a validator that rejects any value diverging from the seeded path, and re-instate `TestRegisterProfile_RejectsEmptyRequiredField` under a new name.

## `filepath.Dir("")` retry seed lands the picker at CWD, not $HOME

> [!WARNING]
> - [Security](docs/guidelines/security.md) — "Keep File Access Inside Intended Roots"
> - [Go guidelines](docs/guidelines/go.md) — edge cases at boundaries

All three retry handlers compute the next picker root as
`filepath.Dir(m.path)`. `filepath.Dir("")` returns `"."`, which
`pathselector.probeStart` resolves against the process CWD — the folder
`./bin/af` was launched from. This is not the intended `$HOME` fallback
that empty `StartFolder` triggers inside `resolveStart`.

Every current in-package producer of the `*FailedMsg` types fills
`path` from a just-submitted form so an empty string is unlikely today.
Combined with the previous finding (users can now submit `""` via the
"read-only" field), the reachable state is: user edits Path to empty,
domain rejects it, notification fires, picker re-opens rooted at CWD
without warning.

```go
case createProfileFailedMsg:
    return s, tea.Batch(
        notificationCmd(m.severity, m.text),
        s.openCreateProfilePathselectorCmd(filepath.Dir(m.path)),
        // if m.path == "" → filepath.Dir("") == "." → probeStart(".") → CWD
    )
```

- [ ] Guard the retry seed: `parent := filepath.Dir(m.path); if !filepath.IsAbs(parent) { parent = "" }`, then pass `parent` to the picker so `resolveStart` falls back to `$HOME`.
- [ ] Enforce non-empty `path` on the failed-msg struct at construction (small constructor helper) so an empty-path retry becomes impossible upstream.

## Three near-identical `*FailedMsg` structs + three near-identical helpers

> [!WARNING]
> - [Clean Code](docs/guidelines/clean_code.md) — "Needless repetition: duplicated rules that can drift apart"
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — CCP
> - [Domain Model](docs/guidelines/domain_model.md) — one concept, one representation

`createProfileFailedMsg`, `registerProfileFailedMsg`, and
`registerProjectFailedMsg` are structurally identical
(`{path string, text string, severity errs.Severity}`) and semantically
identical (retry seed + notification). Alongside them, three
`openXxxPathselectorCmd` helpers repeat the same body verbatim modulo id
and caption; three `afterXxxPath` handlers repeat the same
`ResultFromMsg` extraction and follow-on-form seed. The changelog
justifies distinct *types* ("adding a `path` field to `mutationDoneMsg`
would leak retry concerns into every other screen"), which is a fair
argument — but the argument only supports having distinct messages, not
three copies of the same shape.

The drift risk is concrete: adding a shared field (say `retryHint
string`) requires editing three struct declarations, three `Update`
cases, and three `openXxx…Cmd` helpers. A `filepath.Dir` swap must
happen in three places. The `edit_profile.Update` modal-guard
fall-through at line 243 must be extended for every new failed-msg
type.

The DDD-level concern: the envelope pre-flattens
`err.Severity()`/`err.Error()`, dropping the typed
`errs.DomainError` that `ProjectPathOwnedError` /
`UnsafeProfilePathError` carry.

```go
// profiles.go
type createProfileFailedMsg struct { path, text string; severity errs.Severity }
type registerProfileFailedMsg struct { path, text string; severity errs.Severity }
// edit_profile.go
type registerProjectFailedMsg struct { path, text string; severity errs.Severity }
```

- [ ] Consolidate into a single `pathPickerFailedMsg{ flow flowKind; path string; err errs.DomainError }` and dispatch on `flow` in each screen's `Update`. Retains distinct flow identity, keeps the retry rule in one place, preserves the typed `DomainError`.
- [ ] Extract a shared embedded struct (`pathRetryFailure struct { path string; text string; severity errs.Severity }`) that the three envelopes embed, so a field addition edits one place.
- [ ] Keep the three envelopes but move their construction into a per-screen helper (`s.failedMsg(path, err)`) so future edits touch one call site per screen.

## `handleResolved` dispatches by id in `profiles.go` but by kind in `edit_profile.go`

> [!WARNING]
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — CCP
> - [Clean Code](docs/guidelines/clean_code.md) — consistency across similar concepts
> - [Go guidelines](docs/guidelines/go.md) — avoid magic strings

`edit_profile.go` dispatches modal resolutions via a typed `modalKind`
enum (and this task even added `modalKindRegisterProjectPath`).
`profiles.go` dispatches the same shape of resolutions via raw id
strings inside `switch msg.ID`. Two sibling screens in the same package
now solve the same problem two different ways. The id literals
(`"create-profile-path"`, `"register-profile-path"`,
`"register-project-path"`) also appear as raw strings at every
constructor site AND every test assertion — a typo at one location
silently drops the resolution branch.

`edit_profile_test.go:534` writes `s.modalKind =
modalKindRegisterProjectPath` by hand to simulate a mount, exposing the
awkward duality: the modal produced by
`NewSelectPath("register-project-path", …)` carries the id string,
but the screen dispatches by the kind — two identifiers describing the
same modal.

```go
// profiles.go
switch msg.ID {
case "create-profile-path":
    return s.afterCreatePath(msg)
// edit_profile.go
switch kind {
case modalKindRegisterProjectPath:
    return s.afterRegisterProjectPath(msg)
```

- [ ] Adopt the `modalKind` pattern on `profilesScreen` too — add a `modalKind` field, dispatch through it, and let the id string become an internal detail of the modal wrapper.
- [ ] Introduce package-level constants (`modalIDCreateProfilePath = "create-profile-path"`, …) so both production sites and tests reference the same symbol. Keep the two dispatch styles but eliminate the raw literal.
- [ ] Add a compile-time test that iterates every emitted id and checks each has a matching case in `handleResolved`.

## Retry `StartFolder` seeding is not tested

> [!WARNING]
> - [Testing](docs/guidelines/testing.md) — "Test behavior, not implementation details"

The load-bearing contract of the retry loop is *"pathselector re-opens
seeded at `filepath.Dir(previousPath)`"*. The tests
`TestProfilesScreen_CreateProfileFailureReopensPathselector`,
`TestProfilesScreen_RegisterProfileFailureReopensPathselector`,
`TestEditProfileScreen_RegisterProjectFailureReopensPathselector`, and
the two `…RetryPreservesName/Inputs` variants all assert only that a
new modal opens with the right id — none verifies that its
`pathselector.Options.StartFolder` equals the parent of the failed
path. The plan explicitly waves this away ("options `StartFolder ==
"/tmp"` cannot be introspected from outside; skip that assertion");
that's a *design* limitation worth solving, not an assertion to skip.
A regression swapping `filepath.Dir(m.path)` for `""` (defaulting to
`$HOME`) would ship silently.

```go
// profiles.go — behavior under test
case createProfileFailedMsg:
    return s, tea.Batch(
        notificationCmd(m.severity, m.text),
        s.openCreateProfilePathselectorCmd(filepath.Dir(m.path)),
    )
// profiles_test.go — only the id is asserted
if got := s.modal.ID(); got != "create-profile-path" {
    t.Errorf("modal id = %q, want create-profile-path", got)
}
```

- [ ] Split each `openXxxPathselectorCmd` into `buildXxxPathselectorOptions(startFolder) pathselector.Options` + a thin mounting caller. Tests call the build helper directly and assert `opts.StartFolder`.
- [ ] Expose a test helper `pathselector.LastOptions(*modal.Modal) (Options, ok)` (or an accessor on the notificationTranslator) so tests can introspect the mounted picker.
- [ ] Accept the coverage gap and document it as an intentional limitation in a comment above each retry case.

## Doc drift — three canonical docs still show `NewSelectPath(opts)`

> [!WARNING]
> - [Documentation](docs/guidelines/documentation.md) — "document current reality, not planned state"
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — arc42 as authoritative source

Three source-of-truth documents still describe the pre-task signature:

- `docs/architecture/05-building-block-view.md:264` — "opened by callers through the `modals.NewSelectPath(opts)` wrapper". The paragraph was edited by this PR (the trailing note replaced), but the signature was missed.
- `docs/glossary.md:401-402` — same outdated reference.
- `CLAUDE.md:41` — "reusable file/folder picker modal opened via `modals.NewSelectPath(opts)`."

The changelog for this task documents the widening from `(opts)` to
`(id, opts)`. Leaving the three canonical docs showing the outdated
form contradicts the "current reality" rule and misleads any reader —
human or agent — who consults them.

- [ ] Update all three references to `modals.NewSelectPath(id, opts)` and add a one-sentence note on the `id` parameter's dispatch role.
- [ ] Revert the docs *only* if the "id parameter is caller-supplied magic" issue below is adopted and the widening is rolled back.

## `NewSelectPath(id, opts)` — caller-supplied dispatch magic pushed into the modals wrapper

> [!WARNING]
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — Stable Dependencies Principle
> - [SOLID](docs/guidelines/solid.md) — DIP: shell orchestration leaked into a reusable modal factory
> - [Security](docs/guidelines/security.md) — unvalidated caller-controlled routing key

Every other modal factory in `internal/tui/modals` supplies its own id
(`"create-profile"`, `"register-profile"`, `"create-asset"`, …).
`NewSelectPath` is now the only one where the caller must know the
downstream dispatch convention. There is no validation, deduplication,
or namespacing on the id — two callers could collide, an empty
whitespace string is accepted, and the compiler links
`NewSelectPath("create-profile-path", …)` to
`case "create-profile-path":` via string identity 60 lines away. The
plan describes the widening as "purely additive to consumers", but it
inverts the layering: the more-volatile shell concern (which case the
switch reads) reaches into the more-stable modal wrapper's signature.

```go
// select_path.go — id is passed through without any validation
func NewSelectPath(id string, opts pathselector.Options) (*modal.Modal, errs.DomainError) {
    // ...
    return modal.New(id, &notificationTranslator{inner: content}, modal.WithCaption(caption)), nil
}
```

- [ ] Roll back the widening: keep `NewSelectPath(opts)` returning a fixed id (or a per-instance UUID), have the shell dispatch by `modalKind` enum only. Combines well with the "consistent dispatch" issue above.
- [ ] Keep the signature but validate the id at the wrapper (reject empty/whitespace, log or panic on collision within an active session) and introduce a typed `type SelectPathID string` with named constants so the id becomes compile-checkable.
- [ ] Return a small opaque handle from `NewSelectPath` that carries a `Match(msg modal.ResolvedMsg) bool` method; the raw id never surfaces in caller code.

## Pending screen-state — cross-flow leakage and hidden ordering

> [!WARNING]
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — boundary between policy and detail
> - [Clean Code](docs/guidelines/clean_code.md) — "Avoid hidden ordering requirements between methods or package-level state"
> - [Security](docs/guidelines/security.md) — "Protect Secrets And Local State"

`pendingCreateName`, `pendingRegisterProjectName`, and
`pendingRegisterProjectAgents` live as bare fields on the screen
structs. The "clear before opening a new flow" invariant is spread
across four call sites per flow (init handler, cancel branch of
pathselector-step, cancel branch of form-step, and the failure path).
`afterCreate` (profiles.go:466) stashes `s.pendingCreateName = in.Name`
*before* returning the async cmd, so on success the stash is silently
stale and only cleared by the *next* `c` press. On the Register Project
path, `afterRegisterProject` (edit_profile.go:797-798) stashes name +
agents unconditionally on submit, again cleared only by the next `r`.

`handleResolved` in `edit_profile.go` snapshots and clears
`pendingDelete*` up-front, but does not do the same for
`pendingRegisterProject*` (they must survive into the follow-on form
seed). That is intentional but fragile: a future refactor adding a new
`modalKindXxx` that reuses the same fields would silently break if the
maintainer mechanically extended the up-front clear.

Miss any single clear and the *next* flow silently seeds a stale
value. The changelog labels the design as "avoids mutating screen
state from inside a `tea.Cmd` closure" — the mutation still happens
synchronously on the Update goroutine, so that argument holds, but the
lifecycle is undocumented.

```go
type profilesScreen struct {
    // ...
    pendingCreateName string   // cleared in: onCreate, afterCreatePath cancel, afterCreate cancel
}

func (s *profilesScreen) afterCreate(msg modal.ResolvedMsg) tea.Cmd {
    // ...
    s.pendingCreateName = in.Name   // stashed unconditionally, cleared on next c
    // ...
}
```

- [ ] Introduce a `pendingCreateProfileFlow` / `pendingRegisterProjectFlow` value type on each screen with `Begin() / Stash(T) / Take() T / Clear()` methods — the transitions become named operations and the type enforces "clear together". Replace the pending fields with `pendingRegisterProjectDraft *project.Manifest` for the Register Project flow so the domain type already available via `project.NewDraft` carries the in-flight state.
- [ ] Stash only inside the `if err != nil` arm of the tea.Cmd closure (via a stash-msg sent alongside the failed msg), and clear on `mutationDoneMsg`. Removes the "stale after success" state entirely.
- [ ] Extract a single `resetPendingCreateProfile()` / `resetPendingRegisterProject()` helper called from every terminal branch (belt-and-braces) plus a code comment above each field spelling out the lifecycle.

## Read-only marker `" (read-only)"` is a stringly-typed contract with four literal sites

> [!WARNING]
> - [Clean Code](docs/guidelines/clean_code.md) — "Replace magic strings with named constants"
> - [Domain Model](docs/guidelines/domain_model.md) — UI copy vs domain fact
> - [Testing](docs/guidelines/testing.md) — weak substring assertion

The literal `"read-only"` appears in `fields.go` (`Description(description
+ " (read-only)")`) and in three test files, each asserting
`strings.Contains(view, "read-only")`. Any i18n pass, copy edit
("(display-only)", "(read only)", "(cannot edit)"), or accidental
removal must be coordinated across four locations. The substring match
also does not tie the marker to the Path field — the field could
disappear entirely and, if any other rendered text mentions "read-only",
the tests still pass.

```go
// fields.go
Description(description + " (read-only)").
// three test files, identical pattern
if view := form.View(); !strings.Contains(view, "read-only") {
    t.Errorf(...)
}
```

- [ ] Extract `const readOnlyMarker = " (read-only)"` in `fields.go`; both the helper and the tests reference it. Also tighten the assertion to check the Path field's `Description()` directly via a form-field lookup helper (mirror `create_asset_test.go`).
- [ ] Model the "read-only" trait as data on a wrapper (`type readOnlyInput struct{ *huh.Input }`) so the assertion checks a type, not a string. Combines well with the LSP fix in the first issue.

## Test naming inconsistency: `TestBuild<Type>_...` vs sibling `Test<Type>_...`

> [!WARNING]
> - [Testing](docs/guidelines/testing.md) — "Test behavior, not implementation details"
> - [Clean Code](docs/guidelines/clean_code.md) — consistency across similar tests

Every other modal test in `internal/tui/modals/` follows the
`Test<Type>_<Behavior>` convention (e.g. `TestCreateAsset_PrefillSeedsState`,
`TestCreateProfile_PumpResolvesWithTypedInput`,
`TestRegisterProject_RejectsEmptyRequiredFields`). The three new tests
break the pattern:
`TestBuildCreateProfile_PathReadOnly`,
`TestBuildRegisterProfile_PathReadOnly`,
`TestBuildRegisterProject_PathReadOnly`. The `Build` prefix leaks the
private helper name (`buildCreateProfile`) into the test identifier.

```go
// established convention
func TestCreateProfile_PrefillSeedsState(t *testing.T) { ... }
// new tests
func TestBuildCreateProfile_PathReadOnly(t *testing.T) { ... }
```

- [ ] Rename to `TestCreateProfile_PathIsReadOnly`, `TestRegisterProfile_PathIsReadOnly`, `TestRegisterProject_PathIsReadOnly` for consistency with the sibling suite.

## Tests assert implementation details — `pendingCreateName`, `modalKind` writes

> [!WARNING]
> - [Testing](docs/guidelines/testing.md) — "Test behavior, not implementation details"

Several new tests reach into unexported struct fields to prove
behavior:

- `TestProfilesScreen_CreateProfileFailureRetryPreservesName` asserts
  `s.pendingCreateName == "SecondTry"`. The user-visible behavior is
  *"the Name field on the re-opened form is pre-filled"* — the test
  proves the private stash was written, not that the value reaches the
  form.
- `TestEditProfileScreen_RKeyOpensRegisterProjectPathselector` asserts
  BOTH `s.modalKind == modalKindRegisterProjectPath` and
  `s.modal.ID() == "register-project-path"` — redundant, and binds the
  test to one of two currently-supported dispatch mechanisms.
- `TestEditProfileScreen_RegisterProjectPathselectorCancelClearsModal`
  writes `s.modalKind` and pending fields by hand to simulate state
  that only production code should set.

```go
// profiles_test.go — asserts the private stash
if s.pendingCreateName != "SecondTry" {
    t.Errorf("pendingCreateName = %q, want SecondTry", s.pendingCreateName)
}
```

- [ ] After the retry ResolvedMsg, extract the form-state pointer from the freshly-mounted modal (mirror `buildCreateProfile`'s returned state pointer) and assert `state.Name == "SecondTry"`. Same shape for the Register Project variant.
- [ ] Drop the `modalKind` assertion where the modal id already proves intent — pick one channel and stick with it.
- [ ] Refactor cancel-clearing tests to drive state through production paths (dispatch a Confirmed pathselector ResolvedMsg, then a Confirmed=false form ResolvedMsg) instead of setting `s.modalKind` and `s.pendingRegisterProjectName` directly.

## Missing coverage: cancel-after-failure

> [!WARNING]
> - [Testing](docs/guidelines/testing.md)

Tests cover: (a) initial cancel clears state, (b) confirm→submit
succeeds, (c) confirm→submit→fail re-opens the pathselector, and (d)
confirm→submit→fail→confirm re-seeds the form with the stashed inputs.
The path *not* covered is confirm→submit→fail→**cancel**. When the
pathselector re-opens after a failure and the user hits `esc`,
`afterCreatePath` / `afterRegisterProjectPath` runs its `!ok` branch
and clears the stash — so a subsequent `c` / `r` press must start
empty. That "stash cleanup" invariant keeps a rejected `SecondTry`
name from bleeding into an unrelated future flow.

```go
// afterCreatePath — the untested branch
func (s *profilesScreen) afterCreatePath(msg modal.ResolvedMsg) tea.Cmd {
    result, ok := pathselector.ResultFromMsg(msg)
    if !ok {
        s.pendingCreateName = "" // <-- fires after a failure? untested
        return nil
    }
    ...
}
```

- [ ] Add `TestProfilesScreen_CreateProfileFailureThenCancelClearsStashedName`: dispatch `createProfileFailedMsg`, then dispatch a Cancelled ResolvedMsg on `create-profile-path`, assert `s.pendingCreateName == ""` and `s.modal == nil`. Same for the Register Project variant asserting both `pendingRegisterProjectName` and `pendingRegisterProjectAgents` are cleared.

## Overlapping tests: `_PathReadOnly` duplicates `_PrefillSeedsState`

> [!WARNING]
> - [Testing](docs/guidelines/testing.md) — one behavior per test, avoid duplicated assertions

`TestBuildRegisterProject_PathReadOnly` asserts `state.Path ==
"/repos/seed"`; `TestRegisterProject_PrefillSeedsState` already asserts
the same thing plus Name and EnabledAgents. Same duplication in the
Create Profile and Register Profile suites.

- [ ] Drop the `state.Path` assertion from the three `_PathReadOnly` tests; keep only the marker assertion (tighten it per the "stringly-typed marker" issue above).
- [ ] Consolidate: extend `_PrefillSeedsState` with the marker assertion and delete `_PathReadOnly` entirely.

## Register Profile form has zero editable fields — `huh.Form` is overkill

> [!WARNING]
> - [SOLID](docs/guidelines/solid.md) — OCP: "extending a bad abstraction usually makes the design harder to understand"
> - [Clean Code](docs/guidelines/clean_code.md) — "the simplest design that solves the current problem well"

`buildRegisterProfile` returns a `huh.Form` whose only group contains a
single `readOnlyPathInput`. Nothing is user-editable — the changelog
even notes that `TestRegisterProfile_RejectsEmptyRequiredField` was
removed because there are "zero editable fields". The abstraction
(`huh.Form` as an input container) is carried for a case that has no
input.

```go
func buildRegisterProfile(initial RegisterProfileInput) (*huh.Form, *RegisterProfileInput, func(*huh.Form) any) {
    state := &RegisterProfileInput{Path: initial.Path}
    form := huh.NewForm(
        huh.NewGroup(
            readOnlyPathInput(&state.Path, "Profile directory picked in the previous step"),
        ),
    ).WithTheme(styles.HuhTheme())
    return form, state, func(*huh.Form) any { return *state }
}
```

- [ ] Skip the form entirely: on `afterRegisterPath`, invoke `s.actions.RegisterProfile(actions.RegisterProfileInput{Path: result.Path})` directly, so the flow becomes single-step. Optionally show a `modal.NewConfirm("Register profile at <path>?", …)` first for a visible confirmation gate.
- [ ] Keep the form but note in a comment that it exists purely to preserve the resolved-msg envelope shape the shell expects.

## Cross-package coupling — shell knows `pathselector.Options` directly

> [!WARNING]
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — layering; `internal/tui/modals` is the boundary that hides modal internals
> - [Domain Model](docs/guidelines/domain_model.md) — bounded-context leak

`internal/tui/shell/profiles.go` and `edit_profile.go` now import
`internal/tui/modals/pathselector` directly to reach
`pathselector.Options`, `pathselector.Result`, and
`pathselector.ResultFromMsg`. The three call sites duplicate the
`{Caption, ShowFiles: false, StartFolder}` triple, and each
independently decides that `ShowFiles: false` is the right default for
"pick a folder". Every other modal factory in `modals` returns a
narrow input/output pair; only the pathselector escapes this seam.

```go
// three near-identical Options literals in shell code
pathselector.Options{Caption: "Select profile folder",          ShowFiles: false, StartFolder: startFolder}
pathselector.Options{Caption: "Select existing profile folder", ShowFiles: false, StartFolder: startFolder}
pathselector.Options{Caption: "Select project root",            ShowFiles: false, StartFolder: startFolder}
```

- [ ] Add three flow-specific façades in `modals` (`NewSelectProfileFolder(id, startFolder)`, `NewSelectExistingProfileFolder(id, startFolder)`, `NewSelectProjectRoot(id, startFolder)`) that hard-code `ShowFiles: false` and their captions. Shell imports only `modals`.
- [ ] Or expose a single `NewSelectFolder(id, caption, startFolder)` folder-picker wrapper that hides `ShowFiles: false`; shell imports only `modals`.
- [ ] Have `modals` re-export narrow type aliases (`type SelectPathResult = pathselector.Result`, `func SelectPathResultFromMsg = pathselector.ResultFromMsg`) so shell keeps importing only `modals`.

## Removed `TestRegisterProfile_RejectsEmptyRequiredField` without a positive replacement

> [!WARNING]
> - [Testing](docs/guidelines/testing.md) — "Add or update tests when changing … error handling"

The deletion is defensible today (there is no required field to
reject), but the *invariant* the test was defending — Register Profile
does not accept an empty path — is now unowned. Any future change that
re-adds an editable field and forgets to mark it required would ship
without complaint. The sibling `TestRegisterProject_RejectsEmptyRequiredFields`
is retained, so the file still carries the pattern.

- [ ] Replace with a positive assertion: `TestRegisterProfile_SubmitWithPrefilledPathResolvesImmediately` — build with `{Path: "/x"}`, submit, assert Confirmed and `Value.Path == "/x"` without an `expectFormStuck` pre-check.
- [ ] Accept the deletion and add a note in the changelog's "Other Notes" section that adding any editable field to Register Profile requires re-adding a required-field test.

## Symlink asymmetry — picker refuses to follow but action layer never resolves

> [!WARNING]
> - [Security](docs/guidelines/security.md) — "Keep File Access Inside Intended Roots"

`Options.FollowSymlinks` defaults to `false` in all three call sites
(the field is left unset). Inside the picker that is the safe choice:
pressing Enter on a symlink is a silent no-op, and constraint checks
use `filepath.EvalSymlinks`. But downstream, `Service.CreateProfile` /
`Service.RegisterProfile` / `Service.AddProject` normalize the path via
`utils.ToAbsolute`, which is `filepath.Abs` only — no symlink
resolution.

Combined with the read-only-bypass finding above, an attacker who
plants `~/repos/harmless -> /etc` and coaxes the user into typing
`~/repos/harmless` into the "read-only" field will have the tool
write into `/etc`, because the resolution the picker enforces only
runs when the picker is actually used.

```go
// Service.CreateProfile
func (s *Service) CreateProfile(name, path string) (*registry.ProfileRef, errs.DomainError) {
    path, absErr := utils.ToAbsolute(path)   // filepath.Abs only
    // ...
    manifest, initErr := profile.Init(path, name)  // writes into `path`
}
```

- [ ] Have `Service.CreateProfile` / `Service.RegisterProfile` / `Service.AddProject` symlink-resolve the incoming path via `utils.ResolveAbs(path, true)` before use — belongs to the action layer's boundary duties regardless of what the TUI does.
- [ ] Set `FollowSymlinks: true` on all three pathselector call sites so the picker resolves and enforces containment consistently.

## Unconstrained browsing + error strings leak absolute paths into notifications

> [!WARNING]
> - [Security](docs/guidelines/security.md) — "Protect Secrets And Local State"

All three call sites pass `pathselector.Options` with no
`ConstraintRoot`, so the picker lets the user browse the entire
filesystem. That was the deliberate design ("dirs only, no constraint,
start folder `$HOME`"), but two secondary effects:

1. `translateMsg` in `select_path.go:107-111` formats
   `ReadDirErrorMsg` as `"cannot read directory %q: %s"` — which
   splats absolute paths like `/home/other-user/.ssh` into the
   notification bar (and history).
2. The three `*FailedMsg` structs carry `text: err.Error()` produced by
   domain-layer errors that routinely include the offending absolute
   path.

Not a fresh introduction (task 0037 owns the picker), but the rollout
into three previously-typed flows increases the exposure surface.

- [ ] Constrain Create Profile / Register Profile to `$HOME` (profile folders almost never live above `$HOME`); Register Project can remain unconstrained.
- [ ] Redact or truncate absolute paths in `ReadDirErrorMsg` text, especially since the user did not select the path — merely browsed to it.
- [ ] Accept and document: the flows are single-user local tooling and unconstrained browse is a UX requirement.

## `notificationTranslator.Update` discards returned `modal.Content`

> [!WARNING]
> - [Clean Architecture](docs/guidelines/clean_architecture.md) — abstraction stability

`select_path.go:62-65` calls `t.inner.Update(msg)` and discards the
returned `modal.Content`, returning `t` (self) instead. That works today
because `pathselector.Content.Update` always returns itself as a
value receiver — but the `modal.Content` contract permits returning a
new value. A future change to `pathselector.Content.Update` (or any
generalization) would strand the caller on a stale inner. The widened
wrapper now serves three flows instead of one, amplifying the
fragility.

```go
func (t *notificationTranslator) Update(msg tea.Msg) (modal.Content, tea.Cmd) {
    _, cmd := t.inner.Update(msg)  // returned Content ignored
    return t, wrapCmd(cmd)
}
```

- [ ] Type-assert the returned Content back to `*pathselector.Content` and reassign `t.inner` so a hypothetical value swap is handled.
- [ ] Document on `pathselector.Content.Update` that it always returns its own receiver, making the discard intentional.

## `openModal` + `Init()` two-step protocol is a footgun

> [!WARNING]
> - [Go guidelines](docs/guidelines/go.md) — cohesive functions; avoid two-step protocols where one call suffices

`openModal(m, kind)` mutates state; every caller must then remember to
`return s.modal.Init()`. Six call sites do the dance today
(edit_profile.go:592, 605, 635, 640, 668, 726; profiles.go:341, 375,
388, 438, 449, 505). A caller who forgets `Init()` opens the modal
without an initial cursor blink / focus command — subtle bug. Not
introduced by this task, but the task adds three more call sites so
the growing surface warrants attention.

```go
func (s *editProfileScreen) openModal(m *modal.Modal, kind modalKind) {
    s.modal = m
    s.modalKind = kind
    if s.width > 0 && s.height > 0 {
        mw, mh := modalSize(s.width, s.height)
        s.modal.SetSize(mw, mh)
    }
}
```

- [ ] Change `openModal` to return `tea.Cmd`, calling `m.Init()` internally. Call sites become `return s.openModal(m, modalKindX)`.
- [ ] Accept the pre-existing split; note this in `CLAUDE.md` under TUI conventions so future contributors do not miss it.

## Modal caption is double-sourced

> [!WARNING]
> - [Go guidelines](docs/guidelines/go.md) — one source of truth per concern

`NewSelectPath` reads `opts.Caption` (a `pathselector.Options` field),
then hoists it into `modal.WithCaption(caption)` on the outer
`modal.Modal`. Pathselector's own `View()` never touches the caption —
the outer frame does. The field doc mentions the wrapper as the
default-fallback owner, but a reader of `pathselector.Options` would
reasonably expect the caption to render in the pathselector's own view.

```go
// select_path.go:41-45
caption := opts.Caption
if caption == "" {
    caption = "Select path"
}
return modal.New(id, &notificationTranslator{inner: content}, modal.WithCaption(caption)), nil
```

- [ ] Move `Caption` out of `pathselector.Options` and add it as a distinct parameter: `NewSelectPath(id, caption string, opts pathselector.Options)`.
- [ ] Keep the co-location and add a `Caption` godoc note explicitly stating the outer modal frame consumes it.
- [ ] Drop the `"Select path"` fallback — every call site already provides a specific caption, so the fallback is dead code.
