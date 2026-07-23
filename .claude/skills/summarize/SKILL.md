---
name: summarize
description: Summarize all changes made during the current session into SUMMARY.md so other developers can understand the key changes months later. Use when the user runs /summarize or asks to summarize/document the work done this session.
---

# Summarize Session

## Overview

Writes a durable, human-readable summary of everything produced during the **current session** into `SUMMARY.md` at the repository root. The audience is a developer who joins the project later (possibly six months from now) and needs to understand _what_ changed, _why_, and _what trade-offs were made_ — without re-reading the diff or the chat log.

This is a documentation artifact, not a commit. It captures the reasoning that normally evaporates after a session ends: the alternatives considered, the assumptions made, and the decisions that future readers would otherwise have to reverse-engineer.

**The value of this file is the reasoning, not the file list.** A `git diff` already shows _what_ changed. `SUMMARY.md` exists to capture _why_ and _what else was on the table_. If a section has no rationale, no alternatives, and no assumptions, it is not pulling its weight.

## What counts as "the current session"

Everything you (Claude) did since the conversation started: files created/edited, behaviors changed, decisions taken, problems solved. Use your own working context as the primary source of truth — you know what you actually did and why.

To ground the summary in fact, also gather the mechanical change set:

- `git status` — uncommitted work produced this session.
- `git diff --stat` and `git diff` — unstaged changes.
- `git diff --cached --stat` and `git diff --cached` — staged changes.
- `git log <session-start>..HEAD --stat` — commits created this session, if any. If you know the first commit of the session, diff from there; otherwise infer the session boundary from the conversation and note any uncertainty.

If the mechanical change set and your memory of the session disagree, trust the diff for _what_ changed and your context for _why_.

## Algorithm

### Step 1: Gather the change set

Run the git commands above (in parallel where independent) to get an accurate picture of what changed on disk.

### Step 2: Reconstruct the reasoning

For each meaningful change, recall from the session context:

- **The decision** — what you chose to do.
- **Alternatives considered** — what else was on the table and why it lost. If you genuinely considered only one approach, say so explicitly rather than inventing alternatives.
- **Assumptions made** — anything you took as given that a future reader should double-check (e.g. "assumed the template registry is single-threaded", "assumed callers always pass a non-null `TemplateId`").

### Step 3: Write `SUMMARY.md`

Write to `SUMMARY.md` at the repo root using the structure below. **Append, don't clobber, by default** — if `SUMMARY.md` already exists, add a new dated session section at the top (most recent first) rather than overwriting prior sessions. If the user explicitly asks to overwrite, do so.

### Step 4: Report

Tell the user the path and a one-line description of what you recorded. Do **not** commit the file unless asked (per project memory: do not commit summaries/design docs on your own initiative).

## Required structure of `SUMMARY.md`

````markdown
# Session Summary — <YYYY-MM-DD>

<A few paragraphs (2–4) of prose: what was produced this session, the overall
goal, and the shape of the result. A reader should grasp the "what and why"
from this alone, before reading any individual change.>

---

## Changes

### <Short imperative header for change 1>

<A few paragraphs explaining this change: what it does and why it was needed.>

<Code example where it clarifies the change — use comments in the code to
explain, not just to label:>

```kotlin
// Before: registry scanned the directory on every request — O(n) per call.
// After: scan once at startup; lookups are O(1) map hits.
val template = registry[templateId] ?: throw TemplateNotFoundException(templateId)
```
````

> [!Note] <Highlight the single most important thing about this change — a
> gotcha, an invariant a future reader must preserve, or a risk.>

**Alternatives considered:** <What else was weighed and why it was rejected.
State "only one approach was viable because X" if that's the truth.>

**Assumptions:** <What was taken as given that should be verified if behavior
looks wrong later.>

### <Short imperative header for change 2>

...

```

## Content rules (non-negotiable)

These are the requirements that make the summary worth writing:

1. **Use code examples where they clarify.** Prefer a short, focused snippet over a paragraph of description. Put the explanation *in code comments* (e.g. `// guards against the null TemplateId case`), not only in surrounding prose. Skip the snippet when the change is non-code (config, docs, dependency bump) and prose is clearer.

2. **Always state alternatives considered.** Every change section names what else was on the table and why the chosen path won. If there truly was only one option, say so and why — never silently omit this.

3. **Always state assumptions.** Every change section lists the assumptions baked into it, so a future reader knows what to re-check if something breaks.

4. **Add highlighted notes for the most important points.** Use Markdown blockquote callouts (`> **Note:** ...`, or `> **Warning:** ...` for risks) at the points you consider most critical — invariants to preserve, sharp edges, things most likely to bite later. Don't spray these everywhere; reserve them for what genuinely matters.

> **Note:** The whole point of this skill is to preserve the *reasoning* behind changes. If you find yourself only describing *what* changed and not *why* / *what-else* / *assumed*, stop and add that — the diff already covers the "what".

## Caveman note

If caveman mode is active, **ignore it for the contents of `SUMMARY.md`** — this is durable project documentation read by other developers, so write it in normal, complete prose (the same way commits and PRs are exempt). Your chat replies to the user can stay terse.
```
