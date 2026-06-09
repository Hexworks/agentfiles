---
id: 0015
type: feature
status: pending
depends_on: 0014
---

# Refactor UI

After the analysis of the charm ecosystem packages we figured out how the UI for the MVP
should look like. We're going to use what's available in the Charm ecosystem without
introducing new dependencies.

**Important**: the application will now work in _alt screen mode_ (full screen).

## Actions

An _action_ is a function that invokes a backend function. It can have parameters, but it has no dependencies.
This differentiates an _action_ from a service that we have to instantiate then refer to the instance variable.

*action*s are produced by an **action factory** that holds a reference to `app.Service`:

```go
package actions

type Actions struct { svc *app.Service }

func New(svc *app.Service) *Actions { return &Actions{svc: svc} }

func (a *Actions) LoadProfiles() ([]*profile.Profile, errs.DomainError) { ... }
func (a *Actions) CreateProfile(in CreateProfileInput) (*registry.ProfileRef, errs.DomainError) { ... }
// ...
```

The factory is constructed once in `main.go` (after the Service is built) and threaded
through the TUI; individual screens hold the `*Actions` reference, not the Service. This
preserves dependency injection and keeps tests able to substitute fake services.

An _action_ is therefore a method on the factory that invokes a specific business
function on the service.

**Return shape**: every action returns `(T, errs.DomainError)` — uniformly, including for
side-effect-only actions. For those, `T` is `struct{}` (or any other zero-value placeholder
agreed on at the package level). Domain errors are appended to the [Notifications](#notifications)
list when present.

*action*s can have either no parameters or exactly `1` parameter — **strict rule**. When the
underlying Service function needs more than one input, the action wraps them in a dedicated
input struct (e.g. `CreateProfileInput{Name, Path string}`,
`AddProjectInput{Name, Path string; EnabledAgents, AssetIDs []string}`). The action is then
responsible for unpacking the struct and forwarding the fields to the Service call.

Form modals (see [Modals](#modals)) already return a typed value; that value is the action's
single parameter.

*action*s need to integrate with the charm ecosystem seamlessly (see the related [guidelines](/docs/guidelines/charm.md)).

## Notifications

Notifications is a **ring buffer** of `Notification` entries with a cap of `500` entries.
When a new entry is added past the cap, the oldest entry is dropped. The buffer is
in-memory only — it does not persist across runs.
Example:

```
{
    Level: "INFO",
    Text: "Profile 'hello' created successfully.",
    CreatedAt: "2026.01.05 15:13:35"
}
```

- Whenever an _action_ returns with an error an entry is added to the `Notifications` list with `ERROR` level
  containing the error message as `Text`
- Whenever an _action_ executes successfully an entry is added to `Notifications` with `INFO` level
  and with a `Text` that describes what action was executed successfully.

`Notification`s also show up in the _notification area_ on each screen when created then removed
after `5 seconds` (this is usually called a "Toast" message).

When multiple notifications are produced within a 5-second window they are **queued**: only one
toast is visible at a time, displayed for its full 5 seconds before the next one in the queue
takes its place. The queue is FIFO. The persistent [Notifications Modal](#notifications-modal)
shows every entry regardless of queue state.

`Notification`s can be viewed by opening the [Notifications Modal](#notifications-modal)

## Components

### Mnemonic buttons

Mnemonic buttons are UI elements that appear as a button: `[Button]` and can be activated with a mnemonic.
This means that pressing a specific key will activate the _action_ that is bound to the button.

The implementation should go into the `internal/tui/components` folder (similar to Modal)

Mnemonic buttons have the following parameters that can be passed when created:

- `label`: The label of the button: `[{{label}}]`
- `mnemonic`: the key that will - when pressed - execute the _action_. For example `a` (must be 1 character).
- `action`: The _action_ (see above) that we'll execute when the button is pressed
- `data`: (optional) this is the parameter that we'll pass to the _action_.

Each screen can have any number of _Mnemonic buttons_ but the `mnemonic` has to be unique for the screen
(so no 2 buttons with the same mnemonic).

The `mnemonic` must be a key that is contained in `label`. Visually the `mnemonic` will have a different color
than the rest of the `label` to highlight that the key is a mnemonic.

#### Safety

Uniqueness is enforced two ways:

1. **Runtime helper** (`internal/tui/components/mnemonic/Set`): a registry collection
   that screens register buttons through instead of constructing them ad-hoc. `Set.Add`
   panics on duplicate mnemonic; `Set.View` and `Set.Match` iterate the registered
   buttons. This catches dupes at first render of any focus / selection state.
2. **Per-screen unit tests**: every screen ships a `*_test.go` that walks the screen
   through each possible focus / selection state and asserts no two visible mnemonic
   buttons share a key. Treetable rows with row-context buttons are exercised by
   moving the cursor across all node types.

A's runtime panic is good for catching dupes during dev; the tests catch them in CI
before reaching the user.

### Modals

We'll use Modals (see more in the modal documentation on how they work), these are already implemented.

### Tables

We'll use `bubbles`' table component throughout the app. It already supports scrolling
so we don't need to implement that. Tables always have a fixed height so that they can fit on the screen.

#### Selection

When a row is selected (bubbles supports this) we need to display the actions that can be performed on
the selected item in the "Actions" column. We do this only on the selection to prevent visual noise
and possible mnemonic button duplications.

Actions are invoked using mnemonics, that we need to visually display in the actions. Each mnemonic is
a letter, for example:

- Register -> mnemonic key is `r` (eg: if `r` is pressed the action is invoked). We need to use a highlight color
  on the "R" (eg: R is using highlight color, the rest of the word uses text color)
- Delete -> mnemonic key is `d`

We are using _mnemonic buttons_ (see above). _mnemonic buttons_ need to be unique, this is another reason why we
only display them only on the selected + focused row.

#### Scrolling

`bubbles`' table component supports scrolling, all tables we use should have a fixed size depending on the available screen
real estate.

#### Highlight and navigation

In all tables we navigate with:

- ↑/k up -> move up
- ↓/j down -> move down

and the currently selected item should be visually distinct. bubbles supports this.

### Treetable

`internal/tui/components/treetable` already exists and supports a `Name` column +
optional `Actions` column with row-context mnemonic buttons. It is used on
[Edit Asset Screen](#edit-asset-screen) (files panel) as-is.

The [Plan Project Screen](#plan-project-screen) needs four columns: `Name`, `Status`,
`Current Action`, `Actions`. The treetable component must be **extended** with a new
option so callers can inject N intermediate value columns between the `Name` column
and the `Actions` column:

```go
type ValueColumn struct {
    Column
    Value func(*Node) string
}

func WithValueColumns(cols ...ValueColumn) Option
```

`Value` is called once per render per row and returns the cell text. Rendering order
is: `Name` → injected value columns (in order) → `Actions`. The `Actions` column
behavior (cursor-only render, mnemonic routing) is unchanged.

This keeps the treetable reusable for future multi-column tree views instead of
forking a one-off `plantable` widget.

## Screens

Each screen is opened by calling a function that constructs the screen.
These functions can accept a single parameter when called (usually identifiers) for
loading the data for the screen. These are documented (see below).

_Note that_ on **all screens** there is a status bar with the current available
key bindings. This is supported by _bubbletea_ applications out of the box.

The status bar is **dynamic**: it always shows the global key bindings (see below),
plus — when a row is selected and its table is focused — the mnemonic keys exposed
by that row's [Mnemonic buttons](#mnemonic-buttons) (e.g. `e edit`, `d delete`).
Screen-level buttons (e.g. `c create`, `r register`, `b back`) are visible as
labelled buttons on the screen itself and are **not** repeated in the status bar.
This keeps the bar from doubling as visual noise next to the button row.

Key bindings that are _always available_:

- n notifications -> this opens a modal that shows the application log
- s settings -> this opens the settings screen
- q quit -> quits the app

The following is a template for all screens:

```
                                                 ┌───────────────┐
                                 ┌───────────────┤  All screens  │   ┌──────────────────────────┐
┌────────────────┐               │               │ have a title  │   │  Below the content area  │
│▒▒▒{{Title}}▒▒▒▒◀───────────────┘               └───────────────┘   │    there is a single     │
└────────────────┘                                                   │  notification line that  │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   │   gets cleared after 5   │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   │         seconds          │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   └──┬───────────────────────┘
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │
░░░░░░░░░░░░░░░░░░░░░░░░░░░{{content}}░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │
░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░      │   ┌───────────────────┐
▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒{{last▒notification}}▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒◀──────┘   │Shortcuts are shown│
██████████████████████████{{shortcuts}}██████████████████████████◀──────────┤   at the bottom   │
                                                                            └───────────────────┘
```

### Welcome Screen

When the app is started with `af` the user lands on this screen (see mockup below).

- "Profiles" navigates to the [Profiles Screen](#profiles-screen) (see below)
- "Settings" navigates to the _Settings Screen_ (see below)
- "Quit" exits the app

At the bottom there is the statusbar that shows the possible key bindings:

- ↑/k up -> move up in the list
- ↓/j down -> move down in the list
- enter/v -> choose currently selected list item
- n -> show notifications
- s -> go to settings screen
- q -> quit the app

```
╭────────────╮
│ Agentfiles │
╰────────────╯

┃ Choose a task
┃ > Profiles
┃   Settings
┃   Quit

{{ notification area (no content == invisible by default) }}

↑/k up • ↓/j down • enter/v choose • n notifications • s settings • q quit • ? help
```

### Profiles Screen

When the screen is loaded `Profiles` are loaded using the [Load Profiles](#load-profiles) _Action_ (no parameters).

We use the bubbles table component on this screen.
When a profile is selected in the table we add 2 _mnemonic buttons_

- Pressing `e` loads the [Edit Profile Screen](#edit-profile-screen), using the selected `Profile`'s id as parameter.
- Pressing `d` deletes the profile via a **two-step** confirmation flow:
    1. First [Confirmation Modal](#confirmation-modal): "Are you sure you want to delete profile {{name}}?"
       - "No" → abort, no further prompts.
       - "Yes" → proceed to step 2.
    2. Second [Confirmation Modal](#confirmation-modal): "Also delete profile folder on disk?"
       - "No" → invoke [Delete Profile](#delete-profile) with `KeepFolders` (default).
       - "Yes" → invoke [Delete Profile](#delete-profile) with `DeleteFolders`.

Regardless of table selection

- Pressing `c` opens the [Create Profile Modal](#create-profile-modal) (see below).
- Pressing `r` opens the [Register Profile Modal](#register-profile-modal) (see below).

```
╭──────────╮
│ Profiles │
╰──────────╯

┌────────────────────────────────────────────────────────────────────────────────────┐
│ ID          Name        Path                            Actions                    │
│────────────────────────────────────────────────────────────────────────────────────│
│ another     Another     /Users/addamsson/af/another                                │
│ test        Test        /Users/addamsson/af/profiles/   [Edit] [Delete]            │ <-- selected row
│ ...                                                                                │
└────────────────────────────────────────────────────────────────────────────────────┘
 [Create New Profile] [Register Profile]

{{ notification area (no content == invisible by default) }}

↑/k up • ↓/j down • n notifications • s settings • q quit
```

### Edit Profile Screen

Parameters: the `id` of the `Profile`

When the Edit Profile Screen is opened we load the `Profile` with the `id` that is passed to this screen
using the [Load Profile](#load-profile) _action_.

On the Edit Profile Screen there are 2 tables. _Focus_ can be shifted between the tables using the
`<tab>` key (forwards) or `<shift>+<tab>` (backwards) and with the focus mnemonic keys `1` and `2`.

> [!NOTE]
> Throughout the app, focus-mnemonic digit shortcuts use the `Ctrl` modifier
> (`focus.WithModifier(focus.ModCtrl)`) — i.e. the displayed label is `[1]` but the
> binding fires on `ctrl+1`. This keeps plain digits available to focused text inputs.
> The visual `[N]` indicator stays unchanged.

The UI needs to fit on the current screen. The heading, the buttons, the notifications and
the status parts have a fixed size, so we need to calculate the tables' size based on this.

#### Assets

The assets table lists all the `Asset` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:

- Pressing `e` (mnemonic) will open [Edit Asset Screen](#edit-asset-screen) with the `id` of the selected `Asset`
- Pressing `d` (mnemonic) will open a [Confirmation Modal](#confirmation-modal) with a command
  that deletes the selected `Asset` using the [Delete Asset](#delete-asset) _action_

Below the _assets table_ there is a mnemonic button: "Create Asset". It is invoked by pressing `c`.

When "Create Asset" is invoked the [Create Asset Modal](#create-asset-modal) is displayed. If it
isn't canceled an `Asset.Manifest` is returned and the [Create Asset](#create-asset) _action_
is invoked.

#### Projects

The projects table lists all the `Project` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:

- Pressing `e` (mnemonic) will open the [Edit Project Modal](#edit-project-modal) prefilled
  with the selected `Project`'s `Name`, `Path`, and `EnabledAgents`. If confirmed,
  the [Update Project](#update-project) _action_ is invoked with the modified `Project`.
- Pressing `a` (mnemonic) will open the [Select Project Assets Screen](#select-project-assets-screen) with the `id` of the selected `Project`
- Pressing `p` (mnemonic) will open the [Plan Project Screen](#plan-project-screen) with the `id` of the selected `Project`
- Pressing `d` (mnemonic) will open a [Confirmation Modal](#confirmation-modal) with a command
  that deletes the selected `Project` using the [Delete Project](#delete-project) _action_

Below the _projects table_ on the left side there is a mnemonic button: "Register Project". It is invoked by pressing `r`.

When "Register Project" is invoked the [Register Project Modal](#register-project-modal) is displayed. If it
isn't canceled a `Project` is returned and the [Register Project](#register-project) _action_ is invoked.

Below the _projects table_ on the right side there is a mnemonic button: "Back". It is invoked by pressing `b`.

When `b` is pressed we go back to the [Profiles Screen](#profiles-screen).

The following mockup shows how the Edit Profile Screen should look like.

```
                                          ┌───────────────────────────┐
                                          │Table caption displayed    │
     ┌────────────────────────┐           │next to the focus mnemonic │
     │Editing {{profile-name}}│           │button                     │
     └────────────────────────┘           └──┬────────────────────────┘
                  ┌──────────────────────────┤
┌────▶[1]─Assets─◀┴──────────────────────────┼─────────────────────────────────────────────┐
│    │ Id       Name           Type          │                    Actions         ◀────────┼──┐
│    │───────────────────────────────────────┼─────────────────────────────────────────────│  │
│    │ #1       AGENTS.md      agents_doc    │                    [Edit] [Delete]          │  │
│    │ #2       review-code    skill         │                                             │  │
│    │                                       │                                             │  │
│    │                                       │                                             │  │
│    │                                       │                                             │  │
│    └───────────────────────────────────────┼─────────────────────────────────────────────┘  │
│     [Create Asset]                         │                                                │
│                     ┌──────────────────────┘                                                │
│ ┌──▶[2]─Projects─◀──┴────────────────────────────────────────────────────────────────────┐  │
│ │  │ Id       Name           Path                               Actions                  │  │
│ │  │─────────────────────────────────────────────────────────────────────────────────────┤  │
│ │  │ #1       My project     /home/profiles/some           [Edit] [Assets] [Plan] [Delete] │  │
│ │  │ #2       Other proj     /home/profiles/other                                        │  │
│ │  │                                                                                     │  │
│ │  │                                                                                     │  │
│ │  │                                                                                     │  │
│ │  └─────────────────────────────────────────────────────────────────────────────────────┘  │
│ │   [Register Project]                                                             [Back]   │
│ │                                                                                           │
│ │   {{ notification area (no content == invisible by default) }} ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒    │
│ │                                                                                           │
│ │   ↑/k up • ↓/j down • b░back░• n notifications • s settings • q quit░░░░░░░░░░░░░░░░░░    │
│ │                                                                                           │
│ │      ┌───────────────────────────┐                ┌─────────────────────────────┐         │
│ │      │Mnemonic shortcuts (1 and  │                │Only one table can be        │         │
└─┼──────┤2) that focus the table,   │                │focused, and only one row can│         │
  │      │rendered on the table      │                │be selected per table. We    ├─────────┘
  └──────┤border                     │                │only display mnemonic buttons│
         └───────────────────────────┘                │where a table is focused and │
                                                      │a row is selected.           │
                                                      └─────────────────────────────┘
```

### Edit Asset Screen

Parameters: the `id` of the `Asset`

When the Edit Asset Screen is opened we load the `Asset` with the `id` that is passed to this screen
using the [Load Asset](#load-asset) _action_.

The following mockup shows the Edit Asset Screen. There are 2 columns each occupying 50%
of the available horizontal space.

The UI needs to fit on the current screen. The heading, the buttons, the notifications and
the status parts have a fixed size, so we need to calculate the columns' size based on this.

> [!NOTE] that fields that have a list type (eg: `[]string`) will be joined when displayed, so
> `["foo", "bar"]` will be displayed as `foo, bar` and after editing (on `Blur()`) they will
> be transformed back to a list, so `foo, bar` turns into `["foo", "bar"]`. We split and join
> using `,` as separator and we strip whitespace (eg: `", "` -> `","`)

#### Files

In the left column there is a _treetable_ that shows the contents of the _asset_'s directory.
It occupies 50% of the available horizontal space, and 90% of the available vertical space.

Pressing `1` (focus handling mnemonic button) will focus the `treetable`.

Pressing `o` ("Open" mnemonic button) opens the file for editing using the `tui/editor` functionality.
After the editor is closed we return to the screen and [Update Asset](#update-asset) is called
with the current in-memory `Asset`.

> [!IMPORTANT]
> `UpdateAsset` writes the metadata **and** walks the asset directory to refresh
> per-file SHA — the caller does not need to recompute hashes or reload the asset
> beforehand. This is what makes a file edit surface as `ChangeUpdate` on the
> [Plan Project Screen](#plan-project-screen) instead of `ChangeDrift`.

> [!IMPORTANT]
> the "Open" mnemonic button is only rendered for leaf nodes (files), not for directories

Pressing `d` ("Delete" mnemonic button) opens a _confirmation dialog_ and if "yes" is pressed it:

- deletes the physical file
- deletes the file from the `Asset` object
- calls the [Update Asset](#update-asset) function with the updated `Asset` object

Pressing `a` ("Add" mnemonic button) when the files treetable is focused will open the [Create File Modal](#create-file-modal)
and if confirmed it

- creates the physical file at the given path (relative to the asset folder)
- Adds the new file to the `Asset` object
- calls the [Update Asset](#update-asset) function with the updated `Asset` object

#### Customize

In the right column there is a _group_ named "Summary" that shows non-editable data
(`Name`, `Type`). It occupies 50% of the horizontal and 30% of the vertical space available.

Below it there is a _group_ named "Customize" that shows editable fields and it occupies
50% of the horizontal and 60% of the vertical space available. The editable fields are:

- `Description` (multi-line text input)
- `Tags` (comma-separated text input)
- `CompatibleAgents` (multi-select: `codex`, `claude-code`, `cursor`, `opencode`; empty = all)
- `ExclusiveGroup` (single-line text input)

Pressing `2` (focus handling mnemonic button) will focus the "Description" field.

Whenever focus is moved away from an input field (on `Blur()`) the `Asset` object is updated.

Pressing `e` ("Save" mnemonic button) calls the [Update Asset](#update-asset) function with the updated `Asset` object.

Pressing `b` ("Back" mnemonic button) navigates to the [Profiles Screen](#profiles-screen).

> [!IMPORTANT]
> pressing `b` **must ask** for confirmation if there are unsaved changes.

```
                                                                                     ╔═════════════════════╗
                                         ╔══════════════════════════════════════╗    ║Non-editable summary ║
                                         ║Treetable component is used to show   ║    ║fields               ║
                                         ║the files in the asset folder         ║    ║                     ║
                                         ╚═════╤════════════════════════════════╝    ╚═══════════╤═════════╝
┌────────────────────────────┐                 │                                                 │
│Editing Asset {{asset.name}}│            ┌────┘                                      ┌──────────┘
└────────────────────────────┘            │                                           │
┌[1]─Files────────────────────────────────┼───────────┐┌────Summary───────────────────▽─────────────────────┐
│ Name                              Actions           ││ Name                {{asset.Name}}                 │
│─────────────────────────────────────────┼───────────││ Type                {{asset.Type}}                 │
│ review-task/                            ▽           ││                                                    │
│ ├── SKILL.md                                        ││                                                    │
│ ├── scripts/                                        ││                                                    │
│ │   └── git-commit.sh             [Open]  [Delete]  ││                                                    │
│ ├── references/                                     │└────────────────────────────────────────────────────┘
│ │   ├── good-example.md                             │┌[2]─Customize───────────────────────────────────────┐
│ │   └── bad-example.md                              ││                     ┌────────────────────────────┐ │
│ └── assets/                                         ││ Description         │{{asset.Description}}       │ │
│     └── pr-template.md                              ││      ┌──────────────▷                            │ │
│                                                     ││      │              │                            │ │
│                                                     ││      │              └────────────────────────────┘ │
│                                                     ││      │              ┌────────────────────────────┐ │
│                                                     ││ Tags │              │{{asset.tags}}              │ │
│                                                     ││      │              └─────────△──────────────────┘ │
│                                                     ││      │                        │                    │
└─────────────────────────────────────────────────────┘└──────┼────────────────────────┼────────────────────┘
                                                              │                        │       [Save] [Back]
                                                              │                        │
 {{ notification area (no content == invisible by default) }} ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒
                                                              │                        │
 ↑/k░up░•░↓/j░down░•░e░save░•░b░back░•░n░notifications░•░s░settings░•░q quit░•░?░help░░░░░░░░░░░░░░░░░░░░░░░░
                                                              │                        │
                                                              │                        │
                                       ╔══════════════════════╧══╗   ╔═════════════════╧════════╗
                                       ║Multi-line text input    ║   ║Text input where tags can ║
                                       ╚═════════════════════════╝   ║be added (comma-separated)║
                                                                     ╚══════════════════════════╝
```

### Select Project Assets Screen

Parameters: the `id` of the `Project`

When this screen is opened we load the `Project` with the `id` that is passed to this screen
using the [Load Project](#load-project) _action_.

The following mockup shows the Select Project Assets Screen. There are 2 tables below each other and they
have the same size.

The UI needs to fit on the current screen. The heading, the buttons, the notifications and
the status parts have a fixed size, so we need to calculate the tables' size based on this.

There is a "Plan" button below the "Available Assets" table that will navigate to the [Plan Project Screen](#plan-project-screen).

> [!NOTE]
> The Plan screen will use the current state that exists. Actions performed on this screen are automatically
> saved (select/unselect)

#### Selected Assets

The Selected Assets table contains all the assets that are selected by the user for this _project_.

Pressing `1` (focus handling mnemonic button) will focus the table.

The following context actions are available to selected rows in this table:

- Pressing `u` ("Unselect" mnemonic button) will remove the selected asset from the `Project` and will call the
  [Update Project](#update-project) _action_ with the updated `Project` object.

#### Available Assets

The Available Assets table contains all the assets that are available for selection (eg: they are part of the)
profile, but are not selected for the `Project`).

Pressing `2` (focus handling mnemonic button) will focus the table.

The following context actions are available to selected rows in this table:

- Pressing `l` ("Select" mnemonic button) will select the `Asset` for the `Project` and will call the
  [Update Project](#update-project) _action_ with the updated `Project` object.

```
┌────────────────────────────────────────────────────────────────┐
│Selecting assets for project {{project.name}} ({{profile.name}})│
└────────────────────────────────────────────────────────────────┘
┌[1]─Selected─Assets────────────────────────────────────────────────────────────────────────────────────────┐
│ Id    Name              Type                      Exclusive Group            Actions                      │
│───────────────────────────────────────────────────────────────────────────────────────────────────────────│
│ #1    AGENTS.md         agents_doc                agents_doc                 [Unselect]                   │
│ #2    review-code       skill                                                                             │
│                                                                                                           │
│                                                                                                           │
│                                                                                                           │
│                                                                                                           │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────┘
┌[2]─Available─Assets───────────────────────────────────────────────────────────────────────────────────────┐
│ Id    Name              Type                      Exclusive Group             Actions                     │
│───────────────────────────────────────────────────────────────────────────────────────────────────────────│
│ #1    create-task       skill                                                 [Select]                    │
│ #2    do-task           skill                                                                             │
│ #3    AGENTS2.md        agents_doc                agents_doc                                              │
│                                                                                                           │
│                                                                                                           │
│                                                                                                           │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                                                                               [Plan] [Back]

 {{ notification area (no content == invisible by default) }} ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒

 ↑/k░up░•░↓/j░down░•░e░save░•░b░back░•░n░notifications░•░s░settings░•░q░quit░•░?░help░░░░░░░░░░░░░░░░░░░░░░░░
```

### Plan Project Screen

Parameters: the `id` of the `Project`

When this screen is opened we load the _Project Plan_ for the `Project` with the `id` that is passed to this screen
using the [Plan Project](#plan-project) function.

The following mockup shows the Plan Project Screen. There is a single treetable on the screen.

The UI needs to fit on the current screen. The heading, the buttons, the notifications and
the status parts have a fixed size, so we need to calculate the table's size based on this.

There is a "Apply" button below the "Plan Project" table that will call the [Sync Project](#sync-project)
function with the user's selections.

All files in the plan have a `ChangeKind` value. "create", "update" and "delete" are all managed
changes (eg: user created new asset metadata, deleted asset metadata or updated a file on the filesystem)
so there are no actions that the user can perform. "drift" and "unknown" might need user intervention:

"drift": means a previously managed file was modified locally, apply would overwrite those edits. User should be able to choose:

- overwrite: overwrite the changes
- keep: don't touch the changes
  "Keep" is chosen by default

"unknown" marks an unrecognized file that was never managed.

- delete: delete the file
- keep: don't touch the file
  "Keep" is chosen by default

The table has the following fields:

- Name: the path of the file (or directory)
- Status: The `ChangeKind` of the file (only applicable for files, not directories)
- Current Action: is the action that is currently chosen (only applicable for files, not directories)
- Actions: a single mnemonic button that **toggles** to the alternative — the current
  action lives in the "Current Action" column, the button shows the other option:
    - row `Status = drift`:
        - `Current Action = Keep` → button `[Overwrite]` (mnemonic `o`)
        - `Current Action = Overwrite` → button `[Keep]` (mnemonic `k`)
    - row `Status = unknown`:
        - `Current Action = Keep` → button `[Delete]` (mnemonic `d`)
        - `Current Action = Delete` → button `[Keep]` (mnemonic `k`)
  The button is only present on the selected + focused row (consistent with all other
  tables). Pressing the button swaps `Current Action` and re-renders the row.

Pressing the mnemonic button `Apply` (mnemonic `a`) constructs a list of changes and calls [Sync Project](#sync-project) with it.

```
┌──────────────────────────────────┐
│Planning Project {{project.name}} │
└──────────────────────────────────┘
┌Changes────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ Name                               Status      Current Action   Actions                                   │
│────────────────────────────────────────────────────────────────────────────────────────────────────────── │
│foo/                                                                                                       │
│└── bar/                                                                                                   │
│    └── hello.md                    ? unknown   Keep             [Delete]                                  │
│.claude/                                                                                                   │
│├── commands/                                                                                              │
││   └── rewrite.md                  - delete                                                               │
│└── skills/                                                                                                │
│    ├── implement-task/                                                                                    │
│    │   └── skill.md                ~ update                                                               │
│    └── review-task/                                                                                       │
│        ├── skill.md                + add                                                                  │
│        └── review-template.md      * drift     Keep             [Overwrite]                               │
│                                                                                                           │
│                                                                                                           │
│                                                                                                           │
└───────────────────────────────────────────────────────────────────────────────────────────────────────────┘
                                                                                              [Apply] [Back]

 {{ notification area (no content == invisible by default) }} ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒

 ↑/k░up░•░↓/j░down░•░e░save░•░b░back░•░n░notifications░•░s░settings░•░q quit░•░?░help░░░░░░░░░░░░░░░░░░░░░░░░
```

### Settings Screen

Parameters: none

> [!NOTE]
> MVP stub. The screen renders a single line of text ("Coming soon") plus a `[Back]`
> mnemonic button (mnemonic `b`). Pressing `b` returns to the previous screen (Welcome
> Screen if entered via the menu; whichever screen was active if entered via the global
> `s` shortcut).
>
> The Welcome Screen menu entry and the global `s` binding stay in place so future work
> can flesh the screen out without touching navigation.

```
╭──────────╮
│ Settings │
╰──────────╯

 Coming soon.

                                                                                       [Back]

 {{ notification area (no content == invisible by default) }}

 b░back░•░n░notifications░•░q░quit░•░?░help
```

## Modals

Modals are layers on top of the existing UI with their own focus handling (see documentation).
Modals can return values when closed.

All modals can be closed without action by pressing the `<esc>` key.

### Confirmation Modal

The confirmation dialog opens a simple "Yes" / "No" (modal) dialog.

Parameters:

- `cmd`: a command that can be executed without a parameter. If you wrap
  an _action_ that has a parameter then you need to bind the parameter first
  so that it can be invoked without passing a parameter.
- `cmd_name`: the textual representation of `cmd` (eg: its name)

```
┌──────────────────────────────────────────────┐
│                                              │
│                                              │
│                                              │
│   Are you sure you want to {{cmd_name}}?     │
│                                              │
│                                              │
│                                              │
│   ┌────────┐                  ┌────────┐     │
│   │  Yes   │                  │   No   │     │
│   └────────┘                  └────────┘     │
│                                              │
└──────────────────────────────────────────────┘
```

- If "Yes" is selected the `cmd` is invoked and the dialog is closed and we return
  with the result of executing the command
- If "No" is selected the modal is closed and we return with nothing

### Info Modal

The info modal shows textual information in a popup. This popup contains a viewport so that longform text can also be
displayed. (see the components/help component for more info.)

### Create Asset Modal

When the _Create Asset_ modal is opened it shows a _form modal_ with _1_ form group
containing the following fields:

> [!IMPORTANT]
> A unique ID will be generated for the `Asset` by slugifying `Name` (reusing the existing
> `slug()` helper in `internal/app/service.go`). If the resulting id collides with an
> existing asset in the profile, the action surfaces an `AssetExistsError` and the user
> is asked to pick a different `Name`.

- `Name`
    - label: name
    - description: The name of the asset (eg: `agents.md`)
    - required: `true`
- `Type`
    - label: type
    - description: The type of the asset (eg: `agents_doc`)
    - required: `true`
- `Description`
    - label: description
    - description: Describe the asset
    - required: `true`
- `Tags`
    - label: tags
    - description: Assign (optional) tags, eg: "git, build"
    - required: `false`
- `CompatibleAgents`
    - label: compatible agents
    - description: Multi-select of agents this asset renders for
      (`codex`, `claude-code`, `cursor`, `opencode`). Empty means "all enabled agents".
    - required: `false`
- `ExclusiveGroup`
    - label: exclusive group
    - description: Assign (optional) exclusive group (eg: `agents_doc`)
    - required: `false`

If the form is submitted an `Asset.Manifest` object is returned.

### Edit Project Modal

When the _Edit Project_ modal is opened it shows a _form modal_ prefilled with the
existing `Project`'s values, with _1_ form group containing the following fields:

- `Name`
    - label: name
    - description: The name of the project
    - required: `true`
- `Path`
    - label: path
    - description: The path of the project
    - required: `true`
- `EnabledAgents`
    - label: enabled agents
    - description: Multi-select of agents enabled for this project
      (`codex`, `claude-code`, `cursor`, `opencode`). At least one required.
    - required: `true`

> [!NOTE]
> The `Project.ID` is **not** editable. `SelectedAssetIDs` are also untouched here —
> use the [Select Project Assets Screen](#select-project-assets-screen) for that.

If the form is submitted the modified `Project` object is returned.

### Register Project Modal

When the _Register Project_ modal is opened it shows a _form modal_ with _1_ form group
containing the following fields:

> [!IMPORTANT]
> A unique ID will be generated for the `Project` by slugifying `Name` (reusing the existing
> `slug()` helper in `internal/app/service.go`, matching today's `AddProject` behavior).
> If the slug collides with an existing project in the profile, the action surfaces the
> corresponding typed error and the user is asked to pick a different `Name`.

- `Name`
    - label: name
    - description: The name of the project
    - required: `true`
- `Path`
    - label: path
    - description: The path of the project
    - required: `true`
- `EnabledAgents`
    - label: enabled agents
    - description: Multi-select of agents to enable for this project
      (`codex`, `claude-code`, `cursor`, `opencode`). At least one required.
    - required: `true`

The asset selection is **not** collected here — projects are registered with no assets
selected; the user picks them later from the [Select Project Assets Screen](#select-project-assets-screen).

If the form is submitted a `Project` object is returned (with `SelectedAssetIDs` empty).

### Create File Modal

When the _Create File_ modal is opened it shows a _form modal_ with _1_ form group
containing the following fields:

- `path`
    - label: `Path`
    - description: File path relative to the _asset_ folder
    - required: `true`

If the form is submitted the `path` value is returned.

### Create Profile Modal

When the _Create Profile_ modal is opened it shows a _form modal_ with _1_ form group
containing the following fields:

- `name`
    - label: `Name`
    - description: `Display name for the profile`
    - required: `true`
- `path`
    - label: `Path`
    - description: Profile directory path. ~ is expanded.
    - required: `true`

If the form is submitted the [Create Profile](#create-profile) action is invoked
and the resulting `Profile` is returned.

### Register Profile Modal

When the _Register Profile_ modal is opened it shows a _form modal_ with _1_ form group
containing the following field:

- `Path`
    - label: `path`
    - description: Profile directory path. ~ is expanded.
    - required: `true`

If the form is submitted the [Register Profile](#register-profile) action is invoked
and the resulting `Profile` is returned

### Notifications Modal

The Notifications Modal shows all `Notification`s that were created during the current session
in temporal order (newest first) in a _bubbles_ `Table` component (see below)

```
╭───────────────╮
│ Notifications │
╰───────────────╯

┌────────────────────────────────────────────────────────────────────────────────────┐
│ level  content                                                           time      │
│────────────────────────────────────────────────────────────────────────────────────│
│ INFO   Profile "hello" created successfully.                             13:04:42  │
│ ERROR  Cannot register profile "/af/hello"                               13:05:15  │
└────────────────────────────────────────────────────────────────────────────────────┘
```

## Service API

The following functions need to exist on `app.Service`. All calls are **stateless**:
callers pass `profileRef` explicitly (matches the current pattern). Functions return
`(T, errs.DomainError)` (or just `errs.DomainError` for void operations).

### Existing

- `CreateProfile(name, path string) (*registry.ProfileRef, errs.DomainError)`
- `RegisterProfile(path string) (*registry.ProfileRef, errs.DomainError)`
- `LoadProfile(ref string) (*profile.Profile, errs.DomainError)`
- `AddProject(profileRef, name, path string, agents, assetIDs []string) (*project.Manifest, []errs.DomainError)`
- `InitAsset(profileRef string, manifest asset.Manifest) (string, errs.DomainError)`
- `Plan(profileRef, projectID string) (*llmsync.Preview, errs.DomainError)`
- `Apply(profileRef, projectID string, deleteCandidates bool) (*llmsync.Preview, errs.DomainError)`

### New

- `LoadProfiles() ([]*profile.Profile, errs.DomainError)` — returns every registered profile
  fully loaded (assets + projects scanned). Heavy operation; the TUI caches the result for the
  Profiles Screen lifetime.
- `DeleteProfile(profileRef string, folderAction FolderAction) errs.DomainError` — deletes the
  profile from the registry plus all associated assets and projects. `folderAction` controls
  whether the on-disk profile folder is also removed (`DeleteFolders`) or kept (`KeepFolders`,
  default).
- `LoadAsset(profileRef, assetID string) (*asset.Asset, errs.DomainError)`
- `UpdateAsset(profileRef string, a *asset.Asset) errs.DomainError` — overwrites the asset
  manifest and recomputes content hashes.
- `DeleteAsset(profileRef, assetID string) errs.DomainError` — removes the asset folder inside
  the profile and unselects the asset id from every project in the profile. Already-synced
  files in project repos are **not** touched (they remain orphaned).
- `LoadProject(profileRef, projectID string) (*project.Manifest, errs.DomainError)`
- `UpdateProject(profileRef string, p *project.Manifest) errs.DomainError` — overwrites the
  project manifest.
- `DeleteProject(profileRef, projectID string) errs.DomainError` — removes project metadata
  only; the project's repo files are **not** touched.

### FolderAction

```go
type FolderAction int

const (
    KeepFolders   FolderAction = iota // default
    DeleteFolders
)
```

## Actions Reference

The following is the list of actions available to this app. Each action wraps one
function in the [Service API](#service-api) above.

### Create Profile

This _action_ creates a new `Profile` using the `Service` by invoking the `CreateProfile` function.

Parameters:

- `name`: mandatory
- `path`: mandatory

### Register Profile

This _action_ registers an existing `Profile` using the `Service` by invoking the `RegisterProfile` function.

Parameters:

- `path`: mandatory

### Load Profiles

This _action_ loads the available `Profile` objects from the `Service` using the `LoadProfiles` function.

Parameters: none

### Load Profile

This _action_ loads a `Profile` using the given `id` from the `Service` using the `LoadProfile` function.

Parameters:

- `id`: mandatory

### Delete Profile

This _action_ deletes the `Profile` with the given `id` using the `DeleteProfile` function.

> [!IMPORTANT]
> This will also delete all the **assets** and **projects** associated with
> this profile but it will not delete the physical files on the filesystem by default
> The command should accept an optional `FolderAction` enum an optional parameter
> that has 2 values: `DeleteFolders`, `KeepFolders`. Default is `KeepFolders`

Parameters:

- `id`: mandatory

### Register Project

This _action_ registers a `Project` using the `Service` by invoking the `AddProject` function.

Parameters: `Project` object

### Load Project

This _action_ loads a `Project` using the given `id` from the `Service` using the `LoadProject` function.

Parameters:

- `id`: mandatory

### Update Project

This _action_ overwrites the `Project` with the given `id` using the `UpdateProject` function.

> [!IMPORTANT]
> this will _overwrite_ the previous metadata.

Parameters:

- the updated `Project` object

### Delete Project

This _action_ deletes the `Project` with the given `id` using the `DeleteProject` function.

> [!IMPORTANT]
> This will only delete the **metadata** for the project, not the actual
> project directory where we synchronize the agent files!

Parameters:

- `id`: mandatory

### Plan Project

This _action_ creates a sync plan for the `Project` with the given `id` using the `Plan` function.

> [!IMPORTANT]
> `sync.Plan` must be extended to emit a new `ChangeUnknown` kind in addition to the existing
> `create`/`update`/`drift`/`delete` kinds. An "unknown" entry is a file that lives inside a
> managed surface (`AGENTS.md`, `.claude`, `.cursor`, `.codex`, `.opencode`, `.mcp.json`) but
> is **not** recorded in `ManagedState.ManagedFiles` and is **not** part of the current desired
> output.
>
> The current `detectDeleteCandidates` pass must be split:
>
> - file recorded in `ManagedState` and missing from desired → `ChangeDelete` (auto-handled)
> - file present in managed surface, not in `ManagedState`, not in desired → `ChangeUnknown`
>   (user decides via Plan Project Screen)
>
> **First-apply policy (no `ManagedState`):** treat the project as a clean slate.
> Every desired file is classified as `ChangeCreate` (and will be written, overwriting any
> existing file at the same path). No `ChangeUnknown` entries are emitted; existing files in
> managed surfaces are ignored. The first successful apply writes the initial `ManagedState`,
> after which subsequent plans can distinguish drift from unknown.

Parameters:

- `id`: mandatory

### Sync Project

This _action_ applies a previously created sync plan for the `Project` using the `Apply` function.

> [!IMPORTANT]
> Significant change to `Apply`'s signature. During the planning phase the user selects
> resolutions for each file that is not an automatic change:
>
> - "drift": previously managed file was modified locally.
>     - `ResolveOverwrite`: overwrite the local changes
>     - `ResolveKeep` (default): leave file alone
> - "unknown": unrecognized file in a managed surface that we never managed.
>     - `ResolveDelete`: delete the file
>     - `ResolveKeep` (default): leave file alone
>
> "create", "update" and "delete" carry `ResolveAuto` and are always materialized.

`sync.Apply` signature:

```go
type Resolution int

const (
    ResolveAuto      Resolution = iota // create / update / delete — always applied
    ResolveOverwrite                    // drift only
    ResolveKeep                         // drift or unknown — no-op
    ResolveDelete                       // unknown only
)

type FileResolution struct {
    Path       string
    Resolution Resolution
}

func Apply(preview *Preview, resolutions []FileResolution) errs.DomainError
```

The UI emits a `FileResolution` entry only when the user **toggles** away from the default
(`Keep`). Paths absent from `resolutions` keep the default behavior for their `ChangeKind`.

Parameters:

- `plan`: the plan that was created, mandatory
- `resolutions`: slice of `FileResolution`, may be empty

### Load Asset

This _action_ loads a `Asset` using the given `id` from the `Service` using the `LoadAsset` function.

Parameters:

- `id`: mandatory

### Create Asset

This _action_ creates an `Asset` by calling `InitAsset` with the given `Asset.Manifest`.

Parameters:

- an `Asset.Manifest` object.

### Update Asset

This _action_ overwrites the `Asset` with the given `id` using the `UpdateAsset` function.

> [!IMPORTANT]
> `UpdateAsset` _overwrites_ the previous manifest AND re-walks the asset directory to
> refresh per-file SHA values. Callers therefore do not need to reload the asset before
> calling this action even if only file contents (not metadata) changed.

Parameters:

- the updated `Asset` object

### Delete Asset

This _action_ deletes the `Asset` with the given `id` using the `DeleteAsset` function.

> [!IMPORTANT]
> this function will delete the asset folder within the profile, and remove
> the asset from the projects that are using it **but** it will not remove the files
> generated into the project directory (they will remain orphaned).

Parameters:

- `id`: mandatory
