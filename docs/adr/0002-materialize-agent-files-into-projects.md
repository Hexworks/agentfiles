# Materialize Agent Files Into Projects

## Status

accepted

## Context

The application must synchronize LLM tooling files into normal repositories used
by multiple agents. Several synchronization strategies are possible, including
symlinks, hard links, virtual filesystems, or direct file writes.

The project needs predictable local behavior, normal Git compatibility, and a
clear preview/apply workflow.

## Decision

Materialize agent files directly into target repositories as normal files.
Planning happens first, then apply writes the desired files and records managed
state in `.agentfiles/state.json`.

## Consequences

Repositories remain inspectable with normal tools and Git workflows. Drift and
delete candidates can be explained clearly. The tradeoff is that generated files
must be overwritten on apply rather than remaining live-linked to their source.

