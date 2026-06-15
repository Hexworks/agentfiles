# Form modals review

Seven parallel subagent reviews (security, clean code, clean architecture, SOLID,
DDD, testing, Go) found no security issues, but converged on a cluster of
architecture / domain-boundary problems and a clear gap between the test suite
the description asked for and the test suite that landed.

Headline issues:

- **Domain rules leak into the TUI** — `assetManifestFromState` slugs the
  asset id and `AgentOptions()` owns the canonical agent vocabulary; both belong
  in domain code.
- **Modal payload types are inconsistent** — four modals return typed `*Input`
  structs, two return `*project.Manifest` (one of them in an invalid state).
- **Tests bypass the form wiring** — `submitForm` mutates state pointers and
  flips `form.State` directly, so `Validate(...)` callbacks, huh bindings, and
  the rejection path are unverified, contrary to what `description.md` asked
  for.
- **`gofmt -l` is non-empty** (`agents.go`, `create_asset_test.go`).
- **`edit_project.go` shallow-clones the manifest** and silently aliases
  `SelectedAssetIDs`.
- **UX inconsistencies** — Title-Case vs lowercase field titles diverge
  between modals.

Each issue below has a solution checklist. Tick exactly one box per issue, then
return for the fix pass.

For each issue: choose **one** solution by ticking exactly one `[x]` checkbox in
its **Suggested solutions** list. Leave the rest as `[ ]`. Then signal back.

---

## gofmt is not clean

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Use Behavior-Focused Tests" / general tooling cleanliness
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — Source Structure rule 6 ("let the language formatter win") and rule 7 ("avoid horizontal alignment that makes future edits noisy")

`gofmt -l internal/tui/modals/` lists two files. `agents.go:11-14` over-pads the
const names (a leftover hand-alignment from before `AgentClaudeCode` was added);
`create_asset_test.go:58` has an extra space before a trailing comment, breaking
the column with the next line. `make fmt` will rewrite both the next time it
runs, so any unrelated commit will pick up these diffs.

```go
// internal/tui/modals/agents.go — current
const (
    AgentCodex       = "codex"
    AgentClaudeCode  = "claude-code"
    AgentCursor      = "cursor"
    AgentOpenCode    = "opencode"
)

// gofmt -w wants:
const (
    AgentCodex      = "codex"
    AgentClaudeCode = "claude-code"
    AgentCursor     = "cursor"
    AgentOpenCode   = "opencode"
)
```

### Suggested solutions

- [x] Run `make fmt` and amend both files.
- [ ] Run `gofmt -w internal/tui/modals/agents.go internal/tui/modals/create_asset_test.go` only (narrower).

---

## Asset id slug derivation is domain policy inside the TUI

> [!WARNING]
>
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "Keep Business Logic Behind Functions"
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code", "Use Stable Identifiers"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Single Responsibility Principle

`assetManifestFromState` (`internal/tui/modals/create_asset.go:88-98`) sets
`ID: utils.Slug(state.Name)`. `app.Service.AddProject`
(`internal/app/service.go:133`) does the same for projects. The rule "the id of
an asset/project is the slug of its name" now lives in two places: the modal
and the service. The sibling `register_project.go:24-30` deliberately leaves
`ID` empty so `AddProject` owns the slug — the two modals are inconsistent
about the same policy. If the rule ever evolves (collision suffix, reserved
words, validation), one writer will silently produce stale ids.

```go
// internal/tui/modals/create_asset.go
func assetManifestFromState(state *createAssetState) asset.Manifest {
    return asset.Manifest{
        ID:   utils.Slug(state.Name), // <- TUI is the authoritative id-deriver
        Name: state.Name,
        ...
    }
}
```

### Suggested solutions

- [x] Drop `ID` from the modal's output: leave `Manifest.ID == ""` and have
      `app.Service.InitAsset` apply `utils.Slug(manifest.Name)` once, mirroring
      `AddProject`.
- [ ] Add a single `asset.NewManifest(name, ...)` (or move slug logic into
      `asset.Init`) so both the TUI and the service call into one constructor.

---

## Agent identifier list lives in the wrong bounded context

> [!WARNING]
>
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Put Domain Rules In Domain Code", "Use The Project Language"
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Stable Dependencies Principle
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "Don't duplicate option lists that already have a domain owner"

