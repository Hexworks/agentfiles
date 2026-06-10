//go:build windows

package sync

// safeWriteFlags has no Windows equivalent for O_NOFOLLOW. The Lstat
// guard in writeRendered remains the only protection there.
const safeWriteFlags = 0
