# Documentation

This project uses a small, practical documentation structure built around
`arc42`, Architecture Decision Records, project guidelines, and a shared domain
glossary.

The goal is to keep architecture knowledge close to the code, record important
decisions while they are still fresh, and maintain a stable vocabulary for the
`agentfiles` bounded context. The documents in this folder are intentionally
incremental: they describe the current implementation first, then evolve as the
system grows.

## Structure

- [Architecture](./architecture/01-introduction-and-goals.md): the `arc42`
  architecture set for the current system
- [ADRs](./adr/README.md): durable records of important architectural decisions
- [Guidelines](./guidelines/README.md): cross-cutting development and design
  conventions
- [Glossary](./glossary.md): canonical definitions for project domain terms

## Maintenance Rules

When the implementation changes in a way that affects architecture, update the
matching `arc42` chapter and add a new ADR if the change records a meaningful
decision. When the team wants to standardize how work should be done, add or
update a guideline. When a term changes meaning or a new domain term appears,
update the glossary.
