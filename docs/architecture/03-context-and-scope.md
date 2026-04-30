# 3. Context And Scope

`agentfiles` sits between a user's profile library and one or more target source
repositories. Its responsibility is to resolve selected assets, transform them
into agent-specific outputs, and synchronize those outputs safely.

## System Context

```text
+------------------+        +----------------------+
| User / Terminal  | -----> |     agentfiles TUI   |
+------------------+        +----------------------+
           |                           |
           |                           +----> ~/.agentprofiles.json
           |                           |
           |                           +----> Profile folders
           |                           |      profile.json
           |                           |      assets/
           |                           |      projects/
           |                           |
           |                           +----> Target repositories
           |                                  AGENTS.md
           |                                  .claude/
           |                                  .cursor/
           |                                  .codex/
           |                                  .opencode/
           |                                  .agentfiles/state.json
```

## Business Context

The system has three external participants. Each interacts with the TUI in
domain-level terms; technical encodings are described in the next section.

| Participant         | Direction      | What flows                                                                  |
| ------------------- | -------------- | --------------------------------------------------------------------------- |
| User                | Both ways      | Selects profile, picks assets, confirms previews. Sees plans, reports, errors. |
| Profile library     | In             | Authoritative profile content: profile metadata, asset manifests, asset files. |
| Target repositories | Out (and read) | Receives rendered managed files. Existing managed files are read for drift comparison. |

## Technical Context

### User Interface

The system is fully TUI-driven. Running `af` opens the top-level menu and
all navigation happens from there; the binary takes no positional arguments
and has no subcommand tree. The only flag is `--registry`, used to override
the registry file location for tests and isolated environments.

### Global Registry

The registry file at `~/.agentprofiles.json` is used to discover profiles and
resolve them by id, name, or path.

### Filesystem

Profiles and projects are both normal folders. This is the dominant external
boundary of the application. Reads cover the profile library and the target
repository; writes cover the managed surfaces inside the target repository
and the per-repository state file at `<repo>/.agentfiles/state.json`.
