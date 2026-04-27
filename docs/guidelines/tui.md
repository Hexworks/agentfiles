# TUI Guidelines

Related: [Clean Architecture](clean_architecture.md),
[Clean Code](clean_code.md), [Domain Model](domain_model.md), and
[Testing](testing.md).

`agentfiles` is a TUI-only application. The terminal interface should make the
known choices visible, prevent avoidable mistakes, and keep the implementation
easy to change. Build TUIs as a thin delivery layer over application and domain
functions, not as the place where business behavior lives.

## Core Rule

The TUI may collect input, navigate between flows, ask for confirmation, and
render results. It must not implement business logic.

In this guideline, "render" means terminal presentation: text, layout, color,
tables, lists, and prompts. The `internal/render` package still owns the
business operation of building desired project files from profiles, projects,
and assets.

```text
Do:
- call `app.Service` or domain functions for use cases
- pass stable ids and typed request values into those functions
- render returned metadata, results, and errors into human-facing output
- keep visual formatting, labels, colors, and layout in `internal/tui`
```

```text
Don't:
- decide profile ownership, render eligibility, drift rules, or write policy in the TUI
- parse domain files or inspect target repositories directly from the TUI
- return preformatted user-interface strings from domain packages
- import `huh`, `bubbletea`, `bubbles`, or `lipgloss` outside the TUI layer
  unless a dedicated adapter has been deliberately introduced
```

The dependency direction should stay:

```text
cmd/af -> internal/tui -> internal/app -> domain and infrastructure packages
```

Domain and application packages should never import `internal/tui`.

## Local Charm Stack

Use the versions and import paths currently pinned in `go.mod` as the source of
truth. At the time this guideline was written, the project uses:

- `github.com/charmbracelet/huh` for menus, prompts, and forms.
- `github.com/charmbracelet/bubbletea` underneath `huh` and for any future
  custom model/update/view screens.
- `github.com/charmbracelet/bubbles` for reusable Bubble Tea components such as
  key bindings, tables, lists, viewports, spinners, text inputs, and help.
- `github.com/charmbracelet/lipgloss` for terminal styling, layout, measuring,
  and ANSI-aware rendering.

Charm's latest documentation may show `charm.land/.../v2` import paths. Do not
copy those paths into this repo unless the task is explicitly upgrading the
Charm dependencies and the migration is handled as its own change.

## Use `huh` For Forms

Most `agentfiles` workflows are forms. Prefer `huh` when the user is choosing
from known values, entering a few fields, or confirming an action.

```text
Do:
- use the shared `runForm` helper so Esc and ctrl+c behave consistently
- use `Select` for one value from a closed set
- use `MultiSelect` for many values from a closed set
- use `Input` or `Text` only when free-form text is actually needed
- use `Confirm` for irreversible or high-impact actions
- use `Note` sparingly for short context that prevents mistakes
- split a flow into multiple groups or steps when one answer determines later options
```

```text
Don't:
- ask users to type ids that the registry, profile, or project already knows
- show unrelated fields just because one form can technically hold them
- duplicate option lists that already have a domain owner
- create a new keymap at each call site when a shared helper covers the behavior
```

Option values should be stable machine values, usually ids or enum constants.
Labels should be short, scannable, and human-oriented.

```go
// Do: value is stable, label helps the user decide.
huh.NewOption("Profile - manage reusable content", "profile")

// Don't: value depends on presentation text.
huh.NewOption("Profile - manage reusable content", "Profile - manage reusable content")
```

## Keep Business Logic Behind Functions

A TUI flow should read like orchestration:

1. Collect input.
2. Convert the input into typed values or stable ids.
3. Call a use-case function.
4. Render the result.

Business functions should return structured values that the TUI can present.
They should not print to stdout, ask questions, choose colors, or know about
terminal width.

```go
// Do: the TUI asks the app layer to perform the operation.
manifest, err := service.AddProject(profileID, name, path, agents, assetIDs)
if err != nil {
	return err
}
fmt.Printf("added project %s at %s\n", manifest.Name, manifest.Path)
```

