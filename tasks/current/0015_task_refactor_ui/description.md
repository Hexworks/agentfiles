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

## Screens

_Note that_ on **all screens** there is a status bar with the current available
key bindings. This is supported by _bubbletea_ applications out of the box.

### Welcome Screen

When the app is started with `af` the user will land on this screen.

There is a title at the top ("Agentfiles") followed by a list of options:

- "Profiles" navigates to the _Profiles Screen_ (see below)
- "Settings" navigates to the _Settings Screen_ (see below)
- "Quit" exits the app

At the bottom there is the statusbar that shows the possible key bindings:

- arrow up or k -> move up in the list
- arrow down or j -> move down in the list
- enter -> choose currently selected list item
- s -> go to settings screen
- q -> quit the app

```
┌────────────────────────────────────────────────────────────┐
│ ┌──────────┐                                               │
│ │Agentfiles│                                               │
│ └──────────┘                                               │
│ ┃ Choose a task                                            │
│ ┃ > Profiles                                               │
│ ┃   Settings                                               │
│ ┃   Quit                                                   │
│                                                            │
│                                                            │
│ ↑/k up • ↓/j down • enter submit • s settings • q quit     │
└────────────────────────────────────────────────────────────┘
```

### Profiles Screen

```
╭──────────╮
│ Profiles │
╰──────────╯

┌──────────────────────────────────────────────────────────────
│ ID          Name        Path                            Last opened
│──────────────────────────────────────────────────────────────
│ another     Another     /Users/addamsson/af/another     2026-05-24 16
│ test        Test        /Users/addamsson/af/profiles/   2026-05-24 16
│
└─────────────────────────────────────────────────────────────────────────

↑/k up • ↓/j down • q quit
```

## Actions Reference

The following is the list of actions available to this app.

### Load Profiles

This actions loads all profiles and returns them to be displayed on the profiles screen.
