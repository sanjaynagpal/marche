package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnsureLogsDir creates <service>/logs/ if it does not already exist.
func EnsureLogsDir(entry ServiceEntry) error {
	dir := filepath.Join(entry.FolderPath, "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating logs dir for %s: %w", entry.Name, err)
	}
	return nil
}

// OpenLogFiles opens (or creates) per-service stdout.log and stderr.log in
// append mode inside <service>/logs/. The caller is responsible for closing
// them after the child process has started.
func OpenLogFiles(entry ServiceEntry) (stdout, stderr *os.File, err error) {
	logsDir := filepath.Join(entry.FolderPath, "logs")
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND

	stdout, err = os.OpenFile(filepath.Join(logsDir, "stdout.log"), flags, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("opening stdout.log for %s: %w", entry.Name, err)
	}
	stderr, err = os.OpenFile(filepath.Join(logsDir, "stderr.log"), flags, 0644)
	if err != nil {
		stdout.Close()
		return nil, nil, fmt.Errorf("opening stderr.log for %s: %w", entry.Name, err)
	}
	return stdout, stderr, nil
}

// buildChildEnv assembles the environment slice for a child service process.
// It inherits the current process environment, then overrides LAUNCHPAD_ENV and
// LAUNCHPAD_CONFIG_DIR to point at the service's own cfg/ directory.
func buildChildEnv(entry ServiceEntry, env string) []string {
	cfgDir := filepath.Join(entry.FolderPath, "cfg")
	environ := os.Environ()
	environ = setEnvVar(environ, "LAUNCHPAD_ENV", env)
	environ = setEnvVar(environ, "LAUNCHPAD_CONFIG_DIR", cfgDir)
	return environ
}

// setEnvVar replaces the first occurrence of key=… in env with key=value,
// or appends it if key is not present.
func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// StartService starts entry as a background process.
//
// Standard services (IsCustom=false):
//
//	Launches bin/run.* as a detached background child, writes the child PID
//	to entry.PIDFile (.launchpad.pid), and returns immediately.
//
// Custom services (IsCustom=true):
//
//	Runs the registered start script synchronously — the script daemonises
//	the service and exits quickly. Launchpad then polls for entry.PIDFile
//	to appear within startTimeout seconds and reads the PID from it.
func StartService(entry ServiceEntry, env string, startTimeout int) (int, error) {
	if entry.IsCustom {
		return startCustomService(entry, env, startTimeout)
	}
	return startStandardService(entry, env)
}

// startStandardService launches a service as a detached child,
// captures its output to log files, and writes its PID to entry.PIDFile.
func startStandardService(entry ServiceEntry, env string) (int, error) {
	if err := EnsureLogsDir(entry); err != nil {
		return 0, err
	}
	outFile, errFile, err := OpenLogFiles(entry)
	if err != nil {
		return 0, err
	}
	// The child inherits these handles; close our copies after Start returns.
	defer outFile.Close()
	defer errFile.Close()

	childEnv := buildChildEnv(entry, env)
	cmd := buildStartCmd(entry, childEnv, outFile, errFile)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting %s: %w", entry.Name, err)
	}
	pid := cmd.Process.Pid
	if err := WritePID(entry, pid); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write pid file for %s: %v\n", entry.Name, err)
	}
	return pid, nil
}

// startCustomService runs the registered start script synchronously.
// The script daemonises the service process and exits quickly, writing its own
// PID file. Launchpad polls until that file appears and returns the PID.
func startCustomService(entry ServiceEntry, env string, timeoutSecs int) (int, error) {
	if entry.RunScript == "" {
		return 0, fmt.Errorf("no start script declared for %s on this platform", entry.Name)
	}
	childEnv := buildChildEnv(entry, env)
	cmd := buildRunCmd(entry.RunScript, entry.RunArgs, entry.FolderPath, childEnv)
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("start script for %s failed: %w", entry.Name, err)
	}
	return pollForPIDFile(entry.PIDFile, entry.Name, timeoutSecs)
}

