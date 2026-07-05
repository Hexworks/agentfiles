---
name: Review Task [Agentfiles]
description: Use when the user invokes /review-task <task-number> (e.g. /review-task 0001) to review the implementation of a task that is currently in `in-review` state. Reads the task directory (description, plan, review notes), the changelog, and guidelines, dispatches parallel subagents for security/clean-code/clean-architecture/SOLID/DDD/testing/Go reviews, and writes a consolidated review file inside the task directory with solution checklists for the user to pick from. Stops after writing the review. Applying the chosen fixes is a separate skill (`af.task.review-apply`).
---

# Review Task

Review-document workflow. Takes a task id (e.g. `0001`), validates it is in `in-review`, gathers context (task directory, changelog, guidelines, ADRs, architecture, code), dispatches parallel review subagents, consolidates findings into `review.md` inside the task directory, then **stops**. The user ticks the solution checkboxes; applying the fixes is done by the separate `af.task.review-apply` skill (run in its own clean context).

> [!IMPORTANT]
> This skill **must be run with a clean context**. Do not chain it after other long-running work in the same session.

> [!IMPORTANT]
> This skill only **produces** the review file. It does **not** apply any fixes. Stop after Step 9.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Task Layout

Each task is a directory:

```
tasks/{backlog|current|done}/{task-id}_{task-type}_{short-description}/
    description.md   # task body + frontmatter
    plan.md          # written by implement-task
    review.md        # written by this skill (Step 9)
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

If `status` is anything other than `in-review`, signal the error explicitly: e.g. _"Task 0004 is in `in-progress`, not `in-review`. Cannot review until implementation is finished."_

## Step 3 — Understand the Task

Read in this order; do not start reviewing until all are read:

1. `description.md` (full body, including any `## Clarification` Q&A).
2. Every reference inside the description body (links to ADRs, architecture views, design notes, etc.).
3. `plan.md` inside the task directory if present.
4. The changelog at `docs/changelog/{YYYY-MM-DD}_{task-id}-{short-description}.md` if present. Find it by globbing `docs/changelog/*_{task-id}-{short-description}.md`.

If the plan or changelog is missing, note that as a finding for the review (the implement-task workflow expects both).

## Step 4 — Consult the Guidelines

For each entry in the task's `topics` field, read `docs/guidelines/{topic}.md`.

**Always** read the "must read" list regardless of topics:

- `docs/guidelines/clean_architecture.md`
- `docs/guidelines/clean_code.md`
- `docs/guidelines/domain_model.md` (covers DDD)
- `docs/guidelines/solid.md`
- `docs/guidelines/testing.md`

If you judge another guideline relevant to the diff (e.g. `security.md`, `go.md`, `sync_and_safety.md`, `asset_authoring.md`), read it too.

## Step 5 — Read the Docs

After guidelines:

1. Read every ADR under `docs/adr/` that the task or plan references, plus any ADR whose subject overlaps the diff.
2. Read the relevant arc42 sections under `docs/architecture/` (at minimum the building-block view if package layout changed).
3. Read any other doc you find relevant (glossary entries for new terms, etc.).

## Step 6 — Understand the Code

Inspect the local changes:

```bash
git status
git diff
git diff --stat
```

For each touched file:

- Read it in full (not just the hunks) so you understand surrounding context.
- Map every change back to an item in description / plan / changelog. Anything in the diff that is **not** justified by the task is itself a finding.
- Note coding patterns, naming conventions, and existing abstractions in the package; deviations are findings.

## Step 6.5 — Definition-of-Done Gate (Mandatory, runs BEFORE any subagent)

This gate uses only `description.md` + the `git diff` already read in Step 6 — no
subagents. It runs **first** on purpose: a failed gate means the Step 7 agent
burst would be wasted tokens reviewing work that does not yet meet intent.

1. Read `## Acceptance Criteria` from `description.md`.
   - Missing or empty → **STOP**. Tell the user the task predates the
     acceptance-criteria convention (see `af.create-task`) and must add a
     verifiable `## Acceptance Criteria` checklist before review can run.
2. For **each** criterion, judge **met / unmet from the diff**, citing concrete
   evidence (`file:line`). A criterion you cannot verify from the diff is itself
   a finding (either unmet, or the criterion is unverifiable and must be rewritten).
3. **Scope creep:** every diff hunk must trace to a criterion, or to a refactor
   that respects `## Out of scope`. A hunk that maps to no criterion and is not
   justified by the changelog is a finding.
4. Decision:

   | Result                                    | Action                                                                 |
   | ----------------------------------------- | ---------------------------------------------------------------------- |
   | All criteria met AND no scope creep       | Continue to Step 7.                                                     |
   | Any criterion unmet OR scope creep found  | **STOP.** Report the gap list (unmet criteria + unjustified hunks) to the user. Do **not** dispatch the Step 7 subagents. |

## Step 7 — Dispatch Parallel Review Subagents

Spawn **one subagent per topic in parallel** (single message, multiple Agent tool uses). Required topics:

- Security
- Clean code
- Clean architecture
- SOLID
- Domain Driven Design (DDD)
- Testing
- Golang

Each subagent prompt **must** include:

- The task id, the task directory path, and the changelog path.
- The exact list of changed files (`git diff --name-only`).
- The relevant guideline file path (e.g. `docs/guidelines/solid.md` for the SOLID agent).
- A request for findings in the exact output format specified in Step 9 (one `## {issue-short-description}` block per issue, with the warning callout, description, code example, and solution checklist).
- An instruction to return findings only — no code edits.

Wait for **all** subagents to complete.

## Step 8 — Synchronize Findings

Merge the subagents' findings:

- Deduplicate issues that multiple agents flagged (keep the strongest framing; cite all relevant guidelines in the warning callout).
- Resolve conflicts (e.g. one agent says "extract", another says "inline"): prefer the framing that aligns with the project guidelines and the plan; if still ambiguous, list both options as alternative checkboxes in the same solution checklist.
- Drop findings that are out of scope for this task or that the changelog already justifies.

## Step 9 — Produce the Review File

Path: `tasks/current/{task-id}_{task-type}_{short-description}/review.md`.

Use the template at [`./review-template.md`](./review-template.md). Read it, fill in placeholders, write to the path above.

Rules:

- One `##` block per distinct issue.
- The warning callout **must** link to a concrete file under `docs/guidelines/` (or another doc) — not a vague reference.
- Always include at least one code example per issue.
- Always include at least two solution checkboxes when sensible alternatives exist; otherwise a single checkbox is fine.
- Do **not** apply any fix yet.

After writing, **tell the user**: _"Review written to `tasks/current/{task-id}_{task-type}_{short-description}/review.md`. Read it and tick exactly one checkbox per issue for the solution you want applied. When ready, run `af.task.review-apply {task-number}` in a fresh session to apply the chosen fixes."_

This skill ends here. **Do not apply any fix** — that is the job of `af.task.review-apply`.

## Stopping Conditions Summary

| Condition                 | Response                                        |
| ------------------------- | ----------------------------------------------- |
| Task in `done/`           | Inform user, stop                               |
| Task in `backlog/`        | Inform user (not implemented yet), stop         |
| Task not found            | Inform user, stop                               |
| Missing frontmatter       | Tell user to fix, stop                          |
| Status not `in-review`    | Signal specific error, stop                     |
| `## Acceptance Criteria` missing/empty | Tell user task predates convention, stop before dispatch |
| Acceptance criterion unmet, or scope creep | Report gap list, stop before dispatch |
| Plan or changelog missing | Record as a finding; continue                   |
| Review file written       | Tell user to pick fixes + run apply skill, stop |