`internal/tui/modals/agents.go:10-27` is the only place in the codebase that
declares the closed set `{codex, claude-code, cursor, opencode}` as named
constants. The same strings drive render policy
(`internal/render/render.go:134, 151-154, 218-220, 239`), `project.Manifest.EnabledAgents`
validation, and `asset.SupportsAgent`. Putting the constants in a delivery
package means `render` and `project` cannot import them without inverting the
documented `tui -> app -> domain` direction. Adding a fifth agent requires
synchronized edits in two unrelated packages, with no compile-time signal.

```go
// internal/tui/modals/agents.go — domain vocabulary held by the TUI
const (
    AgentCodex      = "codex"
    AgentClaudeCode = "claude-code"
    AgentCursor     = "cursor"
    AgentOpenCode   = "opencode"
)
```

### Suggested solutions

- [ ] Move the four constants (and an `AllAgents()` helper) to `internal/project`
      (closest to `Manifest.EnabledAgents`); reduce `agents.go` to the
      huh-option wrapper.
- [x] Move them to `internal/config` next to `DefaultProfileSlug`, since `config`
      is already the canonical home for cross-package vocabulary constants.
- [ ] Introduce a new `internal/agent` package with a typed `Agent` (string
      alias) and `AllAgents()`. More invasive but unambiguous ownership.

---

## Register Project returns a half-built `*project.Manifest`

> [!WARNING]
>
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "translate form answers into named structs"
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Model Consistency Boundaries", "Keep Persistence A Detail"
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — SRP (mix of input collection and domain construction)

`register_project.go:24-30` returns `&project.Manifest{Name, Path, EnabledAgents}`
with `ID==""`, `SelectedAssetIDs==nil`, `CreatedAt==zero`.
`project.Manifest.Validate()` (`internal/project/project.go:33-41`) requires
`ID != ""`, so the returned value is by definition an invalid manifest — yet it
shares the same type as the post-`AddProject` artifact. The very same file
already declares `RegisterProjectInput` (line 13), the type the screen actually
needs. The other four input-style modals (`CreateProfileInput`,
`RegisterProfileInput`, `CreateFileInput`, plus the locally-typed
`RegisterProjectInput`) return value structs; only this one and `edit_project.go`
leak the domain type.

```go
// internal/tui/modals/register_project.go
return &project.Manifest{
    Name:          state.Name,
    Path:          state.Path,
    EnabledAgents: state.EnabledAgents,
    // ID="", SelectedAssetIDs=nil, CreatedAt=zero — Manifest.Validate() would reject this
}
```

### Suggested solutions

- [ ] Return the existing `RegisterProjectInput` value type; let the screen pass
      its fields to `app.Service.AddProject`, which already slugs and stamps
      `CreatedAt`.
- [x] Keep the `*project.Manifest` shape but introduce
      `project.NewDraft(name, path, agents)` that fills `ID`/`CreatedAt` so the
      returned value passes `Validate()`.

---

## Edit Project shallow-clones the manifest (aliasing risk)

> [!WARNING]
>
> - [docs/guidelines/go.md](../../../docs/guidelines/go.md) — "Prefer Explicit Types Over Loose Maps" / defensive-copy posture
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Model Consistency Boundaries"

`edit_project.go:25` does `updated := *existing`, a struct copy that shares
slice backing arrays. `Name`, `Path`, and `EnabledAgents` are overwritten with
form-state values, but `updated.SelectedAssetIDs` still points at the same
backing array as `existing.SelectedAssetIDs`. A downstream caller that appends
to or sorts one will mutate the other. The changelog claims the clone "avoids
mutating the caller's manifest in place" — only partly true. The existing
`TestEditProject_RoundTripUnchangedManifest` (`edit_project_test.go:55-83`)
checks `reflect.DeepEqual` and pointer distinctness but does not exercise
aliasing.

```go
return modal.NewForm("edit-project", form, func(*huh.Form) any {
    updated := *existing                        // shallow: aliases slices
    updated.Name = state.Name
    updated.Path = state.Path
    updated.EnabledAgents = state.EnabledAgents // overwritten -> safe
    // updated.SelectedAssetIDs still aliases existing.SelectedAssetIDs
    return &updated
})
```

### Suggested solutions

- [x] Defensively copy `SelectedAssetIDs` in the extract closure
      (`updated.SelectedAssetIDs = append([]string(nil), existing.SelectedAssetIDs...)`)
      and add a regression test that mutates `got.SelectedAssetIDs[0]` and
      asserts `existing.SelectedAssetIDs` is unchanged.
- [ ] If this issue's "Register Project" solution moves to typed inputs, do the
      same here: return an `EditProjectInput`, let the screen merge into the
      manifest it owns (resolves both issues together).

---

## Edit Project takes the full manifest (over-broad input)

