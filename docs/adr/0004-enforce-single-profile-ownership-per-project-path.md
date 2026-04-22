# Enforce Single-Profile Ownership Per Project Path

## Status

accepted

## Context

Multiple profiles can exist on one machine, and each profile can manage many
projects. If two profiles are allowed to control the same repository path, the
resulting apply behavior becomes ambiguous and unsafe.

## Decision

Treat project ownership as exclusive. A repository path may belong to only one
profile, and the application rejects attempts to register the same path under a
different profile.

## Consequences

Ownership is explicit, previews stay reliable, and synchronization conflicts are
reduced. The tradeoff is less flexibility for sharing one target repository
across separate profile contexts.

