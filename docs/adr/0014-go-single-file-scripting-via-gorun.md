# Project Scripting With Single-File Go Via gorun

## Status

accepted

## Context

The project needed a place for small operational/dev scripts (setup,
one-off helpers). The default reach is a shell or Python script, but that
introduces a second language and toolchain into a repository whose entire
codebase, conventions, and contributor expertise are Go. Maintaining shell
for anything non-trivial also forfeits the type safety and standard library
the rest of the project relies on.

`gorun` (`github.com/erning/gorun`) lets a single `.go` file be executed
directly through a shebang, compiling and caching transparently. That keeps
scripts in the project's own language without turning each one into a
package or module.

## Decision

Project scripts are **single-file Go programs run by `gorun`**, living in a
top-level `script/` folder.

- `script/hello` is the reference single-file Go program, using the
  `#!/usr/bin/env gorun` shebang.
- `script/setup` is a POSIX `sh` bootstrap (the one unavoidable shell
  script) that idempotently installs the Go toolchain — preferring `mise`
  when `mise.toml` is present, otherwise the official tarball into
  `~/.local/go` — and runs `go install github.com/erning/gorun@latest` so
  the shebang resolves on a fresh machine.

The `Makefile` remains the entry point for building and testing the binary;
`script/` is for auxiliary tasks that benefit from being written in Go
rather than expanding the `Makefile` or adding shell.

## Consequences

- Scripts share the project's language, tooling, and standard library;
  contributors do not context-switch to shell/Python for non-trivial logic.
- A single `sh` bootstrap (`script/setup`) is the only non-Go script and
  exists precisely to make the Go path available, including on a clean
  environment.
- `gorun` becomes a dev dependency. It is installed by `setup`, not vendored,
  so it does not enter the module graph or the shipped binary.
- This is a developer-tooling decision, not a runtime one: nothing in
  `cmd/af` or `internal/` depends on `gorun` or on anything under `script/`.

## References

- `script/hello`, `script/setup`
- `mise.toml` (toolchain pin consulted by `setup`)
- `github.com/erning/gorun`
