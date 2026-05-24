---
id: 0014
type: feature
status: pending
tags: go, tui, charm
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
