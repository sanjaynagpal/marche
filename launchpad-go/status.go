package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ServiceStatus pairs a discovered service with its live process state.
type ServiceStatus struct {
	Entry ServiceEntry
	State string // "RUNNING" or "STOPPED"
	PID   int    // 0 when STOPPED
}

// ReadPID reads and returns the PID from entry.PIDFile.
// Returns 0 and nil when the file does not exist (service not started or
// stop script has already removed it).
func ReadPID(entry ServiceEntry) (int, error) {
	data, err := os.ReadFile(entry.PIDFile)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading pid file for %s: %w", entry.Name, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("malformed pid file for %s: %w", entry.Name, err)
	}
	return pid, nil
}

// WritePID writes pid to entry.PIDFile.
// Called only for standard services — custom services write their own PID
// file via their start script.
func WritePID(entry ServiceEntry, pid int) error {
	return os.WriteFile(entry.PIDFile, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

// RemovePID deletes entry.PIDFile. No-op if the file does not exist.
// Called only for standard services — custom services clean up their own
// PID file via their stop script.
func RemovePID(entry ServiceEntry) error {
	err := os.Remove(entry.PIDFile)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CheckStatus determines whether entry is RUNNING or STOPPED by reading its
// PID file and probing whether the stored PID is alive.
//
// Stale PID file handling: for standard services, a stale .launchpad.pid is
// removed automatically. For custom services, launchpad never touches the PID
// file — the service's own scripts own it.
func CheckStatus(entry ServiceEntry) ServiceStatus {
	pid, err := ReadPID(entry)
	if err != nil || pid == 0 {
		return ServiceStatus{Entry: entry, State: "STOPPED", PID: 0}
	}
	if IsAlive(pid) {
		return ServiceStatus{Entry: entry, State: "RUNNING", PID: pid}
	}
	// Process is not alive. Remove the stale file only for standard services.
	if !entry.IsCustom {
		_ = RemovePID(entry)
	}
	return ServiceStatus{Entry: entry, State: "STOPPED", PID: 0}
}

// RefreshAll refreshes the status of every entry in entries.
func RefreshAll(entries []ServiceEntry) []ServiceStatus {
	statuses := make([]ServiceStatus, len(entries))
	for i, e := range entries {
		statuses[i] = CheckStatus(e)
	}
	return statuses
}
