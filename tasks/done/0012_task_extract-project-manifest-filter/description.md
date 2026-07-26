---
id: 0012
type: task
status: done
topics: go, profile
---

> [!NOTE]
> **Obsolete — not implemented.** Archived 2026-07-26.
> ADR 0017 moved projects out of profile folders into a single
> `~/.agentfiles/projects.json` (`internal/projectstore`). `profile.scanProjects`
> and the inlined `entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json")`
> filter no longer exist, so there is nothing to extract into
> `isProjectManifestFile`. Task premise is void.

# Extract project-manifest file filter into named helper

`profile.scanProjects` (`internal/profile/profile.go`) inlines the
"directory entry is a JSON file, not a sub-directory" check:

```go
if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
    continue
}
```

Extract this into `isProjectManifestFile(entry os.DirEntry) bool` so the
loop body reads cleanly and the rule has one home if the convention
ever changes (e.g. accept `.yaml` later).

Originally tracked as a `// WARN:` marker in `profile.go`.
