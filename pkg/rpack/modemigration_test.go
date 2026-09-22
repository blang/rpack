package rpack

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- parseLegacyMode (old octal grammar, migration-only) ---------------------

func TestParseLegacyMode(t *testing.T) {
	t.Run("accepts old octal grammar", func(t *testing.T) {
		cases := []struct { //nolint:govet // fieldalignment is not critical in tests
			in   string
			want os.FileMode
		}{
			{"600", 0o600},
			{"750", 0o750},
			{"644", 0o644},
			{"755", 0o755},
			{"0755", 0o755},
			{"0644", 0o644},
			{"000", 0o000},
		}
		for _, tc := range cases {
			t.Run(tc.in, func(t *testing.T) {
				got, err := parseLegacyMode(tc.in)
				if err != nil {
					t.Fatalf("parseLegacyMode(%q) failed: %v", tc.in, err)
				}
				if got != tc.want {
					t.Errorf("parseLegacyMode(%q) = %o, want %o", tc.in, got, tc.want)
				}
			})
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, in := range []string{"", "abc", "999", "75", "0o600", "4755", "00644", "600x"} {
			t.Run(in, func(t *testing.T) {
				if _, err := parseLegacyMode(in); err == nil {
					t.Fatalf("parseLegacyMode(%q) must fail", in)
				}
			})
		}
	})

	t.Run("special bits rejected with dedicated message", func(t *testing.T) {
		_, err := parseLegacyMode("4755")
		if err == nil {
			t.Fatal("parseLegacyMode(\"4755\") must fail")
		}
		if want := "special mode bits"; !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q, got: %v", want, err)
		}
	})
}

// --- MigrateLockfileModes ----------------------------------------------------

// migEntry describes one lockfile fixture entry: a file on disk with a
// concrete mode and a lockfile record with a concrete recorded mode.
type migEntry struct {
	path     string
	content  string
	recorded string
	diskMode os.FileMode
}

