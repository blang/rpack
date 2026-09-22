package cmd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/rpack/pkg/rpack"
)

// cmdMigEntry describes one CLI-test fixture entry.
type cmdMigEntry struct {
	path     string
	content  string
	recorded string
	diskMode os.FileMode
}

// setupMigrateModesEnv creates a config, lockfile with legacy recorded
// modes, and managed files in dir, returning the config path and the
// lockfile path.
func setupMigrateModesEnv(t *testing.T, dir string, entries []cmdMigEntry) (configPath, lockPath string) {
	t.Helper()
	// Cobra commands are package singletons; do not let a prior invocation's
	// acknowledgment or working directory make a later invocation pass.
	resetFlags := func() {
		for _, name := range []string{"acknowledge-permission-change", "working-dir"} {
			flag := migrateModesCmd.Flags().Lookup(name)
			if err := flag.Value.Set(flag.DefValue); err != nil {
				t.Fatal(err)
			}
			flag.Changed = false
		}
	}
	resetFlags()
	t.Cleanup(resetFlags)

	configPath = filepath.Join(dir, "app.rpack.yaml")
	if err := os.WriteFile(configPath, []byte("\"@schema_version\": \"v1\"\nsource: \"./def\"\n"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}

	lf := rpack.NewRPackLockFile()
	for _, e := range entries {
		p := filepath.Join(dir, e.path)
		if err := os.WriteFile(p, []byte(e.content), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(e.content))
		lf.AddFile(e.path, fmt.Sprintf("%x", sum), e.recorded)
		if err := os.Chmod(p, e.diskMode); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}

	lockPath = filepath.Join(dir, "app.rpack.lock.yaml")
	if err := lf.WriteFile(lockPath); err != nil {
		t.Fatal(err)
	}
	return configPath, lockPath
}

func cmdLockfileUnchanged(t *testing.T, lockPath string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(lockPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("lockfile must not be rewritten:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func recordedModes(t *testing.T, configPath string) map[string]string {
	t.Helper()
	ci, err := rpack.LoadRPackConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	modes := map[string]string{}
	for _, f := range ci.LockFile.Files {
		modes[f.Path] = f.Mode
	}
	return modes
}

func onDiskPerm(t *testing.T, base, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(filepath.Join(base, path))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func legacyFixture() []cmdMigEntry {
	return []cmdMigEntry{
		{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
		{path: "deploy.sh", content: "#!/bin/sh\n", recorded: "750", diskMode: 0o750},
	}
}

// TestMigrateModesRequiresAcknowledgment asserts the command refuses to
// migrate unless --acknowledge-permission-change is actually true: mere flag
// presence (e.g. =false) or omission must never start the migration.
func TestMigrateModesRequiresAcknowledgment(t *testing.T) {
	cases := []struct { //nolint:govet // fieldalignment is not critical in tests
		name string
		args []string
	}{
		{"flag omitted", nil},
		{"flag explicitly false", []string{"--acknowledge-permission-change=false"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configPath, lockPath := setupMigrateModesEnv(t, t.TempDir(), legacyFixture())
			before, err := os.ReadFile(lockPath) //nolint:gosec // test fixture
			if err != nil {
				t.Fatal(err)
			}

			args := append([]string{"migrate-modes"}, tc.args...)
			args = append(args, configPath)
			_, _, runErr := runRoot(t, args...)
			if runErr == nil {
				t.Fatalf("expected refusal, got nil error")
			}
			if !strings.Contains(runErr.Error(), "--acknowledge-permission-change") {
				t.Fatalf("error must name the acknowledgment flag, got: %v", runErr)
			}
			cmdLockfileUnchanged(t, lockPath, before)
			if modes := recordedModes(t, configPath); modes["secret.conf"] != "600" {
				t.Fatalf("legacy mode must be untouched, got %v", modes)
			}
		})
	}
}

// TestMigrateModesHasNoForceFlag asserts the command has no --force: nothing
// about mode migration is force-bypassable.
func TestMigrateModesHasNoForceFlag(t *testing.T) {
	if f := migrateModesCmd.Flags().Lookup("force"); f != nil {
		t.Fatalf("migrate-modes must not register --force, got %+v", f)
	}
	configPath, _ := setupMigrateModesEnv(t, t.TempDir(), legacyFixture())
	_, stderr, err := runRoot(t, "migrate-modes", "--force", "--acknowledge-permission-change", configPath)
	if err == nil {
		t.Fatalf("expected unknown-flag error, got nil")
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("expected 'unknown flag' error, got: %q", stderr)
	}
}

// TestMigrateModesMigrates asserts the acknowledged happy path: per-entry
// old/new output, canonicalized lockfile, untouched files on disk.
func TestMigrateModesMigrates(t *testing.T) {
	configPath, _ := setupMigrateModesEnv(t, t.TempDir(), legacyFixture())
	dir := filepath.Dir(configPath)

	stdout, _, err := runRoot(t, "migrate-modes", "--acknowledge-permission-change", configPath)
	if err != nil {
		t.Fatalf("migrate-modes: %v", err)
	}

	for _, want := range []string{
		"WARNING",
		"secret.conf",
		"600 -> 644",
		"deploy.sh",
		"750 -> 755",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output must contain %q, got: %q", want, stdout)
		}
	}

	modes := recordedModes(t, configPath)
	if modes["secret.conf"] != "644" || modes["deploy.sh"] != "755" {
		t.Errorf("migrated modes = %v", modes)
	}
	if got := onDiskPerm(t, dir, "secret.conf"); got != 0o600 {
		t.Errorf("secret.conf on-disk mode = %o, want 600 (no chmod)", got)
	}
	if got := onDiskPerm(t, dir, "deploy.sh"); got != 0o750 {
		t.Errorf("deploy.sh on-disk mode = %o, want 750 (no chmod)", got)
	}
}

// TestMigrateModesNoop asserts a lockfile without legacy modes reports
// nothing to migrate without rewriting the lockfile.
func TestMigrateModesNoop(t *testing.T) {
	configPath, lockPath := setupMigrateModesEnv(t, t.TempDir(), []cmdMigEntry{
		{path: "notes.txt", content: "notes\n", recorded: "644", diskMode: 0o644},
	})
	before, err := os.ReadFile(lockPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runRoot(t, "migrate-modes", "--acknowledge-permission-change", configPath)
	if err != nil {
		t.Fatalf("noop migrate-modes: %v", err)
	}
	if !strings.Contains(stdout, "Nothing to migrate") {
		t.Errorf("output must report nothing to migrate, got: %q", stdout)
	}
	cmdLockfileUnchanged(t, lockPath, before)
}

// TestMigrateModesRefusesDrift asserts content drift refuses the migration
// through the CLI, without rewriting the lockfile.
func TestMigrateModesRefusesDrift(t *testing.T) {
	configPath, lockPath := setupMigrateModesEnv(t, t.TempDir(), legacyFixture())
	dir := filepath.Dir(configPath)
	before, err := os.ReadFile(lockPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.conf"), []byte("tampered\n"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}

	_, _, runErr := runRoot(t, "migrate-modes", "--acknowledge-permission-change", configPath)
	if runErr == nil {
		t.Fatal("drift must refuse migration even with acknowledgment")
	}
	if !strings.Contains(runErr.Error(), "secret.conf") {
		t.Errorf("error must name the drifted file, got: %v", runErr)
	}
	cmdLockfileUnchanged(t, lockPath, before)
}

// TestMigrateModesWorkingDir asserts the local -w/--working-dir flag
// relocates the managed files while the lockfile stays next to the config.
func TestMigrateModesWorkingDir(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "conf")
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test fixture dirs
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil { //nolint:gosec // test fixture dirs
		t.Fatal(err)
	}

	oldConfig, oldLock := setupMigrateModesEnv(t, targetDir, legacyFixture())
	configPath := filepath.Join(confDir, "app.rpack.yaml")
	if err := os.Rename(oldConfig, configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldLock, filepath.Join(confDir, "app.rpack.lock.yaml")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runRoot(t, "migrate-modes", "--acknowledge-permission-change", "-w", targetDir, configPath); err != nil {
		t.Fatalf("migrate-modes with -w: %v", err)
	}
	if modes := recordedModes(t, configPath); modes["secret.conf"] != "644" || modes["deploy.sh"] != "755" {
		t.Errorf("migrated modes = %v", modes)
	}
	if got := onDiskPerm(t, targetDir, "secret.conf"); got != 0o600 {
		t.Errorf("secret.conf on-disk mode = %o, want 600 (no chmod)", got)
	}
}
