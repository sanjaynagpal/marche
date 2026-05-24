package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// runCLIInit scans COMPONENT_ROOT for service-like folders that do not yet have
// any representation in the central launchpad-registry.yaml, then seeds the
// file with best-effort entries for each new folder.
//
// The command is safe to run repeatedly:
//   - Folders already in the registry (complete or partial) are never touched.
//   - Folders that qualify under Tier 1 (launchpad.yaml) or Tier 3 (bin/run.*)
//     are skipped — they do not need a registry entry.
//   - Only genuinely new folders produce output and file changes.
//
// Detection heuristics applied per candidate folder:
//   - Start script: bin/ file whose name contains "run" or "start" (not "stop");
//     if exactly one non-stop script exists it is used unconditionally.
//   - Stop script: bin/ file whose name contains "stop"; blank if none found.
//   - PID file: any *.pid already in the service root; falls back to <folder>.pid.
//   - Name / port: cfg/application-${env}.yaml if present; blank otherwise.
//
// Fields that cannot be inferred are written as commented-out # TODO lines so
// the operator can locate them quickly.
func runCLIInit(cfg Config) int {
	registryPath := filepath.Join(cfg.ComponentRoot, "launchpad-registry.yaml")
	freshFile := !isRegularFile(registryPath)

	// Two complementary views of what is already in the registry:
	//   registry   — parsed map; only entries with a valid pid.file are present.
	//   inFile     — text-level set of top-level section names; covers partial or
	//                work-in-progress entries the user has started but not finished.
	// findLegacyCandidates uses inFile as the gate so that an incomplete section
	// the user is editing is never duplicated on a subsequent init run.
	registry := loadCentralRegistry(cfg.ComponentRoot)
	inFile := loadExistingSections(registryPath)

	candidates, err := findLegacyCandidates(cfg.ComponentRoot, registry, inFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}

	if len(candidates) == 0 {
		fmt.Println("No new legacy service folders found.")
		fmt.Println("Every folder under COMPONENT_ROOT already qualifies under a detection tier")
		fmt.Println("or already has a section in the registry.")
		return exitOK
	}

	fmt.Printf("Found %d new legacy folder(s):\n\n", len(candidates))

	newEntries := make(map[string]LaunchpadConfig)
	for _, folderPath := range candidates {
		folderName := filepath.Base(folderPath)
		inferred := inferLaunchpadConfig(folderPath, cfg.Env)
		newEntries[folderName] = inferred
		printInferredEntry(folderName, folderPath, inferred, cfg.ComponentRoot)
	}

	if err := appendToRegistry(registryPath, newEntries, freshFile); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror writing registry: %v\n", err)
		return exitError
	}

	fmt.Printf("Seeded %d entry/entries into:\n  %s\n\n", len(newEntries), registryPath)
	fmt.Println("Review the file and adjust entries marked # TODO before running launchpad.")
	return exitOK
}

// loadExistingSections reads the registry file as text and returns the set of
// all top-level YAML section names (folder names) already present, regardless
// of whether their entries are complete. This prevents init from duplicating a
// section the user has started editing but not yet finished (e.g. pid.file is
// still a # TODO placeholder — the parsed registry map would miss that entry,
// but the text scan finds it and protects it from being overwritten).
//
// Returns an empty map when the file does not exist or cannot be read.
func loadExistingSections(path string) map[string]bool {
	seen := make(map[string]bool)
	f, err := os.Open(path)
	if err != nil {
		return seen
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		// Skip blank lines, comments, and indented sub-key lines.
		if line == "" || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		// A non-indented line ending with ":" is a top-level YAML key.
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSuffix(trimmed, ":")
			if name != "" {
				seen[name] = true
			}
		}
	}
	return seen
}

