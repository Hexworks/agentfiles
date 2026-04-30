---
name: af.task.implement
description: Use when the user invokes /implement-task <task-number> (e.g. /implement-task 0001) to plan and implement a task from the project's tasks/ folder. Locates the task directory, validates frontmatter, asks clarifying questions, writes a plan inside the task directory, awaits approval, implements the work, then writes a changelog entry. Project-specific to repos that follow the tasks/{backlog,current,done}/ convention with directory-per-task layout.
---

# Implement Task

End-to-end workflow that takes a task id (e.g. `0001`), locates it in `tasks/`, validates it, plans the work inside the task directory, gets human approval, implements it, and records a changelog in `docs/changelog/`.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # optional, written by this skill
    review.md        # optional, written by review-task skill
```

## Input

Single argument: the **task number** as a 4-digit string (e.g. `0001`, `0042`).

## Step 1 — Locate Task

Search `tasks/backlog/`, `tasks/current/`, `tasks/done/` for **directories** matching `{task-number}_*`.

| Found in         | Action                                    |
| ---------------- | ----------------------------------------- |
| `tasks/done/`    | Tell user task already done. **Stop.**    |
| `tasks/backlog/` | Tell user to refine task first. **Stop.** |
| (none)           | Tell user task not found. **Stop.**       |
| `tasks/current/` | Continue to Step 2.                       |

Extract `{task-type}` and `{short-description}` from the directory name: `{task-number}_{task-type}_{short-description}`.

The task body lives in `description.md` inside that directory.

## Step 2 — Validate Frontmatter

Read `description.md` inside the task directory. It must start with frontmatter:

```yaml
---
id: 0006
type: feature
status: pending
topics: research, go
---
```

If frontmatter missing → tell user, stop, let them fix.

Validate each field:

| Field    | Rule                                                                            | On failure         |
| -------- | ------------------------------------------------------------------------------- | ------------------ |
| `id`     | Equals `{task-number}` from directory name                                      | Signal error, stop |
| `type`   | One of `feature`, `bug`, `task`, `spike` AND equals `{task-type}` from dir name | Signal error, stop |
| `status` | One of `pending`, `in-progress`, `blocked`, `in-review`, `done`                 | Signal error, stop |
| `topics` | Non-empty                                                                       | Signal error, stop |

## Step 3 — Enter Plan Mode

Before any further work, if not already in plan mode, **enter plan mode** (`EnterPlanMode` tool). Planning happens in plan mode; implementation happens after the user approves the plan.

## Step 4 — Read Relevant Guidelines

For each entry in `topics`, read `docs/guidelines/{topic}.md`. **Always follow** this "must read" list of guidelines in the `docs/guidelines` folder: `clean_architecture.md`, `clean_code.md`, `domain_model.md`, `solid.md`, `testing.md`.
**Do not** read guideline files for topics that are not listed and aren't in the "must read" list. Apply that knowledge to the plan.
**Make sure** that you write tests too, not just application code.

## Step 5 — Branch + Clean Working Tree

Run `git status --porcelain`. If output non-empty → signal error, ask user to clean up, stop.

Create branch from directory name: `{task-type}/{short-description}`. Example: `0004_task_create-this-and-that` → branch `task/create-this-and-that`.

```bash
git checkout -b {task-type}/{short-description}
```

## Step 6 — Build Context

Before asking clarifying questions:

1. Read `docs/architecture/` to understand current architecture.
2. Read source files relevant to the task. **Important:** **You must** search for links to the task at hand in the source files. These links exist in documentation comments such as `// FIX: fix this thing @see task#0003`. The part you should look for is `@see {task-type}#{task-id}`, example: `@see feature#0017`
3. Note coding patterns relevant to the task.
4. If context sufficient → record what was learned. If not → list specific gaps for Step 7.

## Step 7 — Clarifying Questions

Only if real gaps exist after Step 6.

Rules:

