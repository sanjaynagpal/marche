package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ServiceEntry holds the discovered metadata for a single installed service.
type ServiceEntry struct {
	Name       string   // launchpad.module from config, or folder name as fallback
	FolderPath string   // absolute path to the service root folder
	Port       string   // server.port from cfg YAML, or "" if absent
	RunScript  string   // absolute path to the platform-selected start script
	RunArgs    []string // optional arguments for RunScript (custom services only)
	StopScript string   // absolute path to the stop script; "" for standard services
	StopArgs   []string // optional arguments for StopScript (custom services only)
	PIDFile    string   // absolute path to the PID file (standard or custom)
	IsCustom   bool     // true when driven by launchpad.yaml or launchpad-registry.yaml
}

// LaunchpadConfig holds the per-service customisation declared either in a
// per-service launchpad.yaml or in the central launchpad-registry.yaml.
// All script paths are relative to the service root folder.
type LaunchpadConfig struct {
	Name         string // service.name (launchpad.yaml) or <folder>.name (registry) — optional
	Port         string // service.port (launchpad.yaml) or <folder>.port (registry) — optional
	StartUnix    string // start.unix
	StartWindows string // start.windows
	StopUnix     string // stop.unix
	StopWindows  string // stop.windows
	PIDFileName  string // pid.file  (required — entry is ignored without it)
}

// DiscoverServices scans root for installed services and returns them
// sorted alphabetically by Name.
//
// Detection uses a three-tier precedence for each folder:
//
//  1. Per-service launchpad.yaml in the service folder (highest — service override)
//  2. Entry in $COMPONENT_ROOT/launchpad-registry.yaml (Launchpad's central registry)
//  3. Standard bin/run.ps1 or bin/run.sh script (default for new-style services)
//
// Polaris-style aggregators (folders qualifying by none of the three signals
// but containing qualifying sub-folders one level deeper) are expanded
// transparently.
func DiscoverServices(root, env string) ([]ServiceEntry, error) {
	topDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading COMPONENT_ROOT %q: %w", root, err)
	}

	registry := loadCentralRegistry(root)

	var services []ServiceEntry
	for _, de := range topDirs {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(root, de.Name())
		if qualifiesAsService(dir, registry) {
			services = append(services, makeEntry(dir, env, registry))
			continue
		}
		// No qualifying signal at this level — expand one level for aggregators.
		subDirs, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, subDe := range subDirs {
			if !subDe.IsDir() {
				continue
			}
			subDir := filepath.Join(dir, subDe.Name())
			if qualifiesAsService(subDir, registry) {
				services = append(services, makeEntry(subDir, env, registry))
			}
		}
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
	return services, nil
}

// qualifiesAsService reports whether dir is a Launchpad-managed service folder by checking
// the three detection signals in precedence order.
func qualifiesAsService(dir string, registry map[string]LaunchpadConfig) bool {
	folderName := filepath.Base(dir)
	_, inRegistry := registry[folderName]
	return inRegistry ||
		isRegularFile(filepath.Join(dir, "launchpad.yaml")) ||
		resolveRunScript(dir) != ""
}

