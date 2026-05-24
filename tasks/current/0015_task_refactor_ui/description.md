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

At the bottom there is the statusbar that shows the possible key bindings

```
┌───────────────────────────────────────────────────────────────────────────┐
│ ┌──────────┐                                                              │
│ │Agentfiles│                                                              │
│ └──────────┘                                                              │
│ ┃ Choose a task                                                           │
│ ┃ > Profiles                                                              │
│ ┃   Settings                                                              │
│ ┃   Quit                                                                  │
│                                                                           │
│                                                                           │
│ ↑/k up • ↓/j down • enter submit • s settings • q quit                    │
└───────────────────────────────────────────────────────────────────────────┘
```

## Actions

### Load Profiles

This actions loads all profiles and returns them to be displayed on the profiles screen
