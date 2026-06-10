---
id: 0024
type: feature
status: pending
depends_on: 0021
---

# Welcome + Settings screens

Wires the two simplest screens from `0015_task_refactor_ui/description.md`:
**Welcome Screen** and **Settings Screen** (MVP stub). Together they form
the top-level navigation entry the rest of the screen tasks plug into.

## 1. Welcome Screen

```
╭────────────╮
│ Agentfiles │
╰────────────╯

┃ Choose a task
┃ > Profiles
┃   Settings
┃   Quit

{{ notification area }}

↑/k up • ↓/j down • enter/v choose • n notifications • s settings • q quit • ? help
```

### Behavior

- Vertical list with arrow / `k j` navigation.
- `enter` or `v` chooses the highlighted item.
- "Profiles" pushes the Profiles Screen (task 0025; until then, a stub
  placeholder is acceptable).
- "Settings" pushes the Settings Screen below.
- "Quit" exits the app.
- Replaces the placeholder Welcome stub installed by task 0021 as the
  initial route.

## 2. Settings Screen (MVP stub)

```
╭──────────╮
│ Settings │
╰──────────╯

 Coming soon.

                                                                                       [Back]

 {{ notification area }}

 b░back░•░n░notifications░•░q░quit░•░?░help
```

- Single line of text plus a `[Back]` mnemonic button (`b`).
- Pressing `b` (or `esc`) returns to the previous screen (Welcome Screen if
  reached via the menu; whichever screen was active if reached via the
  global `s` shortcut).

The Welcome Screen menu entry and the global `s` binding stay in place so
future work can flesh the Settings screen out without touching navigation.

## Status bar wiring

Both screens expose their static keys via the shell's status-bar contract
(task 0021). Welcome has no mnemonic buttons; Settings has the single
`[Back]` button — that should appear in the status bar **only** if the
parent task's rule places `[Back]` outside the "screen-level buttons not
duplicated" exception. Per the parent description, `b back` does appear in
the Settings status bar example, so include it.

## Tests

- Welcome: arrow navigation moves selection; `enter` dispatches the
  expected `PushScreenMsg` for "Profiles" / "Settings"; "Quit" dispatches a
  quit message.
- Settings: pressing `b` dispatches `PopScreenMsg`.

## Out of scope

- The actual Profiles screen body (task 0025).
- Real Settings content — that is intentionally deferred.

## Verification

```
make build && make test && make lint
./bin/af   # lands on Welcome; menu reaches Settings stub and back
```
