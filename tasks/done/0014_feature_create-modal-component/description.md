---
id: 0014
type: feature
status: done
topics: go, tui, charm
---

# Modals

Modal windows are not yet supported in the charm ecosystem, but they can be implemented.
We need a Modal component that can host _huh_ forms. _huh_ already supports wizard-like
behavior such as:

- conditional fields (eg: the next field we render depends on a previous selection)
- groups that render as separate "pages"
  We can also use a _lipgloss_ `Compositor` to overlay modals.

This implementation should be part of the [tui](../../../internal/tui) package within
the `components` folder.

## Clarification

### Question

The frontmatter used `tags: go, tui, charm` but the implement-task workflow
expects `topics:`. How should this be handled?

### Answer

Rename `tags` → `topics` in the frontmatter and proceed.

## Plan

See [plan.md](./plan.md). A working reference implementation lives at
`~/projects/charm/go-playground/modal/` and is the basis for this port.
