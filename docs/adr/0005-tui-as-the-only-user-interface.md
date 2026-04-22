# Use The TUI As The Only User Interface

## Status

accepted

## Context

The original CLI exposed every action through Cobra flags. Users had to know
the right `--profile`, `--type`, `--id`, `--agents` and `--assets` values up
front, which is error prone for `asset init` and `project add` in particular:
the asset type list, the supported agents, and the asset ids inside a profile
are all closed sets that the tool already knows about. Reading the help text
to assemble the right flags slowed down the most common workflows.

A separate `af tui` browsing screen existed for inspection but did not drive
any mutations, so two parallel surfaces had to be maintained.

## Decision

Make the TUI the only interactive surface. The Cobra command tree is kept as
a routing layer so users can still type `af profile create` or
`af project add` to jump directly to a flow, but every input is collected
through the TUI:

- Free-form values use single-line text inputs.
- Closed sets (asset type, supported agents) use Select / MultiSelect with the
  options the tool already enumerates internally.
- Profile and project pickers read live data from the registry and the
  selected profile so users never type ids by hand.
- Destructive actions (`project apply`) collect their `--delete` and
  confirmation prompts inside the same form.

The standalone `af tui` command is removed; running `af` with no subcommand
now opens the top-level menu.

## Consequences

The TUI is the single place where command UX evolves, removing duplication
between the flag parser and the browsing screen. Users stop hitting "missing
required flag" errors because the form refuses to submit until each field is
valid, and discoverability improves because every supported value is
presented as a list.

The cost is that flow execution now requires an interactive terminal. There
is no fully non-interactive `--yes`-style path; automation that previously
chained `af` invocations would need to re-introduce a non-interactive entry
point, which is left for a future ADR if the use case appears.
