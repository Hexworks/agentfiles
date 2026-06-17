---
name: cleanup-tasks
description: Use when the user invokes /cleanup-tasks (or asks to "clean up tasks", "move done tasks", "archive finished tasks") to scan the repo's tasks/ folder and relocate every task directory whose description.md frontmatter has `status: done` into tasks/done/. Project-specific to repos that follow the tasks/{backlog,current,done}/ convention with directory-per-task layout.
---

# Cleanup Tasks

Move task directories whose `description.md` declares `status: done` from `tasks/backlog/` or `tasks/current/` into `tasks/done/`.

## Behavior

Delegate the actual work to [`./cleanup-tasks.sh`](./cleanup-tasks.sh). The script:

1. Resolves the repository root via `git rev-parse --show-toplevel` (falls back to `pwd`).
2. Walks `tasks/backlog/*/` and `tasks/current/*/`.
3. Parses the YAML frontmatter of each `description.md` and reads the `status` field.
4. For any task with `status: done`, moves the whole task directory to `tasks/done/`.
5. Skips a task if a directory with the same name already exists in `tasks/done/` (logged to stderr).
6. Prints a `summary: moved=N skipped=M` line at the end.

The script is idempotent — re-running it after every task is moved is a no-op.

## Steps

1. Make the script executable if needed: `chmod +x .claude/skills/cleanup-tasks/cleanup-tasks.sh`.
2. Run it from the repository root:

   ```bash
   .claude/skills/cleanup-tasks/cleanup-tasks.sh
   ```

3. Report the script's stdout (the `moved:` lines and the `summary:` line) back to the user. If anything was sent to stderr (e.g. skip messages), surface those too.

## Stopping Conditions

| Condition                                | Response                                       |
| ---------------------------------------- | ---------------------------------------------- |
| `tasks/` does not exist at the repo root | Script exits non-zero; relay the error message |
| Target name already exists in `done/`    | Skip that task, continue with the rest         |
| `description.md` missing from a task dir | Skip that task, continue with the rest         |
