package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds launchpad's runtime configuration derived from environment
// variables. All fields are set by loadEnvConfig before any work begins.
type Config struct {
	ComponentRoot string // COMPONENT_ROOT — required
	Env           string // LAUNCHPAD_ENV — default "dev"
	StopTimeout   int    // LAUNCHPAD_STOP_TIMEOUT — default 10 seconds
	StartTimeout  int    // LAUNCHPAD_START_TIMEOUT — default 30 seconds (custom services only)
}

// loadEnvConfig reads and validates launchpad's environment configuration.
// Returns an error if COMPONENT_ROOT is unset or not an accessible directory.
func loadEnvConfig() (Config, error) {
	root := os.Getenv("COMPONENT_ROOT")
	if root == "" {
		return Config{}, fmt.Errorf("COMPONENT_ROOT is not set")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Config{}, fmt.Errorf("COMPONENT_ROOT=%q is not an accessible directory", root)
	}

	env := os.Getenv("LAUNCHPAD_ENV")
	if env == "" {
		env = "dev"
	}

	stopTimeout := 10
	if v := os.Getenv("LAUNCHPAD_STOP_TIMEOUT"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			stopTimeout = t
		}
	}

	startTimeout := 30
	if v := os.Getenv("LAUNCHPAD_START_TIMEOUT"); v != "" {
		if t, err := strconv.Atoi(v); err == nil && t > 0 {
			startTimeout = t
		}
	}

	return Config{
		ComponentRoot: root,
		Env:           env,
		StopTimeout:   stopTimeout,
		StartTimeout:  startTimeout,
	}, nil
}

func main() {
	cfg, err := loadEnvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// No subcommand → interactive mode.
	if len(os.Args) < 2 {
		runInteractive(cfg)
		return
	}

	sub := strings.ToLower(os.Args[1])
	switch sub {
	case "status":
		os.Exit(runCLIStatus(cfg))
	case "start":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: launchpad start <name>\n")
			os.Exit(exitError)
		}
		os.Exit(runCLIStart(cfg, os.Args[2]))
	case "stop":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: launchpad stop <name>\n")
			os.Exit(exitError)
		}
		os.Exit(runCLIStop(cfg, os.Args[2]))
	case "start-all":
		os.Exit(runCLIStartAll(cfg))
	case "stop-all":
		os.Exit(runCLIStopAll(cfg))
	case "init":
		os.Exit(runCLIInit(cfg))
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", sub)
		printUsage()
		os.Exit(exitError)
	}
}

// runInteractive enters the full-screen interactive command loop.
//
// On each iteration the status table is always redrawn from a fresh status
// poll so the display stays consistent after every command or error.
// Any deferred error message from the previous command is shown below the
// table before the prompt.
func runInteractive(cfg Config) {
	entries, err := DiscoverServices(cfg.ComponentRoot, cfg.Env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering services: %v\n", err)
		os.Exit(exitError)
	}
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no services found under COMPONENT_ROOT=%s\n", cfg.ComponentRoot)
		os.Exit(exitError)
	}

	var lastErr string
	reader := bufio.NewReader(os.Stdin)

	for {
		statuses := RefreshAll(entries)
		RenderTable(statuses, cfg.ComponentRoot, cfg.Env)
		if lastErr != "" {
			PrintError(lastErr)
			lastErr = ""
		}

		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF (Ctrl-D) or read error — exit cleanly.
			fmt.Println()
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "q", "quit":
			fmt.Println("Exiting. Running services continue in the background.")
			return

		case "r", "refresh":
			// The table is always redrawn at the top of the loop; just continue.

		case "s", "start":
			if len(parts) < 2 {
				lastErr = "usage: start <#>"
				continue
			}
			idx, ok := parseIndex(parts[1], len(statuses))
			if !ok {
				lastErr = fmt.Sprintf("invalid index %q — enter a number between 1 and %d", parts[1], len(statuses))
				continue
			}
			if statuses[idx].State == "RUNNING" {
				lastErr = fmt.Sprintf("%s is already RUNNING", statuses[idx].Entry.Name)
				continue
			}
			if _, err := StartService(statuses[idx].Entry, cfg.Env, cfg.StartTimeout); err != nil {
				lastErr = fmt.Sprintf("start failed: %v", err)
			}

		case "k", "kill":
			if len(parts) < 2 {
				lastErr = "usage: kill <#>"
				continue
			}
			idx, ok := parseIndex(parts[1], len(statuses))
			if !ok {
				lastErr = fmt.Sprintf("invalid index %q — enter a number between 1 and %d", parts[1], len(statuses))
				continue
			}
			if statuses[idx].State == "STOPPED" {
				lastErr = fmt.Sprintf("%s is already STOPPED", statuses[idx].Entry.Name)
				continue
			}
			if err := StopService(statuses[idx].Entry, cfg.StopTimeout); err != nil {
				lastErr = fmt.Sprintf("stop failed: %v", err)
			}

		default:
			lastErr = fmt.Sprintf("unknown command %q — use: start <#>  kill <#>  refresh  quit", cmd)
		}
	}
}

// parseIndex converts a 1-based string index (as typed by the user) to a
// 0-based slice index. Returns (index, true) on success or (0, false) on
// invalid input or out-of-range value.
func parseIndex(s string, length int) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > length {
		return 0, false
	}
	return n - 1, true
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  launchpad                  Interactive console (default)
  launchpad status           Print status of all services (tab-separated)
  launchpad start <name>     Start a specific service by name
  launchpad stop  <name>     Stop a specific service by name
  launchpad start-all        Start all stopped services
  launchpad stop-all         Stop all running services
  launchpad init             Seed launchpad-registry.yaml for legacy service folders

Environment:
  COMPONENT_ROOT           (required) Root directory containing installed service folders
  LAUNCHPAD_ENV            (default: dev) Environment profile forwarded to each service
  LAUNCHPAD_STOP_TIMEOUT   (default: 10) Seconds to wait for graceful shutdown before forced kill
  LAUNCHPAD_START_TIMEOUT  (default: 30) Seconds to wait for a custom service to write its pid file
`)
}