> [!WARNING]
>
> - [docs/guidelines/solid.md](../../../docs/guidelines/solid.md) — Interface Segregation Principle
> - [docs/guidelines/tui.md](../../../docs/guidelines/tui.md) — "translate form answers into named structs, stable ids, or explicit arguments"

`NewEditProject(existing *project.Manifest)` (`edit_project.go:22`) accepts the
whole manifest just so the extract closure can carry `ID`,
`SelectedAssetIDs`, `CreatedAt` forward. The modal's real input surface is
three strings + a string slice. Tests already demonstrate the awkwardness:
`edit_project_test.go:13-21` constructs a manifest with `SelectedAssetIDs` and
`CreatedAt` that the modal never displays.

### Suggested solutions

- [x] Accept an `EditProjectInput{Name, Path, EnabledAgents}` and return the
      same type; let the screen merge the result into the manifest it already
      owns (also fixes the aliasing issue above).
- [ ] Keep the manifest parameter, but document the carry-forward semantics
      explicitly in the `NewEditProject` godoc and add the deep-copy for
      `SelectedAssetIDs`.
- [ ] No change — accept the broader input for prefill convenience.

---

## Tests bypass huh wiring (validators and bindings unverified)

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Assert Behavior, Not Mock Mechanics"
> - `tasks/current/0022_feature_form-modals/description.md` §"Tests" — "drives it via simulated `tea.KeyPressMsg` events"

The description explicitly asks for tests that drive the form via simulated
`tea.KeyPressMsg` events. The implementation instead writes directly to bound
state pointers and flips `form.State = huh.StateCompleted` in
`testhelpers_test.go:23-29`. Consequences:

- `Validate(requiredString)` / `Validate(requiredAgents)` callbacks at
  `create_profile.go:38,44`, `create_asset.go:54,66`, `edit_project.go:46,52,59`
  never fire during tests.
- `Value(&state.X)` bindings are never validated. If `buildCreateAsset`
  accidentally bound `Description` to `&state.Name`, every test would still
  pass.
- The rejection path (form refuses to advance with empty required field) has no
  coverage in any modal test.
- `submitForm` reads `form.Errors()` — a cached snapshot, empty for un-traversed
  fields — and treats that as "validation passed".

The changelog labels the simulated-events approach "Considered but rejected"
because huh's `nextField`/`nextGroup` messages are unexported, but `form.Update`
with `tea.KeyPressMsg{Code: tea.KeyEnter}` is the documented driver pattern
(used in `internal/tui/components/modal/form_test.go`).

```go
// internal/tui/modals/testhelpers_test.go — does not validate or drive fields
func submitForm(t *testing.T, form *huh.Form) {
    if errs := form.Errors(); len(errs) > 0 { // empty for never-touched fields
        t.Fatalf("form has validation errors before submit: %v", errs)
    }
    form.State = huh.StateCompleted
}
```

### Suggested solutions

- [x] Replace `submitForm` with a key-driven pump that calls `form.Update`
      with `tea.KeyPressMsg{Code: tea.KeyEnter}` until `form.State` becomes
      `huh.StateCompleted`; rewrite every modal's happy-path test to drive
      the form through that pump. Adds at least one
      `_RejectsEmptyRequiredField` test per modal that submits with an empty
      required field and asserts non-completed state + non-empty `Errors()`.
