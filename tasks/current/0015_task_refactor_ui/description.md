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

An *action* is a function that invokes a backend function. It can have parameters, but it has no dependencies.
This differentiates an *action* from a service that we have to instantiate then refer to the instance variable.

*action*s behind the scene will use a global hard-coded service instance, but they hide this detail from the
call site. An *action* is a function that just invokes a specific business function in a service. They return
domain errors that we'll add to the error log when present (see below).

*action*s can have either no parameters or `1` parameter. It is the *action*'s job to transform this into a format
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

- Whenever an *action* returns with an entry is added to the `Notifications` list with `ERROR` level
  containing the error message as `Text`
- Whenever an *action* executes successfully an entry is added to `Notifications` with `INFO` level
  and with a `Text` that describes what action was executed successfully.

`Notification`s also show up in the *notification area* on each screen when created then removed
after `5 seconds`.

`Notification`s can be viewed by opening the [Notifications Modal](#notifications-modal)

## Components

### Mnemonic buttons

Mnemonic buttons are UI elements that appear as a button: `[Button]` and can be activated with a mnemonic.
This means that pressing a specific key will activate the *action* that is bound to the button.

The implementation should go into the `internal/tui/components` folder (similar to Modal)

Mnemonic buttons have the following parameters that can be passed when created:
- `label`: The label of the button: `[{{label}}]`
- `mnemonic`: the key that will - when pressed - execute the *action*. For example `a` (must be 1 character).
- `action`: The *action* (see above) that we'll execute when the button is pressed
- `data`: (optional) this is the parameter that we'll pass to the *action*.

Each screen can have any number of *Mnemonic buttons* but the `mnemonic` has to be unique for the screen
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

We are using *mnemonic buttons* (see above). *mnemonic buttons* need to be unique, this is another reason why we
only display them only on the selected + focused row.


#### Scrolling

`bubbles`' table component supports scrolling, all tables we use should have a fixed size depending on the available screen
real estate.


#### Highlight and navigation

In all tables we navigate with:
- ↑/k up    -> move up
- ↓/j down  -> move down

and the currently selected item should be visually distinct. bubbles supports this.


## Screens

Each screen is opened by calling a function that constructs the screen.
These functions can accept parameters when called (usually identifiers) for
loading the data for the screen. These are documented (see below).

_Note that_ on **all screens** there is a status bar with the current available
key bindings. This is supported by _bubbletea_ applications out of the box.

Key bindings that are _always available_:

- n notifications -> this opens a modal that shows the application log
- s settings      -> this opens the settings screen
- q quit          -> quits the app

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

- ↑/k up    -> move up in the list
- ↓/j down  -> move down in the list
- enter/v   -> choose currently selected list item
- n         -> show notifications
- s         -> go to settings screen
- q         -> quit the app


```
╭────────────╮
│ Agentfiles │
╰────────────╯

┃ Choose a task
┃ > Profiles
┃   Settings
┃   Done

{{ notification area (no content == invisible by default) }}

↑/k up • ↓/j down • enter/o choose • n notifications • s settings • q quit
```

### Profiles Screen

When the screen is loaded `Profiles` are loaded using the [LoadProfiles](#load-profiles) *Action*.

We use the bubbles table component on this screen. 
When a profile is selected in the table we add 2 *mnemonic buttons*
- Pressing `v` loads the [Profile View Screen](#profile-view-screen), using the selected `Profile`'s id as parameter.
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
│ test        Test        /Users/addamsson/af/profiles/   [View] [Delete]            │ <-- selected row
│ ...                                                                                │
└────────────────────────────────────────────────────────────────────────────────────┘

{{ notification area (no content == invisible by default) }} 

↑/k up • ↓/j down • c create • r register • n notifications • s settings • q quit
```
 
### Profile View Screen

Parameters: the `id` of the `Profile`

When the Profile View Screen is opened we load the `Profile` with the `id` that is passed to this screen
using the [LoadProfile](#load-profile) *action*.


On the Profile View Screen there are 2 tables. *Focus* can be shifted between the tables using the
`<tab>` key (forwards) or `<shift>+<tab>` (backwards) and with the mnemonic focus keys `1` and `2`.

#### Assets

The assets table lists all the `Asset` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:
- Pressing `v` (mnemonic) will open [View Asset Screen](#view-asset-screen) with the `id` of the selected `Asset`
- Pressing `d` (mnemonic) will open a [Confirmation](#confirmation) dialog with a command
  that deletes the selected `Asset`

#### Projects

The projects table lists all the `Project` objects within the loaded `Profile`.

The following context actions are available to selected rows in this table:
- Pressing `v` (mnemonic) will open [View Project Screen](#view-project-screen) with the `id` of the selected `Project`
- Pressing `d` (mnemonic) will open a [Confirmation](#confirmation) dialog with a command
  that deletes the selected `Project`

```
                                          ┌───────────────────────────┐                                         
                                          │Table caption displayed    │                                         
     ┌────────────────────────┐           │next to the focus mnemonic │                                         
     │Viewing {{profile-name}}│           │button                     │                                         
     └────────────────────────┘           └──┬────────────────────────┘                                         
                  ┌──────────────────────────┤                                                                  
┌────▶[1]─Assets─◀┴──────────────────────────┼─────────────────────────────────┐                                
│    │ Id       Name           Type          │             Actions             │                                
│    │───────────────────────────────────────┼─────────────────────────────────│                                
│    │ #1       AGENTS.md      agents_doc    │             [View] [Delete]     ◀──────────┐                     
│    │ #2       review-code    skill         │                                 │          │                     
│    │                                       │                                 │ ┌────────┴────────────────────┐
│    │                                       │                                 │ │Only one table can be        │
│    │                                       │                                 │ │focused, and only one row can│
│    └───────────────────────────────────────┼─────────────────────────────────┘ │be selected per table. We    │
│                     ┌──────────────────────┘                                   │only display mnemonic buttons│
│ ┌──▶[2]─Projects─◀──┴────────────────────────────────────────────────────────┐ │where a table is focused and │
│ │  │ Id       Name           Path                         Actions            │ │a row is selected.           │
│ │  │─────────────────────────────────────────────────────────────────────────│ └─────────────────────────────┘
│ │  │ #1       My project     /home/profiles/some          [View] [Delete]    │                                
│ │  │ #2       Other proj     /home/profiles/other                            │                                
│ │  │                                                                         │                                
│ │  │                                                                         │                                
│ │  │                                                                         │                                
│ │  └─────────────────────────────────────────────────────────────────────────┘                                
│ │                                                                                                             
│ │   {{ notification area (no content == invisible by default) }} ▒▒▒▒▒▒▒▒▒▒▒▒▒                                
│ │   ↑/k up • ↓/j down • b░back░• n notifications • s settings • q quit░░░░░░░░                                
│ │                                                                                                             
│ │      ┌───────────────────────────┐                                                                          
└─┼──────┤Mnemonic shortcuts (1 and  │                                                                          
  └──────┤2) that focus the table,   │                                                                          
         │rendered on the table      │                                                                          
         │border                     │                                                                          
         └───────────────────────────┘                                                                          
```

### View Asset Screen

### View Project Screen

## Modals

Modals are layers on top of the existing UI with their own focus handling (see documentation).
Modals can return values when closed.

**All modals can be closed without action by pressing the `<esc>` key.

### Confirmation

The confirmation dialog opens a simple "Yes" / "No" (modal) dialog.

Parameters:

- `cmd`: a command that can be executed without a parameter. If you wrap
  an *action* that has a parameter then you need to bind the parameter first
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


### Create Profile Modal

When the *Create Profile* modal is opened it shows a *form modal* with *1* form group
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
and the resulting `Profile` is returned

### Register Profile Modal

When the *Register Profile* modal is opened it shows a *form modal* with *1* form group
containing the following field:
- `path`
  - label: `Path`
  - description: Profile directory path. ~ is expanded.
  - required: `true`

If the form is submitted the [Register Profile](#register-profile) action is invoked
and the resulting `Profile` is returned


### Notifications Modal

The Notifications Modal shows all `Notification`s that were created during the current session
in temporal order (newest first) in a *bubbles* `Table` component (see below)

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

This *action* creates a new `Profile` using the `Service` by invoking the `CreateProfile` function.

Parameters: 
- `name`: mandatory
- `path`: mandatory

### Register Profile

This *action* registers an existing `Profile` using the `Service` by invoking the `RegisterProfile` function.

Parameters: 
- `path`: mandatory


### Load Profiles

This *action* loads the available `Profile` objects from the `Service` using the `LoadProfiles` function.
> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

Parameters: none


### Load Profile

This *action* loads a `Profile` using the given `id` from the `Service` using the `LoadProfile` function.

Parameters:
- `id`: mandatory


### Delete Profile

This *action* deletes the `Profile` with the given `id` using the `DeleteProfile` function.
> [!NOTE] this function doesn't exist yet on `Service`, we need to add it.

Parameters:
- `id`: mandatory

