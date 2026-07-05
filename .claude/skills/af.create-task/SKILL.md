---
name: Create Task [Agentfiles]
description: "Create a new task using the Agentfiles task layout. Invoked by hand."
disable-model-invocation: true
---

Create a new task folder + `description.md` using the Agentfiles `tasks/` layout.
Ask the user for each field **one at a time**, waiting for the answer before the
next question. Prefer the harness selector tool (`AskUserQuestion`) wherever the
field has a fixed set of options.

Two example task descriptions live next to this file
(`example-1-description.md`, `example-2-description.md`) — match their structure.

## Layout facts

- Tasks live under `tasks/` in three folders:
    - `tasks/backlog/` — not started yet. Frontmatter `status: pending`.
    - `tasks/current/` — being worked on now. Frontmatter `status: pending`
      until `af.task.implement` starts and flips it to `in-progress`.
    - `tasks/done/` — finished (out of scope here).
- Folder name pattern: `NNNN_<type>_<slug>` (e.g. `0029_feature_plan-project-screen`).
    - `NNNN` is a zero-padded 4-digit id.
    - `<slug>` is a kebab-case version of the task title.
- `description.md` frontmatter fields: `id`, `type`, `status`, `topics`, `depends_on`.

## Step 0 — compute the next id

Scan `tasks/backlog/`, `tasks/current/`, and `tasks/done/`. Take the highest
existing `NNNN` prefix, add 1, zero-pad to 4 digits. That is the new task's id.

## Step 1 — title

Ask for a short task title (free text). Derive the kebab-case `<slug>` from it.

## Step 2 — type (selector)

Use `AskUserQuestion`. Options: `feature`, `bug`, `task`, `docs`.

## Step 3 — topics (multi-select)

The valid topics are the guideline filenames in `docs/guidelines/` (filename
without the `.md` extension, e.g. `git`, `go`, `tui`). List the directory at
invocation time and present the names as a multi-select via `AskUserQuestion`
(exclude `README`). The user may pick more than one.

## Step 4 — notes (optional, free text)

Ask for optional notes / initial description. If provided, they are set as the `notes` field in Frontmatter.
If skipped, omit the `notes` field.

## Step 5 — depends_on (multi-select, optional)

List every existing task whose id is **lower** than the new id (across all three
task folders). Present them as a multi-select via `AskUserQuestion` so the user
can pick zero or more. Store the chosen ids comma-separated (e.g. `0017, 0020`).
Omit the field if none selected.

## Step 6 — status / activation (selector)

Default is backlog + `pending`. Ask the user (selector, yes/no) whether this task
should be **active right away**:

- **No (default)** → write to `tasks/backlog/<folder>/`, frontmatter `status: pending`.
- **Yes** → write to `tasks/current/<folder>/`, frontmatter `status: pending`.
  (`af.task.implement` flips it to `in-progress` when it starts; `active` is
  **not** a valid status — `af.task.implement` / `af.task.review` only accept
  `pending|in-progress|blocked|in-review|done`.)

## Step 7 — create the files

Create the task folder in the chosen parent and write `description.md`. The body
**must** carry three required sections after the title: `## Acceptance Criteria`,
`## Out of scope`, `## Verification`. They start as placeholders — Step 8
(grilling) fills them.

The acceptance-criteria checklist **is** the Definition of Done: a task is done
when every box is `[x]` and `## Verification` passes. Keep criteria terse and
**verifiable** — behavioral ones name a concrete `input → output` or a one-line
smoke step. No separate DoD section (that would just restate the criteria).

```markdown
---
id: NNNN
type: <type>
status: <pending|in-progress>
topics: <comma-separated topics>
depends_on: <comma-separated ids> # omit line if none
notes: <freeform text> # omit line if none
---

# <Title>

## Acceptance Criteria

- [ ] <verifiable statement; behavioral → concrete input → output or smoke step>

## Out of scope

- <thing explicitly NOT being done>   # write "- none" if truly nothing

## Verification

```
make build && make test && make lint
# + any manual smoke line, e.g.  ./bin/af → <screen> → <action>
```
```

## Step 8 — hand off

After the file exists:

- If the `grilling` skill is available, invoke it to interview the user and flesh
  out the task details in `description.md`. The example tasks can be used for
  inspiration. The interview **must not finish** until:
    - `## Acceptance Criteria` has **≥1** checkbox, every criterion verifiable
      (if you cannot state how you'd check it, rewrite it until you can), and
    - `## Out of scope` is filled (`- none` is allowed only when nothing is
      genuinely excluded).
  These two sections are the task's Definition of Done — `af.task.review` gates
  on them, so a vague or empty checklist will block review later.
- Otherwise, tell the user the task was created (give the path), remind them the
  three required sections must be filled before `af.task.implement`, and open
  `description.md` for editing if the environment supports it.
