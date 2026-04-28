---
id: 0012
type: task
status: Pending
topics: go, profile
---

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
