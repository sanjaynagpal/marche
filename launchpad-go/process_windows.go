//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// createNewProcessGroup is the Windows flag that places the child in a new
// process group so that Ctrl-C signals sent to the launchpad console are not
// forwarded to the service process.
const createNewProcessGroup uint32 = 0x00000200

// buildStartCmd constructs the exec.Cmd for Windows platforms.
// The service is launched via PowerShell 7+ (pwsh), placed in a new process
// group so it survives launchpad exiting.
func buildStartCmd(entry ServiceEntry, childEnv []string, stdout, stderr *os.File) *exec.Cmd {
	cmd := exec.Command("pwsh", "-NonInteractive", "-File", entry.RunScript)
	cmd.Dir = entry.FolderPath
	cmd.Env = childEnv
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
	return cmd
}

// buildRunCmd constructs an exec.Cmd for running a custom script synchronously.
// args are appended after the -File scriptPath so a single script can handle
// both start and stop via different arguments (e.g. "runCMD.ps1 start" / "runCMD.ps1 stop").
func buildRunCmd(scriptPath string, args []string, workDir string, env []string) *exec.Cmd {
	pwshArgs := append([]string{"-NonInteractive", "-File", scriptPath}, args...)
	cmd := exec.Command("pwsh", pwshArgs...)
	cmd.Dir = workDir
	cmd.Env = env
	return cmd
}

// sendStop requests a graceful shutdown via PowerShell Stop-Process.
func sendStop(pid int) error {
	return exec.Command(
		"pwsh", "-NonInteractive", "-Command",
		fmt.Sprintf("Stop-Process -Id %d", pid),
	).Run()
}

// sendKill force-terminates the process via Stop-Process -Force.
func sendKill(pid int) error {
	return exec.Command(
		"pwsh", "-NonInteractive", "-Command",
		fmt.Sprintf("Stop-Process -Id %d -Force", pid),
	).Run()
}
