#!/usr/bin/env bash
# Move every task directory whose description.md has `status: done`
# from tasks/backlog/ or tasks/current/ into tasks/done/.
#
# Run from anywhere; resolves the project's tasks/ relative to the
# repository root (git rev-parse) with a fallback to the current dir.

set -euo pipefail

if root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
    repo_root="$root"
else
    repo_root="$(pwd)"
fi

tasks_dir="$repo_root/tasks"
done_dir="$tasks_dir/done"

if [[ ! -d "$tasks_dir" ]]; then
    echo "error: $tasks_dir does not exist" >&2
    exit 1
fi

mkdir -p "$done_dir"

moved=0
skipped=0

for source in "$tasks_dir/backlog" "$tasks_dir/current"; do
    [[ -d "$source" ]] || continue

    for task_dir in "$source"/*/; do
        [[ -d "$task_dir" ]] || continue

        desc="$task_dir/description.md"
        if [[ ! -f "$desc" ]]; then
            continue
        fi

        # Read frontmatter (between the first two `---` lines) and look for
        # `status: done`. Use awk to keep parsing portable.
        status="$(awk '
            BEGIN { in_fm = 0; seen = 0 }
            /^---[[:space:]]*$/ {
                if (!seen) { in_fm = 1; seen = 1; next }
                else { in_fm = 0; exit }
            }
            in_fm && /^status:[[:space:]]*/ {
                sub(/^status:[[:space:]]*/, "")
                gsub(/[[:space:]]+$/, "")
                print
                exit
            }
        ' "$desc")"

        if [[ "$status" != "done" ]]; then
            continue
        fi

        name="$(basename "$task_dir")"
        target="$done_dir/$name"

        if [[ -e "$target" ]]; then
            echo "skip: $name already exists in tasks/done/" >&2
            skipped=$((skipped + 1))
            continue
        fi

        mv "$task_dir" "$target"
        echo "moved: $(realpath --relative-to="$repo_root" "$source")/$name -> tasks/done/$name"
        moved=$((moved + 1))
    done
done

echo "summary: moved=$moved skipped=$skipped"