- **One question at a time.**
- Wait for user's satisfactory response before asking the next.
- Only ask what you **need** to implement the task.
- Don't ask what the task or docs already answer.

## Step 8 — Record Q&A in description.md

Append every question + answer pair to `description.md` under a `## Clarification` section (create if absent):

```md
## Clarification

### Question

{question text}

### Answer

{answer text}
```

## Step 9 — Set Status to in-progress

Edit `description.md` frontmatter: `status: in-progress`.

## Step 10 — Write the Plan

Plan file path: `tasks/current/{task-id}_{task-type}_{short-description}/plan.md`.

**Important**: if `plan.md` already exists ask the user to review it.

1. If the user approves continue with the next step and skip to step 12 (Implementation)

Use subagents wherever applicable (especially `type: spike` or `topics: research`).

The plan file must:

- Cross-link to `description.md` with a relative link (`./description.md`).
- Cross-link to the relevant docs file (for example if an ADR was implemented in a task)
- Include a step-by-step execution plan.
- Note any ADRs that will be created/updated.
- Note any documentation that will be updated.
- Note any new/updated files in `docs/guidelines/`.

In `description.md`, add a link to `plan.md` (e.g. under a `## Plan` section with `[plan.md](./plan.md)`).

## Step 11 — Request Approval, Iterate

Present the plan and ask the user to review. Loop:

1. User asks for changes → update `plan.md` → ask for confirmation.
2. Repeat until user **approves**.

Do not implement until explicit approval.

## Step 12 — Implement

After approval, exit plan mode and implement per the plan. Follow the plan's step order; don't skip.

While implementing:

- **Architecture changes** → create or update an ADR in `docs/adr/` **if applicable**.
- **Doc-affecting changes** → update relevant files under `docs/`.
- **New patterns or explicit user request** → create/modify `docs/guidelines/{topic}.md`.

## Step 13 — Set Status to in-review

Edit `description.md` frontmatter: `status: in-review`.

## Step 14 — Write Changelog

Path: `docs/changelog/{YYYY-MM-DD}_{task-id}-{short-description}.md`.

`{YYYY-MM-DD}` is today's date (UTC). Example: `docs/changelog/2026-04-27_0004-global-config-refactor.md`.

Use the template at [`./changelog-template.md`](./changelog-template.md). Read it, fill in placeholders, write to the path above.

## Step 15 — Conclusion

Summarize work done. Provide links to:

- The task directory (`tasks/current/{task-id}_{task-type}_{short-description}/`)
- `description.md` and `plan.md` inside it
- The changelog file (`docs/changelog/{YYYY-MM-DD}_{task-id}-{short-description}.md`)
- Any new/updated ADRs, guidelines, or architecture docs.

Tell user the task is complete.

## Notes on Task Body Conventions

`description.md` may contain GitHub-style alerts. Treat them as guidance:

| Block            | Treat as                                         |
| ---------------- | ------------------------------------------------ |
| `> [!NOTE]`      | Useful info — read but don't act on it as a step |
| `> [!TIP]`       | Optimization advice — apply if reasonable        |
| `> [!IMPORTANT]` | **Must** incorporate into the plan               |
| `> [!WARNING]`   | Flag in plan, plan around it                     |
| `> [!CAUTION]`   | Risk — call out in plan and confirm with user    |

Sections under `##` headings in the task body are **steps**. Execute them in file order; never skip; pause at any step requiring human input until the human responds.

## Stopping Conditions Summary

| Condition                 | Response                            |
| ------------------------- | ----------------------------------- |
| Task in `done/`           | Inform user, stop                   |
| Task in `backlog/`        | Tell user to refine first, stop     |
| Task not found            | Inform user, stop                   |
| Missing frontmatter       | Tell user to fix, stop              |
| Invalid frontmatter field | Signal specific error, stop         |
| Dirty working tree        | Tell user to clean up, stop         |
| Plan not yet approved     | Wait for approval, do not implement |
