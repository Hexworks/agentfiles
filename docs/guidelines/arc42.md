# Arc42 Guidelines

Arc42 is a 12-section template for architecture documentation. Each section
has a fixed purpose, and together the set covers goals, constraints, context,
strategy, structure, behaviour, deployment, crosscutting concepts, decisions,
quality, risks, and vocabulary. The template itself is technology- and
process-neutral: it works for a single-binary tool and a distributed platform
alike, and it can be authored in lean, essential, or thorough depth.

These guidelines codify how to **create and maintain** an arc42 set so that
future updates stay consistent in tone, depth, and structure. The body is
language- and project-agnostic; concrete repo conventions live in the final
"Application In This Repo" section.

Related: [Documentation](./documentation.md), [`docs/adr/`](../adr/),
[`docs/glossary.md`](../glossary.md).

## Use Arc42 For Architecture Views

Use arc42 whenever the architecture needs to be communicated beyond the team
that wrote it: onboarding, stakeholder review, audits, or future maintenance
years after the original authors are gone. The template is cheap to apply at
essential mode and pays back the moment a new contributor needs to find their
way around.

Skip arc42 for trivial systems: a single-file script, a throwaway prototype,
or a utility whose entire architecture fits in one paragraph in the README.
The template is overhead unless there are at least a handful of components,
external interfaces, or durable decisions worth recording.

## The Twelve Sections At A Glance

The set is fixed: twelve files, one per section, in the established order.

| #  | Conventional file name              | Purpose                                                              |
| -- | ----------------------------------- | -------------------------------------------------------------------- |
| 1  | `01-introduction-and-goals.md`      | Problem framing, top quality goals, stakeholders.                    |
| 2  | `02-architecture-constraints.md`    | What cannot change: technical, organisational, conventions.          |
| 3  | `03-context-and-scope.md`           | System boundary, external interfaces, business and technical context.|
| 4  | `04-solution-strategy.md`           | Fundamental decisions: tech choices, decomposition, quality strategy.|
| 5  | `05-building-block-view.md`         | Static decomposition; black-box and white-box views.                 |
| 6  | `06-runtime-view.md`                | How components interact at runtime in important scenarios.           |
| 7  | `07-deployment-view.md`             | Software-to-infrastructure mapping.                                  |
| 8  | `08-concepts.md`                    | Crosscutting concerns spanning multiple building blocks.             |
| 9  | `09-architecture-decisions.md`      | Index of significant decisions; each entry references an ADR.        |
| 10 | `10-quality-requirements.md`        | Measurable quality scenarios beyond the top goals in §1.             |
| 11 | `11-technical-risks.md`             | Prioritised risks and technical debt with mitigation.                |
| 12 | `12-glossary.md`                    | Domain vocabulary; usually delegates to a canonical glossary.        |

## Section Authoring Rules

Each section has a narrow purpose. Stay inside it. The most common
maintenance failure is content drifting into the wrong section, which forces
future readers to look in three places for one topic.

### 1. Introduction And Goals

Belongs: a short framing of what the system is and why it exists; a
prioritised table of three to five quality goals; a stakeholder table with
expectations.

Does not belong: requirements detail (link to a backlog or to §10), planning
content like deadlines or sprint goals, or vague qualities like "scalable"
without a concrete expectation.

Common mistake: listing many unranked goals. Force a priority order so
trade-offs can be reasoned about; if everything is top priority, nothing is.

### 2. Architecture Constraints

Belongs: technical constraints (mandated platform, language, storage
choices), organisational constraints (regulatory, team boundaries, vendor
contracts), and conventions (coding standards, versioning scheme, branching
model). Group by these three buckets.

Does not belong: design decisions made within the constraints (those go in
§4 or §9), or preferences framed as constraints to make them harder to
revisit.

Common mistake: omitting the *why* and the *impact* of each constraint.
Without that, future readers cannot tell whether the constraint still binds.

### 3. Context And Scope

Belongs: a system boundary diagram, a business-context view (who uses the
system, what flows in and out at the domain level), and a technical-context
view (protocols, file formats, network paths, integration points).

Does not belong: internal component details, implementation choices inside
the boundary, or operational procedures.

Common mistake: showing only major external systems and missing minor ones.
Show every interface; even small ones constrain the architecture.

### 4. Solution Strategy

Belongs: the handful of decisions that shape everything else — the chosen
decomposition pattern, the technology backbone, the strategy for each
quality goal in §1, and any organisational tactics (build-vs-buy, partner
choices). A small *quality goal → strategy → reference* table is the canonical
form.

Does not belong: implementation detail (defer to §5 and §8) or rejected
alternatives (those belong in an ADR if they are interesting).