// setupMigrationEnv creates a config, lockfile, and managed files in dir.
// It returns the config path and lockfile path.
func setupMigrationEnv(t *testing.T, dir string, entries []migEntry) (configPath, lockPath string) {
	t.Helper()

	configPath = filepath.Join(dir, "app.rpack.yaml")
	configContent := "\"@schema_version\": \"v1\"\nsource: \"./def\"\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}

	lf := NewRPackLockFile()
	for _, e := range entries {
		p := filepath.Join(dir, e.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // test fixture dirs
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(e.content), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		lf.AddFile(e.path, calculateSHA256(t, p), e.recorded)
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

// lockfileBytes snapshots the on-disk lockfile so refusal tests can prove it
// was not rewritten.
func lockfileBytes(t *testing.T, lockPath string) []byte {
	t.Helper()
	b, err := os.ReadFile(lockPath) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// diskPerm returns the on-disk permission bits of a managed file.
func diskPerm(t *testing.T, dir, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func requireLockfileUnchanged(t *testing.T, lockPath string, before []byte) {
	t.Helper()
	after := lockfileBytes(t, lockPath)
	if !bytes.Equal(after, before) {
		t.Fatalf("lockfile must not be rewritten on refusal:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

//nolint:gocognit,gocyclo // test: independent subtest scenarios
func TestMigrateLockfileModes(t *testing.T) {
	t.Run("migrates recorded legacy modes by owner-execute intent", func(t *testing.T) {
		dir := t.TempDir()
		configPath, _ := setupMigrationEnv(t, dir, []migEntry{
			{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
			{path: "deploy.sh", content: "#!/bin/sh\n", recorded: "750", diskMode: 0o750},
			{path: "notes.txt", content: "notes\n", recorded: "644", diskMode: 0o644},
			{path: "unknown.txt", content: "unknown\n", recorded: "", diskMode: 0o600},
		})

		entries, err := MigrateLockfileModes(configPath, "")
		if err != nil {
			t.Fatalf("MigrateLockfileModes: %v", err)
		}

		if len(entries) != 2 {
			t.Fatalf("entries = %+v, want secret.conf and deploy.sh only", entries)
		}
		if entries[0].Path != "secret.conf" || entries[0].OldMode != "600" || entries[0].NewMode != "644" {
			t.Errorf("entries[0] = %+v, want secret.conf 600 -> 644", entries[0])
		}
		if entries[1].Path != "deploy.sh" || entries[1].OldMode != "750" || entries[1].NewMode != "755" {
			t.Errorf("entries[1] = %+v, want deploy.sh 750 -> 755", entries[1])
		}

		// The lockfile on disk records canonical modes now.
		ci, err := LoadRPackConfig(configPath)
		if err != nil {
			t.Fatal(err)
		}
		modes := map[string]string{}
		for _, f := range ci.LockFile.Files {
			modes[f.Path] = f.Mode
		}
		if modes["secret.conf"] != "644" || modes["deploy.sh"] != "755" {
			t.Errorf("migrated lockfile modes = %v", modes)
		}
		// Unknown stays unknown, canonical entries untouched.
		if modes["unknown.txt"] != "" {
			t.Errorf("missing mode must stay unknown, got %q", modes["unknown.txt"])
		}
		if modes["notes.txt"] != "644" {
			t.Errorf("already-canonical entry must stay 644, got %q", modes["notes.txt"])
		}

		// On-disk modes and bytes are unchanged: migration never chmods.
		if got := diskPerm(t, dir, "secret.conf"); got != 0o600 {
			t.Errorf("secret.conf on-disk mode = %o, want 600 (no chmod)", got)
		}
		if got := diskPerm(t, dir, "deploy.sh"); got != 0o750 {
			t.Errorf("deploy.sh on-disk mode = %o, want 750 (no chmod)", got)
		}
		if got := diskPerm(t, dir, "unknown.txt"); got != 0o600 {
			t.Errorf("unknown.txt on-disk mode = %o, want 600 (no chmod)", got)
		}
		for _, e := range []migEntry{
			{path: "secret.conf", content: "private\n"},
			{path: "deploy.sh", content: "#!/bin/sh\n"},
		} {
			if got := calculateSHA256(t, filepath.Join(dir, e.path)); got != contentSha256(t, e.content) {
				t.Errorf("%s bytes changed by migration", e.path)
			}
		}
	})

	t.Run("noop does not rewrite lockfile", func(t *testing.T) {
		dir := t.TempDir()
		_, lockPath := setupMigrationEnv(t, dir, []migEntry{
			{path: "notes.txt", content: "notes\n", recorded: "644", diskMode: 0o644},
			{path: "unknown.txt", content: "unknown\n", recorded: "", diskMode: 0o600},
		})
		past := time.Now().Add(-time.Hour)
		if err := os.Chtimes(lockPath, past, past); err != nil {
			t.Fatal(err)
		}
		infoBefore, err := os.Stat(lockPath)
		if err != nil {
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(filepath.Join(dir, "app.rpack.yaml"), "")
		if err != nil {
			t.Fatalf("noop migration must succeed: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("entries = %+v, want none", entries)
		}

		infoAfter, err := os.Stat(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
			t.Fatalf("lockfile was rewritten by a no-op migration (mtime %v -> %v)",
				infoBefore.ModTime(), infoAfter.ModTime())
		}
	})

	t.Run("content drift refuses without lockfile change", func(t *testing.T) {
		dir := t.TempDir()
		configPath, lockPath := setupMigrationEnv(t, dir, []migEntry{
			{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
		})
		before := lockfileBytes(t, lockPath)
		if err := os.WriteFile(filepath.Join(dir, "secret.conf"), []byte("tampered\n"), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(configPath, "")
		if err == nil {
			t.Fatalf("content drift must refuse migration, got entries %+v", entries)
		}
		if !strings.Contains(err.Error(), "modified files: secret.conf") {
			t.Errorf("error must name the modified file, got: %v", err)
		}
		requireLockfileUnchanged(t, lockPath, before)
	})

	t.Run("owner-execute drift refuses and cannot be blessed", func(t *testing.T) {
		// Recorded 750 (executable intent) but on disk 0644: canonicalizing
		// the record to 755 would bless the on-disk owner-execute removal.
		dir := t.TempDir()
		configPath, lockPath := setupMigrationEnv(t, dir, []migEntry{
			{path: "deploy.sh", content: "#!/bin/sh\n", recorded: "750", diskMode: 0o750},
		})
		before := lockfileBytes(t, lockPath)
		if err := os.Chmod(filepath.Join(dir, "deploy.sh"), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(configPath, "")
		if err == nil {
			t.Fatalf("owner-execute drift must refuse migration, got entries %+v", entries)
		}
		if !strings.Contains(err.Error(), "executable intent changed") {
			t.Errorf("error must describe the executable-intent drift, got: %v", err)
		}
		requireLockfileUnchanged(t, lockPath, before)

		// Reverse direction: recorded 600 (non-executable intent) but on
		// disk owner-executable.
		dir2 := t.TempDir()
		configPath2, lockPath2 := setupMigrationEnv(t, dir2, []migEntry{
			{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
		})
		before2 := lockfileBytes(t, lockPath2)
		if err := os.Chmod(filepath.Join(dir2, "secret.conf"), 0o744); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		if _, err := MigrateLockfileModes(configPath2, ""); err == nil {
			t.Fatal("owner-execute addition must refuse migration")
		}
		requireLockfileUnchanged(t, lockPath2, before2)
	})

	t.Run("removed file refuses without lockfile change", func(t *testing.T) {
		dir := t.TempDir()
		configPath, lockPath := setupMigrationEnv(t, dir, []migEntry{
			{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
		})
		before := lockfileBytes(t, lockPath)
		if err := os.Remove(filepath.Join(dir, "secret.conf")); err != nil {
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(configPath, "")
		if err == nil {
			t.Fatalf("removed managed file must refuse migration, got entries %+v", entries)
		}
		if !strings.Contains(err.Error(), "removed files: secret.conf") {
			t.Errorf("error must name the removed file, got: %v", err)
		}
		requireLockfileUnchanged(t, lockPath, before)
	})

	t.Run("invalid legacy mode refuses without lockfile change", func(t *testing.T) {
		dir := t.TempDir()
		configPath, lockPath := setupMigrationEnv(t, dir, []migEntry{
			{path: "broken.conf", content: "broken\n", recorded: "abc", diskMode: 0o600},
		})
		before := lockfileBytes(t, lockPath)

		entries, err := MigrateLockfileModes(configPath, "")
		if err == nil {
			t.Fatalf("invalid recorded mode must refuse migration, got entries %+v", entries)
		}
		if !strings.Contains(err.Error(), "invalid mode") {
			t.Errorf("error must clearly reject the invalid value, got: %v", err)
		}
		requireLockfileUnchanged(t, lockPath, before)
	})

	t.Run("missing lockfile refused", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "app.rpack.yaml")
		if err := os.WriteFile(configPath, []byte("\"@schema_version\": \"v1\"\nsource: \"./def\"\n"), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(configPath, "")
		if err == nil {
			t.Fatalf("missing lockfile must refuse, got entries %+v", entries)
		}
		if !strings.Contains(err.Error(), "no lockfile found") {
			t.Errorf("error must state the missing lockfile, got: %v", err)
		}
	})

	t.Run("working dir override", func(t *testing.T) {
		dir := t.TempDir()
		confDir := filepath.Join(dir, "conf")
		targetDir := filepath.Join(dir, "target")
		if err := os.MkdirAll(confDir, 0o755); err != nil { //nolint:gosec // test fixture dirs
			t.Fatal(err)
		}
		if err := os.MkdirAll(targetDir, 0o755); err != nil { //nolint:gosec // test fixture dirs
			t.Fatal(err)
		}
		configPath, lockPath := setupMigrationEnv(t, targetDir, []migEntry{
			{path: "secret.conf", content: "private\n", recorded: "600", diskMode: 0o600},
		})
		// Move config + lockfile into confDir; files stay in targetDir.
		if err := os.Rename(configPath, filepath.Join(confDir, "app.rpack.yaml")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(lockPath, filepath.Join(confDir, "app.rpack.lock.yaml")); err != nil {
			t.Fatal(err)
		}

		entries, err := MigrateLockfileModes(filepath.Join(confDir, "app.rpack.yaml"), targetDir)
		if err != nil {
			t.Fatalf("MigrateLockfileModes with working-dir override: %v", err)
		}
		if len(entries) != 1 || entries[0].OldMode != "600" || entries[0].NewMode != "644" {
			t.Fatalf("entries = %+v, want secret.conf 600 -> 644", entries)
		}
		if got := diskPerm(t, targetDir, "secret.conf"); got != 0o600 {
			t.Errorf("secret.conf on-disk mode = %o, want 600 (no chmod)", got)
		}
	})
}

// contentSha256 hashes plain content, for proving managed-file bytes are
// untouched by migration.
func contentSha256(t *testing.T, content string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}