// findLegacyCandidates returns folders that do not qualify under any detection
// tier and do not already have a section in the registry file, but contain a
// bin/ subdirectory indicating a possible legacy service.
//
// Two skip conditions are combined:
//   - qualifiesAsService: the folder is already managed (Tier 1/2/3).
//   - inFile: the folder already has a section in the registry file, even if
//     that section is incomplete (e.g. pid.file still a # TODO). This prevents
//     init from duplicating user-edited entries on subsequent runs.
//
// Aggregator expansion (one level deep) mirrors the logic in DiscoverServices.
func findLegacyCandidates(root string, registry map[string]LaunchpadConfig, inFile map[string]bool) ([]string, error) {
	topDirs, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading COMPONENT_ROOT %q: %w", root, err)
	}

	skip := func(dir string) bool {
		return qualifiesAsService(dir, registry) || inFile[filepath.Base(dir)]
	}

	var candidates []string
	for _, de := range topDirs {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(root, de.Name())
		if skip(dir) {
			continue
		}
		if hasBinDir(dir) {
			candidates = append(candidates, dir)
			continue
		}
		// No bin/ at this level — expand one level for aggregators.
		subDirs, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, subDe := range subDirs {
			if !subDe.IsDir() {
				continue
			}
			subDir := filepath.Join(dir, subDe.Name())
			if skip(subDir) {
				continue
			}
			if hasBinDir(subDir) {
				candidates = append(candidates, subDir)
			}
		}
	}

	sort.Strings(candidates)
	return candidates, nil
}

// hasBinDir reports whether dir contains a bin/ subdirectory.
func hasBinDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "bin"))
	return err == nil && info.IsDir()
}

// inferLaunchpadConfig scans a legacy service folder and returns a best-effort
// LaunchpadConfig. Fields that cannot be determined are left empty; they are
// written as # TODO placeholders in the registry.
func inferLaunchpadConfig(folderPath, env string) LaunchpadConfig {
	// Optional: read name/port from cfg YAML if present.
	name, port := loadServiceMeta(folderPath, env)

	// Collect scripts from bin/.
	binDir := filepath.Join(folderPath, "bin")
	var shFiles, ps1Files []string
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			switch strings.ToLower(filepath.Ext(e.Name())) {
			case ".sh":
				shFiles = append(shFiles, e.Name())
			case ".ps1":
				ps1Files = append(ps1Files, e.Name())
			}
		}
	}

	rel := func(script string) string {
		if script == "" {
			return ""
		}
		return "bin/" + script
	}

	return LaunchpadConfig{
		Name:         name,
		Port:         port,
		StartUnix:    rel(pickScript(shFiles, true)),
		StopUnix:     rel(pickScript(shFiles, false)),
		StartWindows: rel(pickScript(ps1Files, true)),
		StopWindows:  rel(pickScript(ps1Files, false)),
		PIDFileName:  inferPIDFile(folderPath),
	}
}

// pickScript selects the most likely start or stop script from a list of bare
// filenames (no path prefix). For start scripts it prefers names containing
// "run" or "start" (case-insensitive) that do not also contain "stop"; if
// exactly one non-stop script remains it is returned unconditionally. For stop
// scripts a name containing "stop" is required. Returns "" when no confident
// match can be made so the caller can emit a TODO placeholder.
func pickScript(scripts []string, isStart bool) string {
	if len(scripts) == 0 {
		return ""
	}

	if isStart {
		// Prefer: name contains "run" or "start", not "stop".
		for _, kw := range []string{"run", "start"} {
			for _, s := range scripts {
				lower := strings.ToLower(s)
				if strings.Contains(lower, kw) && !strings.Contains(lower, "stop") {
					return s
				}
			}
		}
		// One unambiguous non-stop script — use it.
		var nonStop []string
		for _, s := range scripts {
			if !strings.Contains(strings.ToLower(s), "stop") {
				nonStop = append(nonStop, s)
			}
		}
		if len(nonStop) == 1 {
			return nonStop[0]
		}
		return ""
	}

	// Stop: require the name to contain "stop".
	for _, s := range scripts {
		if strings.Contains(strings.ToLower(s), "stop") {
			return s
		}
	}
	return ""
}

