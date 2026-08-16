---
name: Plan Task [Agentfiles]
description: Use when the user invokes /plan-task <task-number> (e.g. /plan-task 0001) to plan a task from the project's tasks/ folder ahead of implementation. Locates the task directory, validates frontmatter, creates the working branch, gathers context, asks clarifying questions, writes a plan inside the task directory, and iterates with the user until approval. Sets status to `in-progress` only after the plan is approved. Next step: user runs /implement-task. Project-specific to repos that follow the tasks/{backlog,current,done}/ convention with directory-per-task layout.
---

# Plan Task

Produces a reviewed, approved `plan.md` for a task. Takes a task id (e.g. `0001`), locates it in `tasks/current/`, validates it, sets up the working branch, gathers context, asks clarifying questions, writes the plan, and iterates until the user approves.

Implementation is a **separate** skill (`af.task.implement`, invoked via `/implement-task <id>`). This skill never touches production code.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # written by this skill
    review.md        # written by af.task.review
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
| `status` | One of `pending`, `active`, `blocked`, `in-progress`, `in-review`, `done`       | Signal error, stop |
| `topics` | Non-empty                                                                       | Signal error, stop |

If `status` is already `in-review` or `done` → tell user the task is past planning and stop.

If `status` is already `in-progress` and `plan.md` exists → jump to Step 8 (re-planning branch).

## Step 3 — Read Relevant Guidelines

For each entry in `topics`, read `docs/guidelines/{topic}.md`. **Always follow** this "must read" list of guidelines in the `docs/guidelines` folder: `clean_architecture.md`, `clean_code.md`, `domain_model.md`, `solid.md`, `testing.md`.

**Do not** read guideline files for topics that are not listed and aren't in the "must read" list. Apply that knowledge to the plan.

**Make sure** that the plan includes tests, not just application code.

## Step 4 — Branch + Clean Working Tree

Run `git status --porcelain`. If output non-empty → signal error, ask user to clean up, stop.

Target branch name is derived from the task directory: `{task-type}/{short-description}`. Example: `0004_task_create-this-and-that` → branch `task/create-this-and-that`.

Handle the branch in this order:

1. If already on the target branch → continue.
2. Else if the target branch exists locally → `git checkout {target}`.
3. Else → `git checkout -b {target}`.

## Step 5 — Build Context

Before asking clarifying questions:

1. Read `docs/architecture/` to understand current architecture.
2. Read source files relevant to the task. **Important:** **You must** search for links to the task at hand in the source files. These links exist in documentation comments such as `// FIX: fix this thing @see task#0003`. The part you should look for is `@see {task-type}#{task-id}`, example: `@see feature#0017`.
3. Note coding patterns relevant to the task.
4. **Ground every external assumption.** Any literal string, id scheme, on-disk layout, schema key, path pattern, protocol constant, or wire format the plan will encode must be traced back to the function that defines it. Do not guess from documentation or from an existing similar-looking string elsewhere — open the source. Record the source `file:line` and the exact line for each assumption. This is the input to the mandatory `## Assumption grounding` table (Step 8).
5. If context sufficient → record what was learned. If not → list specific gaps for Step 6.

## Step 6 — Clarifying Questions

Only if real gaps exist after Step 5.

Rules:

- **One question at a time.**
- Wait for the user's satisfactory response before asking the next.
- Only ask what you **need** to plan the task.
- Don't ask what the task or docs already answer.

## Step 7 — Record Q&A in description.md

Append every question + answer pair to `description.md` under a `## Clarification` section (create if absent):

```md
## Clarification

### Question

{question text}

### Answer

{answer text}
```

## Step 8 — Write the Plan (or Re-plan)

Plan file path: `tasks/current/{task-id}_{task-type}_{short-description}/plan.md`.

If `plan.md` already exists, present the current plan to the user and offer three choices:

| Choice      | Action                                                                                                     |
| ----------- | ---------------------------------------------------------------------------------------------------------- |
| **Keep**    | Skip to Step 9 with the existing plan as the candidate. If user re-approves → Step 10.                     |
| **Revise**  | Enter the iterate-loop in Step 9 with the existing plan as the starting point.                             |
| **Rewrite** | Discard `plan.md`, re-run Steps 5-7 (rebuild context, re-ask clarifying questions if needed), then write a new plan below. |

