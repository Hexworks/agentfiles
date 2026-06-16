# Notifications

Persistent log of every notification the shell has emitted since launch:
results of mutations (create/delete/sync), warnings, and errors. The modal
opens over whichever screen you were on and closes back to it.

## Operations

- `↑`/`k`, `↓`/`j` — scroll the log.
- `esc` — close the modal and return to the previous screen.
- `q` / `ctrl+c` — quit the application (the global quit binding stays live
  while the modal is open).

## Notes

Toasts you see at the bottom of the screen also land here, so the log is the
place to re-read a message that has already expired.