Common mistake: a list of decisions without rationale. Each decision needs
to point back to a goal or constraint it serves; otherwise it is just
trivia.

### 5. Building Block View

Belongs: a hierarchical static decomposition. Level 1 is the system as a
small set of black boxes with one-sentence purposes and a diagram of their
relationships. Level 2 opens up each box that is architecturally
significant; deeper levels are added only where the structure is non-obvious.
Each block names its responsibility, its interfaces, and where to find its
source code.

Does not belong: every minor module, every internal class, or implementation
algorithms (those go in §8 or in code).

Common mistake: a flat list with no diagram and no levels. The whole point
of §5 is to show structure; a bullet list is not enough.

### 6. Runtime View

Belongs: a small number of important scenarios — a happy-path use case, a
critical error path, startup or shutdown if non-trivial, and any background
or batch flow that is architecturally significant. Each scenario has a
diagram (sequence, activity, or state) and a short numbered narrative.

Does not belong: every conceivable user flow, or trivial sequences that are
obvious from the code.

Common mistake: too many scenarios at the same level of detail. Pick the
two or three that explain the most about how components cooperate.

### 7. Deployment View

Belongs: a description of where the software runs (developer workstation,
container, server, cloud region), a diagram of the relevant infrastructure
nodes, and the mapping of building blocks from §5 to those nodes. Include
the install or distribution path.

Does not belong: internal component detail or configuration management
that lives outside the architecture set.

Common mistake: omitting the deployment view because the system is
"just local" or "just one container". Even a one-node deployment has an
operational picture worth recording.

### 8. Crosscutting Concepts

Belongs: concerns that span multiple building blocks — error handling,
security boundaries, persistence, concurrency, the domain model, testing
strategy, observability, internationalisation. Each concept gets a short
H2 with a paragraph and, if a guideline already covers the detail, a link.

Does not belong: concerns that affect only one building block (document
those inside the block in §5), or every possible buzzword. Pick what
actually crosscuts in this system.

Common mistake: duplicating content from a sibling guideline. §8 names the
concept and links out; the guideline owns the detail.

### 9. Architecture Decisions

Belongs: a curated index of architecturally significant decisions, each
entry one bullet that references an ADR file. ADRs themselves live in a
separate directory and follow the standard *status / context / decision /
consequences* format.

Does not belong: decision rationale itself (it lives in the ADR), or minor
implementation choices (they belong in code review or guidelines).

Common mistake: writing the decision body in §9 and leaving the ADR
directory empty. Always put the rationale in an ADR; §9 is a guide to the
set, not a replacement for it.

### 10. Quality Requirements

Belongs: measurable scenarios in *context / stimulus / response / response
measure* form. Each scenario covers one quality attribute (performance,
availability, security, modifiability, usability, …). The top three to
five goals from §1 should each have at least one scenario here.

Does not belong: vague aspirations without a measure, or repetition of §1
content without elaboration.

Common mistake: writing "the system should be fast" instead of "list
operations return within 200 ms at the 95th percentile under nominal load".
Without a measure, the requirement cannot be checked.

### 11. Risks And Technical Debt

Belongs: a prioritised list — usually a table — of identified risks and
debt items. Each entry names the risk, an impact assessment, a likelihood,
and a mitigation or follow-up plan.

Does not belong: business risks (market, organisational), or speculative
worries with no architectural consequence.

Common mistake: an unprioritised list with no mitigation column. The
section's value is letting decision-makers see what to act on first.

### 12. Glossary

Belongs: domain terms used across the architecture. Two-column form (term
and definition) is the standard. Where a project already maintains a
canonical glossary in another file, this section delegates to it with a
single link.

Does not belong: implementation jargon, every common technical term, or
duplicate definitions of terms maintained elsewhere.

Common mistake: copying glossary content into both files. Keep one source
of truth and delegate from the other.

## Diagram Conventions

```text
Do:
- use Mermaid for new diagrams (flowchart, sequenceDiagram, classDiagram)
- include at least one diagram in §5 and one in §6
- keep small ASCII context boxes if they are already there and still readable
- add a legend whenever the notation is non-obvious
```

```text
Don't:
- commit binary images for diagrams the renderer can produce inline
- mix three different notations in the same chapter without explanation
- replace text with a diagram; diagrams supplement narrative
```

Mermaid is the preferred portable choice because it renders inline in any
Markdown viewer that supports it (most do) and it lives in the same file as
the prose, so the diagram updates with the text.

## Pick The Right Granularity

Three modes are acceptable; pick one explicitly and stick to it across the
whole set:

- **Lean** — ten to forty lines per chapter, almost no diagrams. Suitable
  for a small system where the architecture is simple but worth recording.
- **Essential** — thirty to one hundred and ten lines per chapter, one
  diagram per mandatory section, rationale deferred to ADRs. The default
  for most systems.
- **Thorough** — multi-level decomposition, full diagram coverage, deep
  quality scenarios. Suitable for regulated domains, large platforms, or
  systems with many stakeholder groups.

Drift between modes makes the set hard to read. If a chapter is
consistently too short or too long, change the mode for the whole set
rather than letting individual chapters diverge.

## File Layout

One file per section, named `NN-kebab-case.md`, all under the same
architecture directory (conventionally `docs/architecture/`). Do not
renumber, split, or merge. The fixed numbering is part of the contract —
readers, links, and tooling all rely on it.

A typical layout:

```text
docs/
  architecture/
    01-introduction-and-goals.md
    02-architecture-constraints.md
    ...
    12-glossary.md
  adr/
    0001-...md
    ...
  glossary.md
  guidelines/
    ...
```

## Cross-Reference ADRs And Guidelines

The arc42 set sits in a small documentation ecosystem. Use it.

```text
Do:
- list each significant decision in §9 with a link to its ADR
- link from §8 to the guideline that owns the implementation detail
- delegate §12 to the canonical glossary file
- treat the chapters as views; the source of truth is the code, the ADRs,
  and the guidelines
```

```text
Don't:
- duplicate ADR content into §9
- duplicate guideline content into §8
- duplicate glossary entries into §12
- leave links to files that do not exist
```

## Maintenance Triggers

Architecture documentation rots quickly without explicit triggers. Use this
mapping when reviewing a change:

| Change                                            | Chapter to revisit             |
| ------------------------------------------------- | ------------------------------ |
| New module or component                           | §5                             |
| New user-visible flow or background process       | §6                             |
| New external interface                            | §3                             |
| New invariant or pattern shared across components | §8                             |
| Significant decision                              | new ADR + §9                   |
| New constraint                                    | §2                             |
| New deployment target or infra change             | §7                             |
| New quality target or scenario                    | §1 (if a top goal) or §10      |
| New risk or known debt item                       | §11                            |
| New domain term                                   | canonical glossary, then §12   |

Revisit during the same change that introduces the architectural shift.
After-the-fact updates lose the rationale.

## Style Rules

```text
Do:
- use H1 only for the chapter title
- use H2 for main sub-sections
- use H3 only when structure justifies it (e.g. one entry per item in a
  fixed list)
- write chapters in descriptive-neutral voice ("the system stores...")
- write guideline prose in imperative-active voice ("use a typed error...")
- keep paragraphs to two to four sentences
```

```text
Don't:
- nest headings deeper than H3 in chapters
- write planned state as if implemented (use "currently" if needed)
- include marketing copy or tutorial-style prose
- mix tones inside the same chapter
```

## Common Pitfalls

```text
Do:
- put rationale in an ADR, not in a chapter body
- keep the glossary delegation in §12 if a canonical glossary exists
- describe current reality first; defer planned work to risks or backlog
- include diagrams in §5 and §6
- prune content that has drifted into the wrong section
```

```text
Don't:
- treat arc42 as a one-time deliverable
- add UML for its own sake
- skip §11 because nothing seems risky right now
- keep stale ADR references in §9 after a decision is superseded
```

## Application In This Repo

This is the only repo-specific section. A fork rewrites or removes only
this part.

- **Granularity:** essential. Target thirty to one hundred and ten lines
  per chapter; longest chapter (§5) may exceed slightly when the Mermaid
  block is included.
- **Format:** Markdown. No AsciiDoc.
- **Diagrams:** Mermaid. The existing ASCII diagram in
  `docs/architecture/03-context-and-scope.md` is grandfathered and stays;
  new diagrams use Mermaid.
- **Architecture chapters:** `docs/architecture/01-introduction-and-goals.md`
  through `docs/architecture/12-glossary.md`. Filenames are part of the
  contract; do not rename.
- **ADR index:** `docs/adr/`. New decisions get a new ADR file plus a bullet
  in `docs/architecture/09-architecture-decisions.md`.
- **Glossary:** the canonical glossary lives at `docs/glossary.md`.
  `docs/architecture/12-glossary.md` is a one-paragraph delegation page and
  stays that way.
- **Crosscutting links from §8:** common targets are `errors.md`,
  `sync_and_safety.md`, `domain_model.md`, `security.md`, `testing.md`,
  and `clean_architecture.md` under `docs/guidelines/`. Add the link from
  §8; let the guideline own the detail.