If `plan.md` does not exist, write it fresh.

Use subagents wherever applicable (especially `type: spike` or `topics: research`).

The plan file must:

- Cross-link to `description.md` with a relative link (`./description.md`).
- Cross-link to the relevant docs file (for example if an ADR was implemented in a task).
- Include a step-by-step execution plan.
- Note any ADRs that will be created/updated.
- Note any documentation that will be updated.
- Note any new/updated files in `docs/guidelines/`.
- Include a `## Assumption grounding` section (see below).
- Ensure every `## Acceptance Criteria` item that touches an external boundary is a real DoD checkbox — no manual smoke step lives outside `## Acceptance Criteria` (see below).

### `## Assumption grounding` (mandatory)

Every literal the plan encodes as fact — path layout, id scheme, schema key, protocol constant, subprocess argument shape — appears in a table with its source in-repo. A plan with no external assumptions must say so explicitly (`_None: pure in-memory refactor._`); an empty section is a bug.

```markdown
## Assumption grounding

| Assumption | Source `file:line` | Verified line |
|---|---|---|
| Asset folder layout is `assets/<type>/<id>/` | `internal/asset/asset.go:184` | `dir := filepath.Join(root, config.AssetsDirName, string(manifest.Type), manifest.ID)` |
| `git rev-parse --show-toplevel` prints repo root, no trailing slash | `git` man page (external) — verified in repro shell | — |
```

The reader should be able to click a source line and confirm the assumption without re-reading the plan.

### Acceptance criteria for boundary-crossing changes

For any criterion whose subject crosses to an external system (filesystem, git, subprocess, network, other process), the criterion body must name the **real** components exercised end-to-end. A criterion that only asserts a fake-recorded-tuple does not satisfy the rule and must be paired with a real-stack sibling.

Anti-pattern (single fake-only criterion — reject):

```markdown
- [ ] Fake committer records `(dir, pathspec, msg)` for asset save.
```

Fixed shape (fake + real-stack pair):

```markdown
- [ ] `TestUpdateAsset_CommitsManifestPathspec` uses real `svc.InitAsset` + real `git` binary + real repo (`t.TempDir()` + `git init`), calls `svc.UpdateAsset`, asserts `git log -1 --format=%s` matches `chore(agentfiles): update asset <id> manifest` and `git show --name-only HEAD` matches the on-disk asset dir.
- [ ] Nested-repo variant: profile placed at `<repoRoot>/profiles/<name>/` (not repo root) — same assertions pass.
```

Any `## Verification` smoke step is moved into `## Acceptance Criteria` as a ticked checkbox so `af.task.implement` Step 5's DoD gate catches it. Verification-section-only smoke steps are not enforceable and get skipped in practice (this happened on task 0042; see `docs/changelog/2026-07-23_0042-git-aware-commits.md`).

## Step 9 — Request Approval, Iterate

Present the plan and ask the user to review. Loop:

1. User asks for changes → update `plan.md` → ask for confirmation.
2. Repeat until user **approves**.

Do not proceed to Step 10 without explicit approval.

## Step 10 — Set Status to in-progress

Only after the user has approved the plan, edit `description.md` frontmatter: `status: in-progress`.

This status transition is the signal to `/implement-task` that the plan is ready. Setting it before approval would let the user run `/implement-task` on an unapproved plan.

## Step 11 — Conclusion

Summarize the plan briefly and tell the user:

- Plan is approved and recorded at `tasks/current/{task-id}_{task-type}_{short-description}/plan.md`.
- Task status is now `in-progress`.
- **Next step:** run `/implement-task {task-number}` (ideally in a fresh session).

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

| Condition                     | Response                                     |
| ----------------------------- | -------------------------------------------- |
| Task in `done/`               | Inform user, stop                            |
| Task in `backlog/`            | Tell user to refine first, stop              |
| Task not found                | Inform user, stop                            |
| Missing frontmatter           | Tell user to fix, stop                       |
| Invalid frontmatter field     | Signal specific error, stop                  |
| Status already `in-review`/`done` | Task past planning, stop                 |
| Dirty working tree            | Tell user to clean up, stop                  |
| Plan not yet approved         | Wait for approval, do not touch status       |