```go
// Don't: the TUI reimplements ownership or file-write policy.
if pathAlreadyUsedByAnotherProfile(path) {
	return errors.New("project path already registered")
}
```

Validation follows the same boundary. Field validators may check local input
shape, such as "required" or "must choose at least one". Domain validation, path
safety, compatibility rules, ownership, sync policy, and render policy belong in
the packages that own those rules.

## Apply Go Boundaries To TUI Flows

The Go guideline's package and I/O rules apply at the TUI edge. In TUI code,
"command handling" means menu choice handling, form submission, and Bubble Tea
message handling.

```text
Do:
- let the package that owns a concept validate it
- translate form answers into named structs, stable ids, or explicit arguments
- keep filesystem reads and writes behind app, persistence, render, and sync functions
- preserve actionable errors from lower layers and add presentation, not vague replacements
- test business rules where they live, then test TUI orchestration only where it adds behavior
```

```text
Don't:
- use `internal/tui` as a second command-handler layer for domain validation
- pass loose `map[string]any` values around when the data has stable meaning
- hide arbitrary file writes inside selection, filtering, or view-building code
- replace useful lower-level errors with generic TUI errors like "bad input"
```

## Render Metadata, Not Policy

The TUI owns visual rendering. Domain packages own business meaning.

Return metadata from business code when the interface needs a richer display:

```go
type ChangePreview struct {
	Kind   string
	Path   string
	Reason string
}
```

The TUI may decide whether that preview becomes a table, a list, a colored
line, or a grouped summary. It must not decide whether the change is valid,
whether the path is allowed, or whether applying it is safe.

When errors need structured presentation, return typed errors that implement
`Error() string` and expose enough fields for the TUI to render useful output.
For multiple independent problems, collect them and return them together instead
of failing at the first item when continuing is safe.

## Use Bubble Tea For Richer Screens

Use Bubble Tea directly when a form is no longer enough: long previews,
scrollable tables, async status, progress, filtering, or multi-pane inspection.

For Bubble Tea models:

```text
Do:
- keep the model limited to UI state and already-loaded view data
- use `Init` for startup commands when loading is needed
- use `Update` to translate messages into UI state changes and commands
- use `View` only to render the current model
- use `tea.Cmd` for I/O and calls into services
- return typed messages from commands
- use `tea.Batch` for independent concurrent commands
- use `tea.Sequence` when command order matters
```

```text
Don't:
- run goroutines inside Bubble Tea update logic
- perform slow I/O directly in `Update` or `View`
- use commands merely to pass data within the same update path
- let the model become the domain model
```

Commands may call business functions, but the business function must remain
testable without Bubble Tea. The command should translate the result into a
message, and `Update` should translate that message into UI state.

## Prefer Bubbles Components

Use Bubbles before building custom widgets. Components such as `table`, `list`,
`viewport`, `spinner`, `progress`, `textinput`, `textarea`, `help`, and `key`
already follow Bubble Tea patterns and handle many terminal details.

Use `bubbles/key` for key bindings instead of comparing raw strings everywhere.
Define a small key map with help text for screens that have more than basic
submit/back behavior, and render that help where it fits.

```text
Do:
- keep Enter, Esc, ctrl+c, arrows, j/k, and q behavior consistent with nearby screens
- show available non-obvious keys through a help component or concise footer
- disable key bindings when the action is not available
```

```text
Don't:
- invent custom navigation controls without a strong reason
- require mouse input
- hide destructive actions behind a single keystroke without confirmation
```

## Style With Lip Gloss

Use Lip Gloss for visual polish, but keep style choices restrained. The best TUI
looks intentional while staying fast to scan.

```text
Do:
- define named styles for roles such as title, muted text, success, warning, error, and selected row
- use width, height, max width, and max height to keep output stable
- use Lip Gloss width and size helpers for ANSI-aware measurement
- use borders, color, and emphasis to clarify structure, not to decorate every element
- make important states visible in text as well as color
```

