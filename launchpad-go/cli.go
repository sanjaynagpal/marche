package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CLI exit codes as defined in the specification.
const (
	exitOK       = 0 // success
	exitError    = 1 // general error (bad args, env, discovery failure)
	exitNotFound = 2 // named service does not exist
	exitOpFailed = 3 // start or stop operation failed
)

// runCLIStatus discovers all services, refreshes their statuses, and prints
// one tab-separated line per service to stdout:
//
//	<name> TAB <state> TAB <port> TAB <pid>
//
// Port and PID are "-" when not applicable.
func runCLIStatus(cfg Config) int {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	statuses := RefreshAll(entries)
	for _, s := range statuses {
		pid := "-"
		if s.PID > 0 {
			pid = fmt.Sprintf("%d", s.PID)
		}
		port := s.Entry.Port
		if port == "" {
			port = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", s.Entry.Name, s.State, port, pid)
	}
	return exitOK
}

// runCLIStart starts the named service. Returns exitNotFound if the service
// cannot be located, exitOpFailed on a start error, exitOK otherwise
// (including when the service is already running).
func runCLIStart(cfg Config, name string) int {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	entry, ok := findByName(entries, name)
	if !ok {
		fmt.Fprintf(os.Stderr, "service not found: %q\n", name)
		return exitNotFound
	}
	status := CheckStatus(entry)
	if status.State == "RUNNING" {
		fmt.Printf("%s is already RUNNING (pid %d)\n", entry.Name, status.PID)
		return exitOK
	}
	pid, err := StartService(entry, cfg.Env, cfg.StartTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start %s: %v\n", entry.Name, err)
		return exitOpFailed
	}
	fmt.Printf("started %s (pid %d)\n", entry.Name, pid)
	return exitOK
}

// runCLIStop stops the named service. Returns exitNotFound if the service
// cannot be located, exitOpFailed on a stop error, exitOK otherwise
// (including when the service is already stopped).
func runCLIStop(cfg Config, name string) int {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	entry, ok := findByName(entries, name)
	if !ok {
		fmt.Fprintf(os.Stderr, "service not found: %q\n", name)
		return exitNotFound
	}
	status := CheckStatus(entry)
	if status.State == "STOPPED" {
		fmt.Printf("%s is already STOPPED\n", entry.Name)
		return exitOK
	}
	if err := StopService(entry, cfg.StopTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop %s: %v\n", entry.Name, err)
		return exitOpFailed
	}
	fmt.Printf("stopped %s\n", entry.Name)
	return exitOK
}

// runCLIStartAll starts every service that is currently STOPPED.
// Already-running services are skipped with a notice. Returns exitOpFailed
// if any start failed, exitOK otherwise.
func runCLIStartAll(cfg Config) int {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	statuses := RefreshAll(entries)
	failed := 0
	for _, s := range statuses {
		if s.State == "RUNNING" {
			fmt.Printf("%s already RUNNING (pid %d) — skipped\n", s.Entry.Name, s.PID)
			continue
		}
		pid, err := StartService(s.Entry, cfg.Env, cfg.StartTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start %s: %v\n", s.Entry.Name, err)
			failed++
			continue
		}
		fmt.Printf("started %s (pid %d)\n", s.Entry.Name, pid)
	}
	if failed > 0 {
		return exitOpFailed
	}
	return exitOK
}

// runCLIStopAll stops every service that is currently RUNNING.
// Already-stopped services are skipped with a notice. Returns exitOpFailed
// if any stop failed, exitOK otherwise.
func runCLIStopAll(cfg Config) int {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	statuses := RefreshAll(entries)
	failed := 0
	for _, s := range statuses {
		if s.State == "STOPPED" {
			fmt.Printf("%s already STOPPED — skipped\n", s.Entry.Name)
			continue
		}
		if err := StopService(s.Entry, cfg.StopTimeout); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop %s: %v\n", s.Entry.Name, err)
			failed++
			continue
		}
		fmt.Printf("stopped %s\n", s.Entry.Name)
	}
	if failed > 0 {
		return exitOpFailed
	}
	return exitOK
}

// findByName locates a ServiceEntry by module name (launchpad.module) or by the
// base name of its folder. The comparison is case-insensitive.
func findByName(entries []ServiceEntry, name string) (ServiceEntry, bool) {
	lower := strings.ToLower(name)
	for _, e := range entries {
		if strings.ToLower(e.Name) == lower {
			return e, true
		}
		if strings.ToLower(filepath.Base(e.FolderPath)) == lower {
			return e, true
		}
	}
	return ServiceEntry{}, false
}
