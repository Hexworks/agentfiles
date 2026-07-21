---
name: Implement Task [Agentfiles]
description: Use when the user invokes /implement-task <task-number> (e.g. /implement-task 0001) to implement a task whose plan has already been approved by `af.task.plan`. Re-locates the task, validates that status is `in-progress` and `plan.md` exists, checks out the task branch, rebuilds context, implements the plan, runs the mandatory build/test/lint gate, sets status to `in-review`, and writes a changelog entry. Assumes /plan-task has already produced plan.md and set status to `in-progress`. Project-specific to repos that follow the tasks/{backlog,current,done}/ convention with directory-per-task layout.
---

# Implement Task

Executes the plan produced by `af.task.plan`. Takes a task id (e.g. `0001`), validates it is in `in-progress` with an approved `plan.md`, rebuilds context in a clean session, implements per the plan, gates on build/test/lint, moves the task to `in-review`, and records a changelog in `docs/changelog/`.

This skill is the second half of the plan/implement workflow. Planning is done first by `af.task.plan` (via `/plan-task <id>`); this skill executes the approved plan.

> [!IMPORTANT]
> This skill **must be run with a clean context**. Do not chain it after other long-running work in the same session.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # written by af.task.plan
    review.md        # written later by af.task.review
```

## Input

Single argument: the **task number** as a 4-digit string (e.g. `0001`, `0042`).

## Step 1 — Locate Task

Search `tasks/backlog/`, `tasks/current/`, `tasks/done/` for **directories** matching `{task-number}_*`.

| Found in         | Action                                    |
| ---------------- | ----------------------------------------- |
| `tasks/done/`    | Tell user task already done. **Stop.**    |
| `tasks/backlog/` | Tell user to run `/plan-task {id}` first. **Stop.** |
| (none)           | Tell user task not found. **Stop.**       |
| `tasks/current/` | Continue to Step 2.                       |

Extract `{task-type}` and `{short-description}` from the directory name: `{task-number}_{task-type}_{short-description}`.

## Step 2 — Validate Handoff From /plan-task

Read `description.md`. Frontmatter must look like:

```yaml
---
id: 0006
type: feature
status: in-progress
topics: research, go
---
```

Then verify plan artefacts and working tree:

| Check                       | On failure                      |
| --------------------------- | ------------------------------- |
| Frontmatter present         | Signal error to user, **stop**. |
| `id` equals `{task-number}` | Signal error to user, **stop**. |
| `status` equals `in-progress` | Signal error to user, **stop**. Tell user to run `/plan-task {id}` first if status is `pending`. |
| `topics` non-empty          | Signal error to user, **stop**. |
| `plan.md` present in task dir | Tell user to run `/plan-task {id}` first. **Stop.** |
| Current git branch equals `{task-type}/{short-description}` | Tell user to `git checkout {task-type}/{short-description}`. **Stop.** |
| `git status --porcelain` empty | Tell user to commit or stash first. **Stop.** |

If `status` is anything other than `in-progress`, signal the error explicitly: e.g. _"Task 0004 is in `pending`, not `in-progress`. Run `/plan-task 0004` first to produce and approve a plan."_

## Step 3 — Rebuild Context

You are in a clean context, so re-gather what you need to implement correctly:

1. Read `description.md` full body, including any `## Clarification` Q&A.
2. Read `plan.md` full body.
3. For each entry in the task's `topics` field, read `docs/guidelines/{topic}.md`. **Always** also read the "must read" set: `clean_architecture.md`, `clean_code.md`, `domain_model.md`, `solid.md`, `testing.md`.
4. Read every doc the plan references or touches (ADRs under `docs/adr/`, arc42 sections under `docs/architecture/`, glossary entries).
5. Read in full each source file the plan will edit, so you understand surrounding context, naming, and existing abstractions.

## Step 4 — Implement

Follow the plan's step order; don't skip. While implementing:

- **Architecture changes** → create or update an ADR in `docs/adr/` **if applicable**.
- **Doc-affecting changes** → update relevant files under `docs/`.
- **New patterns or explicit user request** → create/modify `docs/guidelines/{topic}.md`.
- Update tests alongside production code; add new tests where a change alters observable behavior.
- If you discover the plan is wrong or a step is infeasible, **stop and tell the user** before deviating. Do not silently improvise.

## Step 5 — Quality Gate (Mandatory)

This gate is **mandatory**. If any check fails or produces a new warning, **stop** and report the failure to the user — do not touch status, do not write the changelog.

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

Only after **all four** items pass may you proceed to Step 6.

## Step 6 — Set Status to in-review

Edit `description.md` frontmatter: `status: in-review`.

## Step 7 — Write Changelog

Path: `docs/changelog/{YYYY-MM-DD}_{task-id}-{short-description}.md`.

`{YYYY-MM-DD}` is today's date (UTC). Example: `docs/changelog/2026-04-27_0004-global-config-refactor.md`.

Use the template at [`./changelog-template.md`](./changelog-template.md). Read it, fill in placeholders, write to the path above.

## Step 8 — Conclusion

Summarize work done. Provide links to:

- The task directory (`tasks/current/{task-id}_{task-type}_{short-description}/`)
- `description.md` and `plan.md` inside it
- The changelog file (`docs/changelog/{YYYY-MM-DD}_{task-id}-{short-description}.md`)
- Any new/updated ADRs, guidelines, or architecture docs.

Do **not** auto-commit. Leave staging to the user so they can group commits as they prefer. Tell the user the task is ready for review.

## Notes on Task Body Conventions

`description.md` may contain GitHub-style alerts. Treat them as guidance:

| Block            | Treat as                                         |
| ---------------- | ------------------------------------------------ |
| `> [!NOTE]`      | Useful info — read but don't act on it as a step |
| `> [!TIP]`       | Optimization advice — apply if reasonable        |
| `> [!IMPORTANT]` | **Must** incorporate into the implementation     |
| `> [!WARNING]`   | Flag and work around it                          |
| `> [!CAUTION]`   | Risk — call out and confirm with user            |

## Stopping Conditions Summary

| Condition                     | Response                                              |
| ----------------------------- | ----------------------------------------------------- |
| Task in `done/`               | Inform user, stop                                     |
| Task in `backlog/`            | Tell user to run `/plan-task {id}`, stop              |
| Task not found                | Inform user, stop                                     |
| Missing frontmatter           | Tell user to fix, stop                                |
| Invalid frontmatter field     | Signal specific error, stop                           |
| Status not `in-progress`      | Signal specific error, stop                           |
| `plan.md` missing             | Tell user to run `/plan-task {id}`, stop              |
| Wrong branch checked out      | Tell user to `git checkout` the task branch, stop     |
| Dirty working tree            | Tell user to commit or stash, stop                    |
| Quality gate fails            | Report failure, stop — do not touch status or write changelog |