// makeEntry builds a ServiceEntry for folderPath, applying three-tier precedence:
//  1. Per-service launchpad.yaml
//  2. Central registry entry
//  3. Standard bin/run.* detection
//
// Service metadata (display name and port) follows its own fallback cascade:
// cfg/application-${env}.yaml → registration (launchpad.yaml or registry) → folder name / empty.
// Legacy services may have no cfg YAML at all; the registration fields cover that case.
func makeEntry(folderPath, env string, registry map[string]LaunchpadConfig) ServiceEntry {
	// Primary metadata source: cfg/application-${env}.yaml (may be absent for legacy services).
	name, port := loadServiceMeta(folderPath, env)

	// Tier 1: per-service launchpad.yaml overrides everything.
	if lpCfg := loadLaunchpadConfig(folderPath); lpCfg != nil {
		if name == "" {
			name = lpCfg.Name
		}
		if port == "" {
			port = lpCfg.Port
		}
		if name == "" {
			name = filepath.Base(folderPath)
		}
		return buildCustomEntry(name, folderPath, port, lpCfg)
	}

	// Tier 2: central registry entry overrides standard detection.
	if regCfg, ok := registry[filepath.Base(folderPath)]; ok {
		if name == "" {
			name = regCfg.Name
		}
		if port == "" {
			port = regCfg.Port
		}
		if name == "" {
			name = filepath.Base(folderPath)
		}
		return buildCustomEntry(name, folderPath, port, &regCfg)
	}

	// Tier 3: standard service — bin/run.* script, PID managed by launchpad.
	if name == "" {
		name = filepath.Base(folderPath)
	}
	return ServiceEntry{
		Name:       name,
		FolderPath: folderPath,
		Port:       port,
		RunScript:  resolveRunScript(folderPath),
		StopScript: "",
		PIDFile:    filepath.Join(folderPath, ".launchpad.pid"),
		IsCustom:   false,
	}
}

// buildCustomEntry constructs a custom ServiceEntry from a LaunchpadConfig.
// Used for both per-service launchpad.yaml and central registry entries.
func buildCustomEntry(name, folderPath, port string, lpCfg *LaunchpadConfig) ServiceEntry {
	runScript, runArgs := platformCustomCommand(folderPath, lpCfg.StartUnix, lpCfg.StartWindows)
	stopScript, stopArgs := platformCustomCommand(folderPath, lpCfg.StopUnix, lpCfg.StopWindows)
	return ServiceEntry{
		Name:       name,
		FolderPath: folderPath,
		Port:       port,
		RunScript:  runScript,
		RunArgs:    runArgs,
		StopScript: stopScript,
		StopArgs:   stopArgs,
		PIDFile:    filepath.Join(folderPath, lpCfg.PIDFileName),
		IsCustom:   true,
	}
}

// loadCentralRegistry reads $COMPONENT_ROOT/launchpad-registry.yaml and
// returns a map of service folder name → LaunchpadConfig.
//
// The registry uses the existing two-level YAML format with the service folder
// name as the section and dotted field names as keys:
//
//	legacy-svc:
//	  start.unix: bin/runCMD.sh
//	  start.windows: bin/runCMD.ps1
//	  stop.unix: bin/stopCMD.sh
//	  stop.windows: bin/stopCMD.ps1
//	  pid.file: legacy-svc.pid
//
// Services without a pid.file declaration are silently ignored.
// Returns an empty map when the registry file is absent.
func loadCentralRegistry(root string) map[string]LaunchpadConfig {
	registry := make(map[string]LaunchpadConfig)
	cfgPath := filepath.Join(root, "launchpad-registry.yaml")
	if !isRegularFile(cfgPath) {
		return registry
	}
	props := parseYAML(cfgPath)

	// Anchor on pid.file declarations to find every service in the registry.
	// Keys look like "folder-name.pid.file"; the folder name is the prefix.
	for key, pidFile := range props {
		if !strings.HasSuffix(key, ".pid.file") || pidFile == "" {
			continue
		}
		folderName := strings.TrimSuffix(key, ".pid.file")
		registry[folderName] = LaunchpadConfig{
			Name:         props[folderName+".name"],
			Port:         props[folderName+".port"],
			StartUnix:    props[folderName+".start.unix"],
			StartWindows: props[folderName+".start.windows"],
			StopUnix:     props[folderName+".stop.unix"],
			StopWindows:  props[folderName+".stop.windows"],
			PIDFileName:  pidFile,
		}
	}
	return registry
}