```text
Don't:
- scatter inline style chains through business flows
- use color as the only signal for success, warning, error, or selection
- make narrow terminals wrap important labels into unreadable fragments
- over-style simple prompts that `huh` already renders clearly
```

Treat terminal width as variable. Full-screen Bubble Tea views should handle
`WindowSizeMsg`; form and output rendering should avoid assuming an 80-column
terminal when paths, ids, and error messages can be longer.

## Make The Interface Obvious

Borrow the practical parts of "Don't Make Me Think": users should not have to
decode the interface before they can act.

```text
Do:
- make the next useful action obvious
- use concrete verbs and nouns in menu labels
- keep descriptions short and specific
- ask only for information needed now
- put warnings before the irreversible action
- preserve output long enough for the user to read it
- make recovery clear when something fails
```

```text
Don't:
- make users remember exact ids, valid agents, or asset types
- bury the main action under decorative copy
- ask optional questions before the required path is clear
- use clever labels where plain labels would scan faster
- make an error technically correct but impossible to act on
```

Menus should be shallow and predictable. A good menu option says what will
happen, not how the code is organized. A good form title says where the user is.
A good field description removes ambiguity without becoming documentation.

## Handle Safety And Errors Deliberately

The TUI is the user's last checkpoint before local files are changed.

```text
Do:
- show a preview before writes
- require explicit confirmation before applying changes
- keep delete candidates opt-in
- render structured errors with severity, location, and likely next step when available
- distinguish user aborts from failures
```

```text
Don't:
- collapse drift, update, create, and delete-candidate states into vague text
- continue after an error when the business function says the operation failed
- print low-level implementation details without explaining the user impact
```

If a flow cannot safely continue, stop and return the error. If several
independent items can be checked safely, collect all problems and render them in
one pass so the user does not have to fix one issue at a time.

## Accessibility

Terminal apps still need accessible paths.

```text
Do:
- keep all workflows keyboard accessible
- make color-enhanced output understandable without color
- prefer plain text labels alongside symbols
- consider exposing `huh.WithAccessible(true)` through configuration or an environment variable
- avoid animations as the only indication that work is happening
```

```text
Don't:
- require mouse support
- rely on icons alone
- hide essential status in styling that screen readers cannot use
```

## Testing TUIs

Test the business behavior under the TUI at the app and domain layers first.
Then add focused TUI tests for formatting helpers, view-model mapping, and
Bubble Tea update behavior when those pieces carry risk.

```text
Do:
- unit-test service and domain behavior without a terminal
- test render helpers with stable metadata inputs
- drive Bubble Tea `Update` with messages when custom models are added
- use fakes for service calls when testing TUI state transitions
- do a short manual pass through changed forms before merging
```

```text
Don't:
- rely only on manual terminal testing for business rules
- make brittle snapshots of full ANSI output when a smaller assertion proves the behavior
- require a real user registry or project repository for TUI tests
```

When arguments about wording or layout get subjective, run a small usability
check: watch someone try the task, note where they pause or backtrack, and fix
the biggest obstacle first.

## Review Checklist

Before finishing a TUI change, check:

1. Is every business rule behind `app.Service` or a domain package?
2. Does `internal/tui` only collect input, coordinate calls, and render results?
3. Are closed sets presented as choices instead of free-form text?
4. Are destructive actions previewed and confirmed?
5. Are errors actionable and distinct from user aborts?
6. Does the flow work without color or mouse input?
7. Did tests cover the business behavior outside the TUI?

## References

- [Charm Huh package documentation](https://pkg.go.dev/github.com/charmbracelet/huh)
- [Charm Bubble Tea package documentation](https://pkg.go.dev/github.com/charmbracelet/bubbletea)
- [Charm Bubble Tea commands guidance](https://charm.land/blog/commands-in-bubbletea/)
- [Charm Bubbles README](https://github.com/charmbracelet/bubbles)
- [Charm Lip Gloss package documentation](https://pkg.go.dev/github.com/charmbracelet/lipgloss)
- [Don't Make Me Think summary](https://howtoes.blog/2025/06/07/dont-make-me-think-a-book-summary/)