// StopService stops a running service.
//
// Custom services (IsCustom=true):
//
//	Runs the registered stop script synchronously. The script terminates the
//	service and removes the PID file. Launchpad polls until the PID file
//	disappears or the process is no longer alive.
//
// Standard services (IsCustom=false):
//
//	Sends SIGTERM / Stop-Process directly to the stored PID, polling for
//	exit. Falls back to SIGKILL / Stop-Process -Force on timeout.
func StopService(entry ServiceEntry, timeoutSecs int) error {
	if entry.IsCustom {
		return stopCustomService(entry, timeoutSecs)
	}
	return stopStandardService(entry, timeoutSecs)
}

// stopCustomService runs the registered stop script and waits for the service
// to disappear (PID file gone and/or process dead).
func stopCustomService(entry ServiceEntry, timeoutSecs int) error {
	if entry.StopScript == "" {
		return fmt.Errorf("no stop script declared for %s on this platform", entry.Name)
	}
	// Snapshot the PID before the stop script runs so we can check liveness.
	pid, _ := ReadPID(entry)

	cmd := buildRunCmd(entry.StopScript, entry.StopArgs, entry.FolderPath, os.Environ())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stop script for %s failed: %w", entry.Name, err)
	}
	return pollUntilStopped(entry.PIDFile, entry.Name, pid, timeoutSecs)
}

// stopStandardService sends a graceful stop signal and falls back to a forced
// kill if the process has not exited within timeoutSecs.
func stopStandardService(entry ServiceEntry, timeoutSecs int) error {
	pid, err := ReadPID(entry)
	if err != nil {
		return fmt.Errorf("reading pid for %s: %w", entry.Name, err)
	}
	if pid == 0 || !IsAlive(pid) {
		_ = RemovePID(entry)
		return nil // already stopped
	}

	if err := sendStop(pid); err != nil {
		return fmt.Errorf("sending stop signal to %s (pid %d): %w", entry.Name, pid, err)
	}

	deadline := time.Duration(timeoutSecs) * time.Second
	interval := 500 * time.Millisecond
	elapsed := time.Duration(0)
	for elapsed < deadline {
		time.Sleep(interval)
		elapsed += interval
		if !IsAlive(pid) {
			_ = RemovePID(entry)
			return nil
		}
	}
	// Graceful shutdown timed out — force kill.
	_ = sendKill(pid)
	time.Sleep(200 * time.Millisecond)
	_ = RemovePID(entry)
	return nil
}

// pollForPIDFile polls until pidFile appears and contains a valid PID, or
// until timeoutSecs elapses. Used after running a daemonising start script.
func pollForPIDFile(pidFile, serviceName string, timeoutSecs int) (int, error) {
	deadline := time.Duration(timeoutSecs) * time.Second
	interval := 500 * time.Millisecond
	elapsed := time.Duration(0)
	for {
		if isRegularFile(pidFile) {
			if data, err := os.ReadFile(pidFile); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
					return pid, nil
				}
			}
		}
		if elapsed >= deadline {
			return 0, fmt.Errorf(
				"timed out after %ds waiting for %s to write its pid file (%s)",
				timeoutSecs, serviceName, pidFile)
		}
		time.Sleep(interval)
		elapsed += interval
	}
}

// pollUntilStopped waits until pidFile is gone or the process (pid) is no
// longer alive — whichever comes first. Used after a custom stop script runs.
// pid=0 means the PID was already unknown before the stop script ran.
func pollUntilStopped(pidFile, serviceName string, pid, timeoutSecs int) error {
	deadline := time.Duration(timeoutSecs) * time.Second
	interval := 500 * time.Millisecond
	elapsed := time.Duration(0)
	for elapsed < deadline {
		time.Sleep(interval)
		elapsed += interval
		pidFileGone := !isRegularFile(pidFile)
		processGone := pid == 0 || !IsAlive(pid)
		if pidFileGone || processGone {
			return nil
		}
	}
	return fmt.Errorf("service %s did not stop within %d seconds", serviceName, timeoutSecs)
}
