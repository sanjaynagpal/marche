//go:build !windows

package main

import "syscall"

// IsAlive reports whether the process with pid is still running.
// On Unix we send signal 0, which tests process existence without delivering
// an actual signal. EPERM means the process exists but is owned by another
// user — we treat that as alive too.
func IsAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
