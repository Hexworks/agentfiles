---
name: Drift Cleanup [Agentfiles]
description: Use when the user invokes /drift-cleanup [commit-hash] to detect and fix documentation drift introduced by commits made since a baseline commit. If no commit hash is given, list recent commits and ask the user to pick one. Reconciles code changes against the project's docs: writes a summary changelog for undocumented code changes (docs/changelog/), updates arc42 architecture chapters (docs/architecture/), creates Architecture Decision Records (docs/adr/), and updates affected documentation and glossary entries (docs/glossary.md). Every fix is gated on the relevant docs folder existing — folders that are absent are skipped, never created.
---

# Clean up Drift

Documentation drift is the gap that opens when code moves forward but the
project's documentation does not. This skill takes a **baseline commit**,
inspects everything that changed between it and `HEAD`, and reconciles those
changes against the project's documentation system — creating or updating
changelogs, architecture docs, ADRs, documentation pages, and glossary entries
as needed.

Reference examples for every artifact format are bundled under this skill's
own `docs/` folder: [`docs/changelog/`](./docs/changelog/),
[`docs/architecture/`](./docs/architecture/), [`docs/adr/`](./docs/adr/), and
[`docs/glossary.md`](./docs/glossary.md). Match their format and tone.

## Input

Single **optional** argument: a commit hash (full or short) or any git ref.

- If provided → use it as the baseline.
- If absent → present recent commits and let the user choose (Step 1).

## Step 1 — Resolve the Baseline Commit

If a hash/ref was provided, validate it:

```bash
git rev-parse --verify "<hash>^{commit}"
```

If it does not resolve → tell the user the ref is invalid. **Stop.**

If no argument was provided, list recent commits and ask the user to pick one:

```bash
git --no-pager log -20 --pretty=format:'%h  %ad  %s' --date=short
```

Present the list and ask which commit to use as the baseline. **Wait** for the
user's choice before continuing. Do not guess.

## Step 2 — Collect Changes Since the Baseline

Gather the diff and commit log between the baseline and `HEAD`:

```bash
git --no-pager diff --stat <baseline>..HEAD
git --no-pager log <baseline>..HEAD --pretty=format:'%h %s'
```

- If there are **no** changes since the baseline → tell the user there is
  nothing to reconcile. **Stop.**
- Partition the changed paths into **code changes** (source files) and
  **doc changes** (anything already under `docs/`). The doc changes tell you
  what has _already_ been documented; the code changes are what you reconcile
  against.

Read the actual diffs for the code changes so you understand _what_ changed and
_why_ — you cannot write an accurate changelog or ADR from filenames alone.

## Step 3 — Detect Which Documentation Systems Exist

Probe the repository. Each detected folder/file **gates** one reconciliation
step. **Never create a documentation folder that does not already exist** — its
absence means the project opted out of that documentation type.

| Path                 | Gates                                   | Step |
| -------------------- | --------------------------------------- | ---- |
| `docs/changelog/`    | Changelog reconciliation                | 4    |
| `docs/architecture/` | Architecture (arc42) reconciliation     | 5    |
| `docs/adr/`          | ADR reconciliation                      | 6    |
| `docs/` (+ glossary) | Documentation + glossary reconciliation | 7    |

If `docs/` does not exist at all → tell the user there is no documentation to
reconcile. **Stop.**

## Step 4 — Changelog Drift (gated on `docs/changelog/`)

Compare the code changes since the baseline against existing entries in
`docs/changelog/`. A code change is **drifted** if no changelog entry describes
it.

If drifted code changes exist, write **one summary changelog** covering them:

- **Path:** `docs/changelog/{YYYY-MM-DD}_{NNNN}-{slug}.md`, where `{YYYY-MM-DD}`
  is today's date (UTC), `{NNNN}` is the highest existing changelog number `+ 1`
  (zero-padded to 4 digits), and `{slug}` is a short kebab-case description.
- **Format:** mirror the existing entries — a `# {NNNN} changes` heading, a
  prose summary, then `## Decisions`, `## Assumptions`, and `## Other Notes`
  sections, followed by per-change sections with `before`/`after` code blocks.
  See the bundled [`docs/changelog/`](./docs/changelog/) examples.

If every code change is already covered → note "no changelog drift" and move on.

## Step 5 — Architecture Drift (gated on `docs/architecture/`)

**Skip entirely if `docs/architecture/` does not exist.**

If the changes altered an architectural view (building blocks, runtime flows,
deployment, concepts, quality requirements), update the matching arc42
chapter(s). Read the existing chapters first, follow
[`docs/guidelines/arc42.md`](./docs/guidelines/arc42.md) if present, and
**document current reality** — do not write future-state docs as if shipped.

## Step 6 — ADR Drift (gated on `docs/adr/`)

**Skip entirely if `docs/adr/` does not exist.**

If the changes embody a durable architectural **decision** (a choice with
alternatives and consequences, not a routine edit), create an ADR per decision:

- **Naming:** zero-padded numeric prefix + kebab slug, next number after the
  highest existing (e.g. `0012-...md`).
- **Structure:** Michael Nygard style — `# Title`, `## Status`, `## Context`,
  `## Decision`, `## Consequences`. Default status `accepted`. See the bundled
  [`docs/adr/`](./docs/adr/) examples and [`docs/adr/README.md`](./docs/adr/README.md).
- Add the new ADR(s) to the Index in `docs/adr/README.md`.

Routine changes that make no real decision do **not** warrant an ADR.

## Step 7 — Documentation + Glossary Drift (gated on `docs/`)

Update any documentation pages (`docs/manual/`, `docs/guidelines/`, etc.) whose
content the code changes have made inaccurate or incomplete.

If a glossary file exists (e.g. [`docs/glossary.md`](./docs/glossary.md)), check
it: add entries for new domain terms introduced by the changes, and update
entries whose meaning changed. Keep one precise definition per term.

## Step 8 — Cross-Links

Keep the documentation set navigable:

- New ADRs referenced from arc42 §9 (architecture decisions), if that chapter exists.
- Glossary linked from arc42 §12, if that chapter exists.
- README indexes (`docs/README.md`, `docs/adr/README.md`) updated for new files.

## Step 9 — Report

Summarize:

- The baseline commit used.
- Per category (changelog, architecture, ADR, documentation, glossary): what
  drift was found and what you did, or "no drift" / "skipped — folder absent".
- Links to every file created or updated.

## Stopping Conditions Summary

| Condition                          | Response                            |
| ---------------------------------- | ----------------------------------- |
| Provided ref does not resolve      | Tell user, stop                     |
| No commit chosen (no-arg path)     | Wait for user's choice              |
| No changes since baseline          | Inform user, stop                   |
| No `docs/` folder at all           | Inform user, stop                   |
| A category's docs folder is absent | Skip that category, never create it |
