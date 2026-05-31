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

*action*s behind the scene will use a global hard-coded service instance, but they hide this detail from the
call site. An _action_ is a function that just invokes a specific business function in a service. They return
domain errors that we'll add to the error log when present (see below).

*action*s can have either no parameters or `1` parameter. It is the _action_'s job to transform this into a format
that will be accepted by the business function.

*action*s need to integrate with the charm ecosystem seamlessly (see the related [guidelines](/docs/guidelines/charm.md)).

## Notifications

Notifications is a list of `Notification` entries. These only exist within the TUI.
Example:

```
{
    Level: "INFO",
    Text: "Profile 'hello' created successfully.",
    CreatedAt: "2026.01.05 15:13:35"
}
```

- Whenever an _action_ returns with an entry is added to the `Notifications` list with `ERROR` level
  containing the error message as `Text`
- Whenever an _action_ executes successfully an entry is added to `Notifications` with `INFO` level
  and with a `Text` that describes what action was executed successfully.

`Notification`s also show up in the _notification area_ on each screen when created then removed
after `5 seconds`.

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

### Modals

We'll use Modals (see more in the modal documentation on how they work), these are already implemented.

### Tables

We'll use `bubbles`' table component throughout the app. It already supports scrolling
so we don't need to implement that. Tables always have a fixed height so that they can fit on the screen.

#### Selection

When a row is selected (bubbles supports this) we need to display the actions that can be performed on
the selected item in the "Actions" column. We do this only on the selection to prevent visual noise.

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

## Screens

Each screen is opened by calling a function that constructs the screen.
These functions can accept parameters when called (usually identifiers) for
loading the data for the screen. These are documented (see below).

_Note that_ on **all screens** there is a status bar with the current available
key bindings. This is supported by _bubbletea_ applications out of the box.

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

When the app is started with `af` the user will land on this screen (see mockup below).

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
┃   Done

{{ notification area (no content == invisible by default) }}

↑/k up • ↓/j down • enter/o choose • n notifications • s settings • q quit • ? help
```

### Profiles Screen

When the screen is loaded `Profiles` are loaded using the [LoadProfiles](#load-profiles) _Action_.

We use the bubbles table component on this screen.
When a profile is selected in the table we add 2 _mnemonic buttons_

- Pressing `i` loads the [Edit Profile Screen](#edit-profile-screen), using the selected `Profile`'s id as parameter.
- Pressing `d` deletes the profile. It uses the confirmation modal (see below) to ask for confirmation.

Regardless of table selection

- Pressing `c` opens the [CreateNewProfile] modal. (see below).
- Pressing `r` opens the [RegisterProfile] modal (see below).

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

{{ notification area (no content == invisible by default) }}

↑/k up • ↓/j down • c create • r register • n notifications • s settings • q quit
```

### Edit Profile Screen

Parameters: the `id` of the `Profile`

