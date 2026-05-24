//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// buildStartCmd constructs the exec.Cmd for Unix platforms.
// The service is launched via bash, placed in its own process group
// (Setpgid: true) so it survives launchpad exiting.
func buildStartCmd(entry ServiceEntry, childEnv []string, stdout, stderr *os.File) *exec.Cmd {
	cmd := exec.Command("bash", entry.RunScript)
	cmd.Dir = entry.FolderPath
	cmd.Env = childEnv
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// buildRunCmd constructs an exec.Cmd for running a custom script synchronously.
// args are appended after scriptPath so a single script can handle both start
// and stop via different arguments (e.g. "runCMD.sh start" / "runCMD.sh stop").
// No process-group detachment is needed because the script exits quickly.
func buildRunCmd(scriptPath string, args []string, workDir string, env []string) *exec.Cmd {
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Dir = workDir
	cmd.Env = env
	return cmd
}

// sendStop sends SIGTERM to pid, requesting a graceful shutdown.
func sendStop(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// sendKill sends SIGKILL to pid for an immediate forced termination.
func sendKill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
