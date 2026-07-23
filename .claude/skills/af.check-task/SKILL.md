---
name: Check Task [Agentfiles]
description: Use when the user invokes /check-task <task-number> (e.g. /check-task 0001) to audit a task's `description.md` against the format enshrined by `af.create-task`. Locates the task, validates folder + frontmatter + body, reports every deviation, and — when a fix requires user judgement — hands off to the `grilling` skill instead of guessing.
disable-model-invocation: true
---

# Check Task

Audits an existing task and confirms it matches the layout produced by
`af.create-task`. Reports every deviation. When a fix cannot be inferred
mechanically, invoke the `grilling` skill to interview the user; do **not**
silently rewrite ambiguous fields.

This skill only touches `description.md` (and only after grilling produces an
agreed answer). It never edits `plan.md`, `review.md`, code, or config.

> [!IMPORTANT]
> Follow the steps **in order**. Do not skip a step.

## Input

Single argument: the **task number** as a 4-digit string (e.g. `0001`, `0042`).
If missing, list every task across `tasks/backlog/`, `tasks/current/`, and
`tasks/done/` with `AskUserQuestion` and let the user pick one.

## Task Layout (authoritative)

Same layout as `af.create-task`:

- Tasks live in `tasks/backlog/`, `tasks/current/`, or `tasks/done/`.
- Folder pattern: `NNNN_<type>_<slug>` (e.g. `0029_feature_plan-project-screen`).
    - `NNNN` — zero-padded 4-digit id.
    - `<type>` — one of `feature`, `bug`, `task`, `docs`.
    - `<slug>` — kebab-case derived from the title.
- `description.md` frontmatter fields:
    - `id` (required) — 4-digit string, must match `NNNN` in folder name.
    - `type` (required) — must match `<type>` in folder name.
    - `status` (required) — `pending` in backlog, `active` in current, `done` in done.
    - `topics` (required) — comma-separated list; each entry must be a filename (without `.md`) under `docs/guidelines/`, excluding `README`.
    - `depends_on` (optional) — comma-separated 4-digit ids; every id must exist as a task and be **numerically lower** than `id`.
    - `notes` (optional) — freeform text.
- Body starts with a level-1 heading: `# <Title>`.

## Step 1 — Locate Task

Scan `tasks/backlog/`, `tasks/current/`, `tasks/done/`. Find the directory whose
folder name starts with the input id.

- **0 matches** → report `task <id> not found` and stop.
- **>1 matches** → report every path found; stop and ask the user which one to
  check (do not guess).
- **1 match** → continue with the resolved directory + parent (`backlog` /
  `current` / `done`).

## Step 2 — Read the file

Read `description.md`. If it is missing, report `<path> has no description.md`
and stop.

Parse the YAML frontmatter and the body separately. If frontmatter is missing
or malformed YAML, record it as a hard finding and continue checking the body
where possible.

## Step 3 — Validate the folder name

Split the folder name on `_` into `<id>`, `<type>`, `<slug>`.

- `<id>` must be exactly 4 digits.
- `<type>` must be one of `feature`, `bug`, `task`, `docs`.
- `<slug>` must be non-empty kebab-case (`[a-z0-9]+(-[a-z0-9]+)*`).

Record any mismatch as a finding, but do not rename the folder in this skill.
Folder renames are a manual/user decision because they may break references
elsewhere (branch names, commit trailers, links).

## Step 4 — Validate frontmatter fields

For each field:

- **`id`** — present, 4-digit string, equal to folder `<id>`. If it disagrees
  with the folder id, that is a hard finding: mark it and ask the user which
  side is correct via `grilling`.
- **`type`** — present, equal to folder `<type>`.
- **`status`** — present, matches parent folder:
    - `tasks/backlog/` → `pending`
    - `tasks/current/` → `active`
    - `tasks/done/` → `done`
- **`topics`** — present, non-empty. List `docs/guidelines/` at check time.
  Every topic entry must appear (case-sensitive, filename minus `.md`, excluding
  `README`). Unknown topics are hard findings.
- **`depends_on`** (only if present) — every id must be a 4-digit string,
  match an existing task folder (any of the three parents), and be numerically
  lower than `id`. Self-references and forward references are hard findings.
- **`notes`** (only if present) — must be a non-empty string. Empty `notes:`
  with no value is a soft finding (recommend removing the field).

Unknown extra frontmatter keys are soft findings — flag but do not remove.

## Step 5 — Validate the body

- The first non-blank line after the frontmatter must be `# <Title>`. Missing
  title is a hard finding.
- Derive the expected `<slug>` by kebab-casing the title (lowercase, spaces →
  `-`, drop punctuation). Compare against folder `<slug>`. Divergence is a
  **soft** finding — slugs drift as titles are edited and renaming is a manual
  call; ask via `grilling` before proposing a rename.

## Step 6 — Categorise findings

Split findings into two buckets:

- **Hard** — clearly wrong per the format above (missing required field,
  wrong `status` for parent folder, unknown `type`, unknown topic, forward /
  self dependency, missing title, mismatched `id`/`type` between folder and
  frontmatter).
- **Soft** — ambiguous or judgement calls (slug drift vs. title, empty
  `notes:`, extra unknown keys, dependencies whose semantic relevance you
  cannot judge from the outside).

## Step 7 — Report

Emit one report block per finding. Format:

```
[hard|soft] <field-or-location>: <what is wrong>  →  <what a compliant value looks like>
```

If there are zero findings, say so and stop.

## Step 8 — Grill on anything ambiguous

For every finding whose fix requires user judgement (all soft findings, and
any hard finding where the correct value is not mechanically obvious — e.g.
`id` mismatch between folder and frontmatter, unknown topic that might be a
new guideline), invoke the `grilling` skill and let it interview the user
one question at a time.

Do **not** batch these into a single multi-question prompt yourself — that is
exactly the failure mode `grilling` exists to prevent.

Findings whose fix **is** mechanically obvious (e.g. `status: active` but the
task sits in `tasks/backlog/` — the correct value is unambiguous once the
user confirms which side to trust) may be proposed directly, but still wait
for user confirmation before writing.

## Step 9 — Apply agreed fixes

After grilling / confirmation, edit `description.md` in place with the agreed
values. Never edit fields the user has not signed off on. Never move the task
folder in this skill — surface the recommendation and let the user run the
folder move themselves (`git mv` preserves history and may need to update
branch names).

## Step 10 — Hand off

Print a final summary: path checked, count of hard vs soft findings, count
fixed vs. left open. If any hard finding remains unresolved, exit non-quietly
so the user knows follow-up is required.
