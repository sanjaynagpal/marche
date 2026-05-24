package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadExistingSections verifies that loadExistingSections reads top-level
// YAML keys correctly, including partial entries without pid.file.
func TestLoadExistingSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launchpad-registry.yaml")

	// File does not exist yet → empty map, no error.
	got := loadExistingSections(path)
	if len(got) != 0 {
		t.Errorf("missing file: expected empty map, got %v", got)
	}

	content := `# launchpad-registry.yaml
# comment line

complete-svc:
  start.unix: bin/run.sh
  stop.unix: bin/stop.sh
  pid.file: complete-svc.pid

partial-svc:
  start.unix: bin/runCMD.sh start
  # stop.unix: # TODO
  # pid.file: # TODO  ← user hasn't filled this in yet

user-edited-svc:
  start.unix: bin/manage.sh start
  stop.unix: bin/manage.sh stop
  pid.file: user-edited-svc.pid
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got = loadExistingSections(path)

	for _, want := range []string{"complete-svc", "partial-svc", "user-edited-svc"} {
		if !got[want] {
			t.Errorf("expected section %q to be present, got map: %v", want, got)
		}
	}
	// Comment lines and indented lines must not appear as sections.
	for _, bad := range []string{"# comment line", "start.unix", "stop.unix", "pid.file"} {
		if got[bad] {
			t.Errorf("section %q should not be present", bad)
		}
	}
}

// TestFindLegacyCandidatesIdempotent verifies that folders already present in
// the registry file (even with incomplete entries) are not returned as
// candidates on a second init run.
func TestFindLegacyCandidatesIdempotent(t *testing.T) {
	root := t.TempDir()

	// Folder A: complete registry entry — must be skipped.
	mkBinDir(t, root, "complete-svc")

	// Folder B: partial registry entry (no pid.file yet) — must also be skipped
	// so the user's edits are preserved.
	mkBinDir(t, root, "partial-svc")

	// Folder C: brand-new folder, not in file at all — must be returned.
	mkBinDir(t, root, "new-svc")

	// Folder D: has bin/run.ps1 (Tier 3) — qualifies already, must be skipped.
	mkBinDir(t, root, "standard-svc")
	touch(t, filepath.Join(root, "standard-svc", "bin", "run.ps1"))

	registry := map[string]LaunchpadConfig{
		"complete-svc": {PIDFileName: "complete-svc.pid"},
		// partial-svc is NOT in the parsed registry (no pid.file), but IS in inFile.
	}
	inFile := map[string]bool{
		"complete-svc": true,
		"partial-svc":  true,
		// new-svc intentionally absent.
	}

	candidates, err := findLegacyCandidates(root, registry, inFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	if filepath.Base(candidates[0]) != "new-svc" {
		t.Errorf("expected new-svc as candidate, got %q", filepath.Base(candidates[0]))
	}
}

// mkBinDir creates <root>/<name>/bin/ in the test temp tree.
func mkBinDir(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
}

// touch creates an empty file at path (directories must already exist).
func touch(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