- [ ] Keep `submitForm` but add a thin "wiring" test per modal: build the form
      with empty initial values, call `form.Validate()` (or huh's equivalent),
      and assert at least one `errRequired` per required field. Covers
      validator wiring without the full pump.
- [ ] Document the deliberate shortcut in `testhelpers_test.go` (rename
      `submitForm` to `forceComplete`, fix the misleading comment described in
      the next issue) and accept the validator/binding coverage gap.

---

## `submitForm` comment misrepresents behavior

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Comments" rule 3 / rule 6
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Make the test flow visible"

`testhelpers_test.go:12-22` says the helper will "validate every required
field, mark all groups submitted, and set State to StateCompleted". The body
does none of the first two: it only reads `form.Errors()` (cached) and flips
`form.State`. A reader who trusts the comment will assume validators are
exercised. (This is the same root cause as the wiring issue above; address them
together when possible.)

```go
// validate every required field, mark all groups submitted, and set State
// ...
form.State = huh.StateCompleted // <- the only thing that actually happens
```

### Suggested solutions

- [ ] Rewrite the comment to match reality: "skips validation and group
      lifecycle; asserts no cached errors and forces `StateCompleted` so the
      modal adapter emits `ResolvedMsg`. Does not re-run validators."
- [x] If the previous issue chose the key-driven pump, this issue is resolved
      automatically — rename the helper and rewrite the comment in that same
      change.

---

## Field title casing is inconsistent across modals

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Understandability" rule 2 ("similar concepts should look similar across packages")
> - [docs/guidelines/domain_model.md](../../../docs/guidelines/domain_model.md) — "Use The Project Language"

`create_profile.go:35,41`, `register_profile.go:29`, `create_file.go:29` use
Title Case (`"Name"`, `"Path"`). `create_asset.go:51,57,63,69,74,80`,
`edit_project.go:43,49,55`, `register_project.go:43,49,55` use lowercase
(`"name"`, `"path"`, `"tags"`, `"compatible agents"`, `"enabled agents"`). The
user sees two different form styles depending on which menu they opened.
`docs/glossary.md` writes the canonical terms in Title Case.

```go
// create_profile.go
huh.NewInput().Key("name").Title("Name")...
// create_asset.go
huh.NewInput().Key("name").Title("name")...
```

### Suggested solutions

- [x] Adopt Title Case across all modal field titles: `"Name"`, `"Path"`,
      `"Type"`, `"Description"`, `"Tags"`, `"Compatible Agents"`,
      `"Enabled Agents"`, `"Exclusive Group"`.
- [ ] Adopt lowercase across all six modals.
- [ ] Extract a shared `labels.go` with constants (`labelName`, `labelPath`,
      `labelEnabledAgents`, ...) so the rule has one home and drift is
      compile-checked.

---

## Hidden Type defaulting in Create Asset

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Functions" rule 5 ("avoid surprising side effects")

`create_asset.go:44-46` rewrites `initial.Type` to `asset.TypeSkill` when the
caller passes an empty string. The doc comment on `NewCreateAsset` only says
"`initial` lets callers preload fields"; it does not warn that `Type==""` is
silently reinterpreted. A caller that wants "no preselection" cannot express
it.

```go
if state.Type == "" {
    state.Type = asset.TypeSkill
}
```

### Suggested solutions

- [ ] Document the default in the `NewCreateAsset` godoc: "an empty
      `initial.Type` is replaced by `asset.TypeSkill` so the Select field has a
      preselection".
- [x] Push the default into the call site that builds `initial` (the screen
      that opens the modal) so the modal stays a pure projection of its input.

---

## `joinTags` / `parseTags` lose tags that contain commas

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Understandability" rule 4 ("put boundary checks and edge-case handling in one obvious place")

`validators.go:30-47` and `create_asset.go:109-118` use plain `,` as the
delimiter with no escaping. A tag like `"foo, bar"` round-trips to two tags
(`"foo"`, `"bar"`). `asset.Manifest.Tags` accepts arbitrary strings, so this is
a real data path; the existing tests
(`validators_test.go:32-52`, `create_asset_test.go:102-109`) exercise only
well-formed inputs.

```go
// internal/tui/modals/validators.go
parts := strings.Split(csv, ",")  // no escape, no quote handling
```

### Suggested solutions

- [ ] Reject commas inside individual tags in `parseTags` (return an error / a
      `huh` validator on the field) so the user sees a clear message instead
      of silent splitting.
- [ ] Document tag-syntax constraints inline in the `tags` field's
      `Description` (no commas inside a tag) and accept the silent split as
      "user error".
- [x] No change — current behavior is good enough for free-form tag entry.

---

## Boilerplate duplication across six modals

> [!WARNING]
>
> - [docs/guidelines/clean_code.md](../../../docs/guidelines/clean_code.md) — "Code Smells" rule 5 ("needless repetition: duplicated rules that can drift apart")

Every modal file follows the same pattern: `New<X>` calls `build<X>`, which
builds state and a form, wrapped by `modal.NewForm` with a closure that
dereferences `state`. The Name/Path input pair is re-typed across four files
with subtly different `Title`/`Description` strings; the Title-Case-vs-lowercase
divergence above is exactly the drift this rule warns about. The `build<X>`
split also forces tests to re-implement the `extract` closure
(`create_asset_test.go:23-25, 63-65, 86-88` paste `assetManifestFromState(state)`
three times).

### Suggested solutions

- [x] Extract shared field constructors (`nameInput(value *string, desc string)`,
      `pathInput(...)`, `enabledAgentsSelect(...)`) in a new
      `internal/tui/modals/fields.go` and call them from every modal.
- [ ] Drop the `build<X>` / `New<X>` split. Either inline `build` into `New`
      and expose state via a test-only accessor, or have `build` return
      `(form, state, extract)` so tests reuse the same extract closure as
      production.
- [ ] No change — accept the duplication for now.

---

## `parseTags` / `joinTags` are asset serialization living in the TUI

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Common Closure Principle (asset tag (de)serialisation should change in the asset package)

`validators.go:30-47` and `create_asset.go:109-118` define how the user-facing
CSV maps to `asset.Manifest.Tags`. If a second consumer ever needs the same
round-trip (a CLI export, a different screen, a config import), the rule will
either move or get re-implemented. Today it is a small piece of asset
serialisation policy hosted by the modal package.

### Suggested solutions

- [ ] Move `parseTags` / `joinTags` to `internal/asset` (`asset.ParseTags`,
      `asset.JoinTags`) so the round-trip lives with the type it serialises.
- [x] Keep as-is and accept it as a TUI-side mapping until a second consumer
      appears.

---

## `internal/utils` accreting identifier policy (slug fallback)

> [!WARNING]
>
> - [docs/guidelines/clean_architecture.md](../../../docs/guidelines/clean_architecture.md) — Common Reuse Principle

`utils/slug.go:20` hard-codes the fallback `"item"` for empty inputs.
`internal/config` already declares `DefaultProfileSlug = "profile"` for the same
concept (profile id fallback). The project now has two sources of truth for
"what id do we return when slugging produces nothing".

### Suggested solutions

- [ ] Move `Slug` to `internal/config` next to `DefaultProfileSlug` (both
      become a single id-policy module).
- [x] Keep `Slug` in `utils` but accept a `fallback string` argument so callers
      pass `config.DefaultProfileSlug` (or any other domain-specific default).
- [ ] No change — `"item"` is a fine generic fallback and the duplication is
      cosmetic.

---

## `New<Modal>` constructors are barely tested

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — pyramid / coverage of public API

`runResolvedThroughModal` (`testhelpers_test.go:69-74`) rebuilds the modal
wrapper around the **form returned by the build helper**, not around the public
`New<X>` constructor. Each public constructor is only exercised by a
`UsesStableID` test that checks `m.ID()`. If `NewCreateProfile` got its extract
closure wrong (e.g. returned `*state` instead of `state` — different type to
consumers), every other test would still pass.

### Suggested solutions

- [x] Add one resolution-path test per modal that drives the **public**
      `New<X>(initial)` constructor (not the build helper) and asserts the
      `ResolvedMsg.Value` payload type and contents.
- [ ] Expose the extract closure from `build<X>` so the public constructor and
      tests share the same closure (resolves coverage gap without new tests).

---

## Prefill round-trip coverage is uneven

> [!WARNING]
>
> - [docs/guidelines/testing.md](../../../docs/guidelines/testing.md) — "Test Behavior, Not Implementation Details"

Only `TestNewCreateAsset_PrefillRoundtripsTags` (`create_asset_test.go:102`)
and `TestEditProject_RoundTripUnchangedManifest` (`edit_project_test.go:55`)
assert that initial values reach the bound state. `buildCreateProfile`,
`buildRegisterProfile`, `buildRegisterProject`, `buildCreateFile` accept
initial values but no test asserts the values actually land. A regression that
ignored the initial parameter would slip through.

### Suggested solutions

- [x] Add a `_PrefillSeedsState` test per modal: construct with non-zero
      initial values, assert every bound state field equals the seed.
- [ ] Fold prefill assertions into the existing happy-path tests by checking
      `state` before mutating it.
- [ ] No change — prefill is structurally identical across modals; trust
      `CreateAsset`'s test as representative.

---

## Verified non-issues (no action needed)

These items were flagged or considered and are intentionally **not** issues:

- **Security**: no exploitable issues. Modals are pure input collection;
  domain safety (path traversal, ownership, slug collisions) is enforced by
  `app.Service` per the architecture, which is the documented boundary.
- **`Value()` before `Options()` for huh Select/MultiSelect**: verified
  correct in `create_asset.go:59-60,76-77`, `edit_project.go:57-58`,
  `register_project.go:57-58` (the bug the changelog mentions has been
  fixed).
- **`errRequired = errors.New("required")`**: acceptable per
  [docs/guidelines/errors.md](../../../docs/guidelines/errors.md) — TUI shape
  validators are exempt from the typed-error rule.
- **Defensive copies on `EnabledAgents` / `CompatibleAgents`**: present and
  correct in `create_asset.go:41`, `edit_project.go:37`,
  `register_project.go:37`. Only `SelectedAssetIDs` in `edit_project.go`'s
  extract is unsafe (covered above).
- **`go vet ./...`**: clean.
