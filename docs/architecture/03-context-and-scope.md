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
           |                           +----> ~/.agentfiles/           (user-config dir)
           |                           |      profiles.json            (registry)
           |                           |      projects.json            (project store)
           |                           |
           |                           +----> Profile folders          (shareable)
           |                           |      profile.json
           |                           |      assets/
           |                           |
           |                           +----> Target repositories
           |                                  AGENTS.md
           |                                  .claude/
           |                                  .cursor/
           |                                  .codex/
           |                                  .opencode/
           |                                  .agentfiles/state.json   (managed-state dir)
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
and has no subcommand tree. The flags are `--registry` and `--projects`
(both used to override the corresponding user-config file location for
tests and isolated environments) and `--theme`.

### User-Config Dir

The centralized user-config directory `~/.agentfiles/` (see ADR 0017)
holds both `profiles.json` (the profile registry) and `projects.json`
(the projects store). Its name intentionally matches the target-repo
managed-state dir; the two live under different anchors ($HOME vs repo
root) and the glossary disambiguates them.

### Filesystem

Profile folders hold shareable content only (`profile.json` + `assets/`);
per-user selections live in the user-config dir. Target repositories are
the second external boundary — reads cover managed files for drift
comparison, writes cover the managed surfaces and the per-repository
state file at `<repo>/.agentfiles/state.json`.
