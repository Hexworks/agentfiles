# Security Guidelines

Security in `agentfiles` is mostly about protecting the user's repositories,
profile content, local configuration, and credentials. The project should assume
that manifests, profile files, project paths, and generated content can be
mistyped, stale, or hostile until validated.

The default posture is simple: validate inputs at boundaries, keep writes inside
known roots, avoid exposing secrets, and make risky operations visible before
they happen.

## Treat External Input As Untrusted

Validate data loaded from manifests, state files, command arguments, environment
variables, and filesystem paths before using it.

```text
Do:
- parse structured files with the expected parser
- validate required fields, ids, compatibility, and ownership rules
- reject unknown or unsafe path forms early
- return errors that explain the invalid field or constraint
```

```text
Don't:
- trust profile or project files because they live in a known folder
- accept partially valid manifests and rely on later code to fail safely
- build behavior from unchecked free-form strings
```

## Keep File Access Inside Intended Roots

All reads and writes should stay within the profile, registry, project, or
managed-output root that the operation is responsible for.

```text
Do:
- clean and validate paths before joining them with a trusted root
- reject path traversal, absolute paths where relative paths are required, and
  paths that escape the expected root after evaluation
- treat symlinks and existing unexpected files as safety-sensitive during sync
- write only the files identified by the preview or apply plan
```

```text
Don't:
- concatenate paths with string operations
- let asset ids or manifest fields choose arbitrary filesystem destinations
- broaden deletion or overwrite logic outside the managed surface
```

## Protect Secrets And Local State

The tool should not collect, persist, print, or render credentials unless a
feature explicitly requires it and documents the handling rules.

```text
Do:
- keep tokens, API keys, and private paths out of logs, previews, errors, and
  generated files
- redact sensitive values when reporting configuration or environment problems
- store only the minimum state needed for drift detection and ownership checks
- use restrictive permissions for files that may contain user-local data
```

```text
Don't:
- include environment dumps in diagnostics
- store secrets in `.agentfiles/state.json` or profile manifests
- copy credentials from source assets into generated project files
```

## Avoid Unsafe Execution Paths

Profile assets and generated files are content, not code to execute as part of
normal rendering or synchronization.

```text
Do:
- prefer Go APIs over shell commands for filesystem and JSON work
- pass command arguments as structured argument lists when a subprocess is
  unavoidable
- require an explicit, reviewed feature boundary before adding network access or
  command execution
```

```text
Don't:
- execute hooks, scripts, templates, or commands from profile content
- build shell commands by interpolating user-controlled values
- add background network calls to normal read, render, preview, or apply paths
```

## Keep Dependencies Boring

Dependencies expand the trusted code surface. Add them only when the security and
maintenance tradeoff is clear.

```text
Do:
- prefer the standard library for parsing, paths, hashing, and file I/O when it
  is sufficient
- pin dependency versions through the Go module files
- review new dependencies for maintenance status, transitive size, and security
  history
```

```text
Don't:
- add a dependency for a small helper that is easy to implement safely
- vendor or copy code without preserving license and update responsibility
- ignore security advisories in packages that touch parsing, filesystem, or
  command execution
```

## Test Security-Sensitive Behavior

Security rules should be covered where mistakes would create unsafe writes,
secret exposure, or unexpected execution.

```text
Do:
- test path traversal and root-escape rejection
- test drift, overwrite, and deletion safeguards with `t.TempDir()`
- test redaction for errors or diagnostics that include configuration values
- test invalid manifest and state inputs directly
```

```text
Don't:
- rely only on happy-path sync tests for safety behavior
- skip tests for edge cases because they look like validation details
```
