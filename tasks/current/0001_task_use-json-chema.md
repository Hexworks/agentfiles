---
id: 0001
type: task
status: Pending
tags: research
---

# Use Json Schema

Currently we load JSON as-is and don't validate whether the file has the appropriate structure.

We need to migrate this to use JSON Schema, or some other schema utility whenever when serialize/deserialize JSON.

The related functions in `fsutil.go` also need to be refactored and they need to be generic functions.

All data structures that we load into the app need to have their own model type in go to keep type-safety.

As part of this task we also need to find the appropriate library. The goal is to have a mechanism that can

- Serialize in-memory data based on a type into a JSON file (the `WriteJSON` function)
- Deserialize JSON into in-memory data based on a type from a JSON file (the `ReadJSON` function)
- Signal an error in the TUI if the file is malformed

**Note that** it is possible that we don't need JSON Schema for this. If Go already has built-in functionality
that we can re-use then JSON Schema won't be necessary, but it needs to be an utility that allows for
**schema evolution**. **Research this!**
