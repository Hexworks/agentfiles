---
id: 0023
type: feature
status: pending
topics: go, tui, charm
depends_on: 0019, 0021
---

# System modals: Notifications + Info

Wires the two non-form modals from `0015_task_refactor_ui/description.md`:
**Notifications Modal** and **Info Modal**. These are opened by the global
`n` and `?` key bindings registered by the shell (task 0021).

## 1. Notifications Modal

Shows every `Notification` entry from the in-memory ring buffer added in
task 0019, **newest first**, in a `bubbles/table` widget.

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

### Implementation

- A `modal.Content` implementation backed by `bubbles/table.Model`.
- Constructor accepts a `*notifications.Log` and pulls `Entries()` once on
  open (no live refresh; reopening pulls fresh data).
- Levels rendered with a small style helper: `INFO` neutral, `ERROR`
  red/bold.
- `time` column shows `HH:MM:SS`.
- `esc`/`q` close.

## 2. Info Modal wiring

`internal/tui/components/help/help.go` is the markdown-rendered viewport
modal already in place. This task wires it into the shell as the `?` global
key handler.

### Topic registry

A small map keyed by screen (or a single global topic for MVP) determines
which `.md` file in `docs/manual/` opens. For the MVP a single entry that
points to `docs/manual/overview.md` is acceptable; per-screen overrides can
be added as screens land.

Create `docs/manual/overview.md` with a short intro page if it does not
already exist (one paragraph + a list of global keys is enough).

## Shell integration

Update the shell from task 0021 so its global key handlers actually open
these modals instead of no-ops.

## Tests

- Notifications modal: snapshot of the rendered table for a fixed log
  (3 entries, mixed levels) — assert order is newest-first.
- Notifications modal: empty log renders a non-crashing "No notifications
  yet" body.
- Info modal: opening with a known `.md` file resolves and renders without
  error; non-existent file surfaces the existing typed error from
  `help/errors.go` inside the viewport (already covered by `help/help_test.go`
  — extend if needed).

## Out of scope

- Persistent storage of the notification log (defer to revived task 0016).
- Per-screen Info topic mapping beyond the global default — add as needed
  when screens land.

## Verification

```
make build && make test && make lint
./bin/af   # pressing n opens Notifications modal; ? opens Info modal
```