// loadLaunchpadConfig reads <folderPath>/launchpad.yaml and returns a
// *LaunchpadConfig when the file exists and declares a non-empty pid.file.
// Returns nil when the file is absent or the declaration is incomplete.
func loadLaunchpadConfig(folderPath string) *LaunchpadConfig {
	cfgPath := filepath.Join(folderPath, "launchpad.yaml")
	if !isRegularFile(cfgPath) {
		return nil
	}
	props := parseYAML(cfgPath)
	pidFile := props["pid.file"]
	if pidFile == "" {
		return nil // incomplete registration — ignore
	}
	return &LaunchpadConfig{
		Name:         props["service.name"],
		Port:         props["service.port"],
		StartUnix:    props["start.unix"],
		StartWindows: props["start.windows"],
		StopUnix:     props["stop.unix"],
		StopWindows:  props["stop.windows"],
		PIDFileName:  pidFile,
	}
}

// platformCustomCommand picks the platform-appropriate command string from
// unixCmd or windowsCmd, then delegates to parseCustomCommand to split it into
// an absolute script path and optional arguments.
// Returns ("", nil) when the selected command is empty.
func platformCustomCommand(folderPath, unixCmd, windowsCmd string) (string, []string) {
	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = windowsCmd
	} else {
		cmdStr = unixCmd
	}
	return parseCustomCommand(folderPath, cmdStr)
}

// parseCustomCommand splits a command string of the form
//
//	"rel/path/to/script [arg1 arg2 ...]"
//
// into an absolute script path and an optional argument slice.
// The first whitespace-separated token is the script path (resolved relative
// to folderPath); any remaining tokens are passed as arguments to the script.
// This lets a single script serve as both start and stop handler:
//
//	start.unix: bin/runCMD.sh start
//	stop.unix:  bin/runCMD.sh stop
//
// Returns ("", nil) when cmdStr is empty.
func parseCustomCommand(folderPath, cmdStr string) (string, []string) {
	if cmdStr == "" {
		return "", nil
	}
	parts := strings.Fields(cmdStr)
	scriptPath := filepath.Join(folderPath, parts[0])
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}
	return scriptPath, args
}

// resolveRunScript returns the absolute path to the platform-appropriate run
// script inside dir/bin/, or "" if none is found.
//
// All launch scripts and lib/ live under bin/ so executables are separated
// from cfg/ and logs/.
//
// Priority:
//   - Windows: bin/run.ps1 → bin/run.bat
//   - Unix:    bin/run.sh
func resolveRunScript(dir string) string {
	binDir := filepath.Join(dir, "bin")
	if runtime.GOOS == "windows" {
		for _, name := range []string{"run.ps1", "run.bat"} {
			if p := filepath.Join(binDir, name); isRegularFile(p) {
				return p
			}
		}
		return ""
	}
	if p := filepath.Join(binDir, "run.sh"); isRegularFile(p) {
		return p
	}
	return ""
}

// loadServiceMeta reads cfg/application-{env}.yaml from folderPath and
// returns the launchpad.module name and server.port values.
func loadServiceMeta(folderPath, env string) (name, port string) {
	cfgPath := filepath.Join(folderPath, "cfg", "application-"+env+".yaml")
	props := parseYAML(cfgPath)
	return props["launchpad.module"], props["server.port"]
}

// parseYAML performs a minimal two-level YAML parse. It handles lines of the form:
//
//	section:
//	  key: value
//
// and returns a map of "section.key" → "value". Keys may themselves contain
// dots (e.g. "start.unix", "pid.file") — only the first ": " on a line is
// treated as the key-value separator. Top-level keys without sub-keys and
// deeper nesting are ignored.
func parseYAML(path string) map[string]string {
	result := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()

	var section string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			trimmed := strings.TrimSpace(line)
			if strings.HasSuffix(trimmed, ":") {
				section = strings.TrimSuffix(trimmed, ":")
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(trimmed, ": "); idx >= 0 && section != "" {
			result[section+"."+trimmed[:idx]] = trimmed[idx+2:]
		}
	}
	return result
}

// isRegularFile reports whether path refers to an existing regular file.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