When the Edit Profile Screen is opened we load the `Profile` with the `id` that is passed to this screen
using the [Load Profile](#load-profile) _action_.

On the Edit Profile Screen there are 2 tables. _Focus_ can be shifted between the tables using the
`<tab>` key (forwards) or `<shift>+<tab>` (backwards) and with the mnemonic focus keys `1` and `2`.

#### Assets

The assets table lists all the `Asset` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:

- Pressing `i` (mnemonic) will open [Edit Asset Screen](#edit-asset-screen) with the `id` of the selected `Asset`
- Pressing `d` (mnemonic) will open a [Confirmation](#confirmation-modal) dialog with a command
  that deletes the selected `Asset` using the [Delete Asset](#delete-asset) _action_

Below the _assets table_ there is a mnemonic button: "Create Asset". It is invoked by pressing `c`.

When "New Asset" is invoked the [Create Asset Modal](#create-asset-modal) is displayed. If it
isn't canceled an `Asset.Manifest` is returned and the [Create Asset](#create-asset) _action_
is invoked.

#### Projects

The projects table lists all the `Project` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:

- Pressing `l` (mnemonic) will open the [Select Project Assets Screen](#select-project-assets-screen) with the `id` of the selected `Project`
- Pressing `p` (mnemonic) will open the [Plan Project Screen](#plan-project-screen) with the `id` of the selected `Project`
- Pressing `d` (mnemonic) will open a [Confirmation](#confirmation-dialog) dialog with a command
  that deletes the selected `Project` using the [Delete Project](#delete-project) _action_

Below the _projects table_ there is a mnemonic button: "Register Project". It is invoked by pressing `r`.

When "Register Project" is invoked the [Register Project Modal](#register-project-modal) is displayed. If it
isn't canceled a `Project` is returned and the [Register Project](#register-project) _action_
is invoked.

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
│ │  │ #1       My project     /home/profiles/some                [Assets] [Plan] [Delete] │  │
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

The following mockup shows the Edit Asset Screen. There are 2 columns each occupying 50%
of the available horizontal space.

The heading occupies 10% of the vertical space.

> [!NOTE] that fields that have a list type (eg: `[]string`) will be joined when displayed, so
> `["foo", "bar"]` will be displayed as `foo, bar` and after editing (on `Blur()`) they will
> be transformed back to a list, so `foo, bar` turns into `["foo", "bar"]`. We split and join
> using `,` as separator and we strip whitespace (eg: `", "` -> `","`)

#### Files

In the left column there is a _treetable_ that shows the contents of the _asset_'s directory.
It occupies 50% of the available horizontal space, and 90% of the available vertical space.

Pressing `1` (focus handling mnemonic button) will focus the `treetable`.

Pressing `e` ("Edit" mnemonic button) opens the file for editing using the `fsutil/editor` functionality.
After the editor is closed we return to the screen.

> [!IMPORTANT]
> the "Edit" mnemonic button is only rendered for leaf nodes (files), not for directories

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

In the right column there is a _group_ named "Summary" that shows non-editable data. occupying
50% of the horizontal and 30% of the vertical space available.

Below it there is a _group_ named "Customize" that shows editable fields and it occupies
50% of the horizontal and 60% of the vertical space available.

Pressing `2` (focus handling mnemonic button) will focus the "description" field.

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
│ review-task/                            ▽           ││ Compatible Agents   {{asset.CompatibleAgents}}     │
│ ├── SKILL.md                                        ││ Exclusive Group     {{asset.ExclusiveGroup}}       │
│ ├── scripts/                                        ││                                                    │
│ │   └── git-commit.sh             [Edit]  [Delete]  ││                                                    │
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

The following mockup shows the Select Project Assets Screen. There are 2 tables below each other each occupying
40% of the available vertical space.

The heading occupies 10% of the vertical space.

There is a "Plan" button below the "Available Assets" table that will navigate to the [Plan Project Screen](#plan-project-screen).

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

The following mockup shows the Plan Project Screen.

The heading occupies 10% of the vertical space.

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
> An unique ID will be generated for the `Asset`

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
    - requited: `false`
- `ExclusiveGroup`
    - label: exclusive group
    - description: Assign (optional) exclusive group (eg: `agents_doc`)
    - required: `false`

If the form is submitted an `Asset.Manifest` object is returned.

### Register Project Modal

When the _Register Project_ modal is opened it shows a _form modal_ with _1_ form group
containing the following fields:

> [!IMPORTANT]
> An unique ID will be generated for the `Project`

- `Name`
    - label: name
    - description: The name of the project
    - required: `true`
- `Path`
    - label: path
    - description: The path of the project
    - required: `true`

If the form is submitted an `Project` object is returned.

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

## Actions Reference

The following is the list of actions available to this app.

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

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

Parameters: none

### Load Profile

This _action_ loads a `Profile` using the given `id` from the `Service` using the `LoadProfile` function.

Parameters:

- `id`: mandatory

### Delete Profile

This _action_ deletes the `Profile` with the given `id` using the `DeleteProfile` function.

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

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

### Update Project

This _action_ overwrites the `Project` with the given `id` using the `UpdateProject` function.

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

> [!IMPORTANT]
> this will _overwrite_ the previous metadata.

Parameters:

- the updated `Project` object

### Delete Project

This _action_ deletes the `Project` with the given `id` using the `DeleteProject` function.

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

> [!IMPORTANT]
> This will only delete the **metadata** for the project, not the actual
> project directory where we synchronize the agent files!

Parameters:

- `id`: mandatory

### Create Asset

This _action_ creates an `Asset` by calling `InitAsset` with the given `Asset.Manifest`.

Parameters:

- an `Asset.Manifest` object.

### Update Asset

This _action_ overwrites the `Asset` with the given `id` using the `UpdateAsset` function.

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

> [!IMPORTANT]
> this will _overwrite_ the previous metadata.

Parameters:

- the updated `Asset` object

### Delete Asset

This _action_ deletes the `Asset` with the given `id` using the `DeleteAsset` function.

> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

> [!IMPORTANT]
> this function will delete the asset folder within the profile, and remove
> the asset from the projects that are using it **but** it will not remove the files
> generated into the project directory (they will remain orphaned).

Parameters:

- `id`: mandatory
