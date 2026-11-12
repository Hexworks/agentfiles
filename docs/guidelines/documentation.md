---
# This is a generic guidleine that can be shared accross projects
type: generic
keywords: architecture, readme, progressive-disclosure, ARC42
related: ./architecture.md, ../adr/, ../glossary.md, ../architecture/
---

# Documentation Guidelines

Documentation should stay close to the implementation and should prefer durable project knowledge over generic boilerplate. The goal is to make it easy for a future engineer or agent to understand the current architecture, the current vocabulary, and the reason past decisions were made.

You may find several complementary documentation forms. The `architecture` folder capture architecture views, `adr`s capture important decisions, guidelines capture working conventions, and the `glossary` captures shared domain language. The authoring conventions for architecture chapters live in [`architecture.md`](./architecture.md).

## Update The Right Artifact

Use the most specific documentation type for the change you are making.

```text
Do:
- update an architecture chapter when the architecture view changes
- add an ADR when the team makes a durable architecture choice
- update a guideline when the team standardizes a way of working
- update the glossary when a domain term changes or appears
```

```text
Don't:
- store long-term decision rationale only in a README
- add implementation conventions to an ADR
- define domain terms differently in multiple files
```

## Document Current Reality First

The first job of a document is to describe the current system accurately. Planned work can be noted, but it should not overwrite the present truth of the code.

```text
Do:
- say "the current implementation does X"
- call out gaps or ambiguities explicitly
```

```text
Don't:
- write future-state documentation as if it is already implemented
- smooth over mismatches between code and docs
```

## Keep Cross-Links Stable

The documentation set is meant to be navigated. Entry pages should link to canonical locations rather than duplicating content.

```text
Do:
- keep docs/README.md as the index for progressive disclosure
- let arc42 section 12 point to docs/glossary.md
- link arc42 section 9 to docs/adr/
```

```text
Don't:
- copy the same glossary content into multiple files
- leave orphan documentation pages without inbound links
```
