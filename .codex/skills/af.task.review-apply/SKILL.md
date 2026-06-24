---
name: Apply Task Review [Agentfiles]
description: Use when the user invokes /apply-review <task-number> (e.g. /apply-review 0001) to apply the fixes they selected in a task's `review.md`. The review file is produced by the separate `af.task.review` skill; this skill reads the task's selected solution checkboxes, implements the chosen fixes, runs the build/test/lint quality gate, and commits the changes. The task must be in `in-review` state with a `review.md` whose every issue block has exactly one ticked checkbox.
---

# Apply Task Review

Applies the fixes a user selected in `review.md`. Takes a task id (e.g. `0001`), validates it is in `in-review`, re-reads the review file and the task context, verifies the user has chosen exactly one solution per issue, implements the chosen fixes, gates on build/test/lint, and commits.

This skill is the second half of the review workflow. The review document is produced first by `af.task.review`; the user then ticks the checkboxes; this skill applies the selections.

> [!IMPORTANT]
> This skill **must be run with a clean context**. Do not chain it after other long-running work in the same session.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # written by implement-task
    review.md        # written by af.task.review
```

## Input

Single argument: the **task number** as a 4-digit string (e.g. `0001`, `0042`).

## Step 1 — Locate Task

Search `tasks/backlog/`, `tasks/current/`, `tasks/done/` for **directories** matching `{task-number}_*`.

| Found in         | Action                                    |
| ---------------- | ----------------------------------------- |
| `tasks/done/`    | Tell user task already done. **Stop.**    |
| `tasks/backlog/` | Tell user task not implemented. **Stop.** |
| (none)           | Tell user task not found. **Stop.**       |
| `tasks/current/` | Continue to Step 2.                       |

Extract `{task-type}` and `{short-description}` from the directory name: `{task-number}_{task-type}_{short-description}`.

## Step 2 — Validate Status

Read `description.md` inside the task directory. Frontmatter must look like:

```yaml
---
id: 0006
type: feature
status: in-review
topics: research, go
---
```

| Check                       | On failure                      |
| --------------------------- | ------------------------------- |
| Frontmatter present         | Signal error to user, **stop**. |
| `id` equals `{task-number}` | Signal error to user, **stop**. |
| `status` equals `in-review` | Signal error to user, **stop**. |
| `topics` non-empty          | Signal error to user, **stop**. |

If `status` is anything other than `in-review`, signal the error explicitly: e.g. _"Task 0004 is in `in-progress`, not `in-review`. Cannot apply review fixes until implementation is finished."_

## Step 3 — Read the Review File

Path: `tasks/current/{task-id}_{task-type}_{short-description}/review.md`.

| Condition           | Action                                                                         |
| ------------------- | ------------------------------------------------------------------------------ |
| `review.md` missing | Tell user to run `af.task.review {task-number}` first to produce it. **Stop.** |
| `review.md` present | Read it in full. Continue.                                                     |

Verify selections: **every** `## {issue}` block must have **exactly one** `[x]` checkbox in its solution checklist.

| Condition                          | Action                                                                  |
| ---------------------------------- | ----------------------------------------------------------------------- |
| Every block has exactly one `[x]`  | Continue to Step 4.                                                     |
| One or more blocks have zero `[x]` | List the offending issues by title, ask the user to choose, **stop**.   |
| Any block has more than one `[x]`  | List the offending issues by title, ask the user to pick one, **stop**. |

## Step 4 — Rebuild Context

You are in a clean context, so re-gather what you need to implement the fixes correctly:

1. Read `description.md` (full body, including any `## Clarification` Q&A) and `plan.md` if present.
2. For each entry in the task's `topics` field, read `docs/guidelines/{topic}.md`. Also read any guideline the review's warning callouts link to.
3. Read every doc (ADR under `docs/adr/`, arc42 sections under `docs/architecture/`, glossary entries) that the chosen fixes touch.
4. Inspect the current state of the code:

```bash
git status
git diff
git diff --stat
```

Read in full each file a chosen fix will touch, so you understand surrounding context, naming, and existing abstractions.

## Step 5 — Implement Fixes (Iterate)

For each issue, implement the **single ticked** solution. While implementing:

- If the chosen solution is ambiguous or you discover a conflict with another fix, **ask the user** before proceeding. Do not guess.
- Group related edits per file to keep the diff readable.
- Update tests alongside production code; add new tests where a fix changes observable behavior.

After each round of edits, summarize what changed and ask the user to confirm. Loop:

1. User asks for adjustments → revise → ask again.
2. Repeat until the user **approves**.

Do not continue to the quality gate without explicit approval.

## Step 6 — Quality Gate (Mandatory)

This gate is **mandatory**. If any check fails or produces a new warning, **stop** and report the failure to the user — do not commit.

Run, in order:

```bash
make fmt
make lint
make build
make test
```

The gate validates four checklist items at once:

- _My changes generate no new warnings_
- _I have added/updated tests where necessary_
- _New and existing tests pass locally_
- _Static analysis / linters pass_

Only after **all four** items pass may you proceed.

## Step 7 — Commit the Changes

Follow `docs/guidelines/git.md` for commit message style. Stage only the files touched during the review fix-up — do not bulk-add unrelated work.

After committing:

- Run `git log -1 --stat` and display the commit hash, subject, and changed-file summary to the user.
- Tell the user the review fixes are complete and committed.

Do **not** push, and do **not** change the task's `status` — leaving it in `in-review` is intentional so the user can decide when to mark it `done`.

## Stopping Conditions Summary

| Condition                            | Response                                |
| ------------------------------------ | --------------------------------------- |
| Task in `done/`                      | Inform user, stop                       |
| Task in `backlog/`                   | Inform user (not implemented yet), stop |
| Task not found                       | Inform user, stop                       |
| Missing frontmatter                  | Tell user to fix, stop                  |
| Status not `in-review`               | Signal specific error, stop             |
| `review.md` missing                  | Tell user to run `af.task.review`, stop |
| Review block with zero or many `[x]` | Ask user to pick exactly one, stop      |
| Quality gate fails                   | Report failure, stop — do not commit    |
