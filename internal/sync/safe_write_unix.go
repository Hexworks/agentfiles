//go:build !windows

package sync

import "syscall"

// safeWriteFlags adds O_NOFOLLOW to writeRendered's open call. The
// kernel refuses to traverse a final-segment symlink so a race
// between Lstat and open cannot follow the link to an unrelated
// target.
const safeWriteFlags = syscall.O_NOFOLLOW