// inferPIDFile returns the name of an existing *.pid file found directly in
// folderPath, or falls back to <foldername>.pid.
func inferPIDFile(folderPath string) string {
	if entries, err := os.ReadDir(folderPath); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pid") {
				return e.Name()
			}
		}
	}
	return filepath.Base(folderPath) + ".pid"
}

// printInferredEntry prints a human-readable summary of one inferred registry
// entry, noting which fields were detected and which need manual attention.
func printInferredEntry(folderName, folderPath string, cfg LaunchpadConfig, root string) {
	rel, _ := filepath.Rel(root, folderPath)
	fmt.Printf("  %s  (%s)\n", folderName, rel)
	printInitField("start (unix)", cfg.StartUnix)
	printInitField("start (windows)", cfg.StartWindows)
	printInitField("stop (unix)", cfg.StopUnix)
	printInitField("stop (windows)", cfg.StopWindows)
	printInitField("pid file", cfg.PIDFileName)
	if cfg.Name != "" {
		printInitField("name", cfg.Name)
	}
	if cfg.Port != "" {
		printInitField("port", cfg.Port)
	}
	fmt.Println()
}

func printInitField(label, value string) {
	const w = 18
	if value != "" {
		fmt.Printf("    %-*s  %s\n", w, label+":", value)
	} else {
		fmt.Printf("    %-*s  %s\n", w, label+":", "— not detected, set manually")
	}
}

// appendToRegistry appends new registry entries to path, creating the file if
// absent. A file header with field documentation is written when a new file is
// created; a section separator comment is used when appending to an existing
// file. Existing content is never modified.
func appendToRegistry(path string, entries map[string]LaunchpadConfig, freshFile bool) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var sb strings.Builder

	if freshFile {
		sb.WriteString("# launchpad-registry.yaml\n")
		sb.WriteString("# Central registry for legacy services managed by Launchpad.\n")
		sb.WriteString("# Generated by: launchpad init  —  review and adjust before use.\n")
		sb.WriteString("#\n")
		sb.WriteString("# Field reference (all script paths relative to service root):\n")
		sb.WriteString("#   name            Display name shown in status table (optional — used when cfg YAML absent)\n")
		sb.WriteString("#   port            Port shown in status table        (optional — used when cfg YAML absent)\n")
		sb.WriteString("#   start.unix      Start script, Linux/macOS\n")
		sb.WriteString("#   start.windows   Start script, Windows\n")
		sb.WriteString("#   stop.unix       Stop script,  Linux/macOS\n")
		sb.WriteString("#   stop.windows    Stop script,  Windows\n")
		sb.WriteString("#   pid.file        PID filename in service root (required)\n")
		sb.WriteString("\n")
	} else {
		sb.WriteString("\n# --- entries added by launchpad init ---\n\n")
	}

	// Deterministic output order.
	names := make([]string, 0, len(entries))
	for k := range entries {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		c := entries[name]
		sb.WriteString(name + ":\n")
		if c.Name != "" {
			sb.WriteString("  name: " + c.Name + "\n")
		}
		if c.Port != "" {
			sb.WriteString("  port: \"" + c.Port + "\"\n")
		}
		writeRegistryLine(&sb, "start.unix", c.StartUnix)
		writeRegistryLine(&sb, "start.windows", c.StartWindows)
		writeRegistryLine(&sb, "stop.unix", c.StopUnix)
		writeRegistryLine(&sb, "stop.windows", c.StopWindows)
		sb.WriteString("  pid.file: " + c.PIDFileName + "\n")
		sb.WriteString("\n")
	}

	_, err = f.WriteString(sb.String())
	return err
}

// writeRegistryLine writes one registry YAML key-value line. When value is
// empty a commented-out TODO placeholder is written instead so the operator
// can find and fill in the missing field.
func writeRegistryLine(sb *strings.Builder, key, value string) {
	if value != "" {
		sb.WriteString("  " + key + ": " + value + "\n")
	} else {
		sb.WriteString("  # " + key + ": # TODO — set path relative to service root\n")
	}
}
