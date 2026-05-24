//go:build windows

package main

import "syscall"

// stillActive is the Windows STILL_ACTIVE / WAIT_TIMEOUT exit-code sentinel
// returned by GetExitCodeProcess when a process has not yet terminated.
const stillActive = 259

// IsAlive reports whether the process with pid is still running.
// On Windows we open a query handle and read the process exit code;
// a value of STILL_ACTIVE (259) confirms the process is alive.
func IsAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
