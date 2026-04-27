# ─────────────────────────────────────────────────────────────────────────────
# Makefile for agentfiles (af)
# ─────────────────────────────────────────────────────────────────────────────
#
# A Makefile is a build automation file used by the `make` command. It defines
# "targets" (like recipes) that you invoke by name:
#
#   make build      ← runs the "build" target
#   make test       ← runs the "test" target
#   make            ← runs the first target (build) by default
#
# Each target has a name, optional dependencies (other targets that must run
# first), and one or more shell commands indented with a TAB character.
# The TAB indentation is mandatory — spaces will not work.
#
# Format:
#   target-name: dependency1 dependency2
#   	command1
#   	command2
#
# ─────────────────────────────────────────────────────────────────────────────


# ─── Variables ───────────────────────────────────────────────────────────────
#
# Variables store reusable values. They are referenced with $(NAME) syntax.
# There are two assignment operators used here:
#
#   :=   "Simple assignment" — evaluated once, immediately, when the Makefile
#        is first read. The value is fixed from that point on.
#
#   ?=   "Conditional assignment" — only sets the variable if it is NOT already
#        defined. This lets you override it from the command line:
#          make build VERSION=1.2.3
#        If you don't pass VERSION, the ?= default kicks in.

# The name of the binary we're building.
BINARY    := af

# The Go package path containing main.go (the entry point).
CMD       := ./cmd/af

# The directory where the compiled binary is placed.
BUILD_DIR := ./bin

# ─── Version info (injected at compile time) ────────────────────────────────
#
# $(shell ...) runs a shell command and captures its output as the variable's
# value. These three variables gather version metadata from git:
#
#   git describe --tags --always --dirty
#     → Produces a version string like "v1.0.0" or "v1.0.0-3-gabcdef-dirty"
#       --tags:    use annotated and lightweight tags
#       --always:  fall back to commit hash if no tags exist
#       --dirty:   append "-dirty" if there are uncommitted changes
#
#   git rev-parse --short HEAD
#     → The short commit hash, e.g. "a1b2c3d"
#
#   date -u +%Y-%m-%dT%H:%M:%SZ
#     → Current UTC timestamp in ISO 8601 format, e.g. "2026-04-04T12:00:00Z"
#
# The "2>/dev/null || echo ..." part means: if git fails (e.g. not a git repo),
# suppress the error message (2>/dev/null) and use the fallback value instead
# (|| echo "dev").
#
# These are assigned with ?= so you can override them:
#   make build VERSION=1.0.0 COMMIT=abc123

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# ─── Linker flags ───────────────────────────────────────────────────────────
#
# LDFLAGS are "linker flags" passed to the Go compiler via -ldflags. They use
# the -X flag to inject values into Go variables at compile time:
#
#   -X main.version=$(VERSION)
#
# This sets the Go variable `version` in package `main` (cmd/af/main.go) to
# whatever $(VERSION) resolved to. This is how the binary knows its own version
# without hardcoding it in source code.
#
# The three -X flags set: main.version, main.commit, and main.date — which
# are the variables read by `af --version`.

LDFLAGS   := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)


# ─── .PHONY declaration ─────────────────────────────────────────────────────
#
# Normally, make treats each target name as a file. If a file called "build"
# existed in this directory, `make build` would say "nothing to do" because
# the file already exists.
#
# .PHONY tells make: "these targets are commands, not files — always run them
# regardless of whether a file with that name exists."

.PHONY: build test lint fmt clean run


# ─── Targets ─────────────────────────────────────────────────────────────────

# build: Compile the Go binary.
#
#   @mkdir -p $(BUILD_DIR)
#     Create the output directory if it doesn't exist.
#     The @ prefix suppresses printing the command itself (only output is shown).
#     The -p flag means "create parent directories as needed, don't error if
#     it already exists" (like mkdir --parents).
#
#   go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(CMD)
#     Compile the Go package at ./cmd/af, inject version info via ldflags,
#     and write the binary to ./bin/af.
#
# Usage:
#   make build                    ← build with auto-detected version
#   make build VERSION=1.0.0      ← build with a specific version

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(CMD)
	@echo "Built $(BUILD_DIR)/$(BINARY)"
	@mkdir -p $(HOME)/.local/bin
	@install -m 0755 $(BUILD_DIR)/$(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "Installed $(HOME)/.local/bin/$(BINARY)"

# test: Run all Go tests in the project.
#
#   ./... is a Go wildcard meaning "this directory and all subdirectories".
#   It finds and runs every *_test.go file in the project.
#
# Usage: make test

test:
	go test ./...

# lint: Run Go's built-in static analysis tool.
#
#   go vet checks for common mistakes that the compiler doesn't catch, such as
#   unreachable code, incorrect format strings, or suspicious constructs.
#   It's not a full linter (like golangci-lint) but catches many bugs.
#
# Usage: make lint

lint:
	go vet ./...

# fmt: Auto-format all Go source files.
#
#   gofmt is Go's official code formatter. The -w flag means "write changes
#   back to the files" (instead of just printing the formatted output).
#   The "." means "recursively format all .go files in the current directory."
#
#   In Go, there is ONE canonical formatting style enforced by gofmt — there
#   are no configuration options. This eliminates style debates.
#
# Usage: make fmt

fmt:
	gofmt -w .

# clean: Remove build artifacts.
#
#   rm -rf $(BUILD_DIR)    ← delete the ./bin/ directory and everything in it
#     -r = recursive (delete directories), -f = force (don't prompt, don't
#     error if it doesn't exist)
#
#   go clean               ← remove Go's cached build artifacts
#
# Usage: make clean

clean:
	rm -rf $(BUILD_DIR)
	go clean

# run: Build and immediately run the binary.
#
#   "run: build" means the "build" target is a dependency — make will run
#   "build" first, then run the binary. This is called a "prerequisite".
#
#   $(ARGS) is lintan empty variable by default. You pass arguments via:
#     make run ARGS="status --audit"
#   which expands to:
#     ./bin/af status --audit
#
# Usage:
#   make run ARGS="init"
#   make run ARGS="status --json"
#   make run ARGS="doctor --redundancy"

run: build
	$(BUILD_DIR)/$(BINARY) $(ARGS)


all: clean fmt lint test build
