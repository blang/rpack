package rpack

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
)

// calculateSHA256 reads the file at filePath, calculates its sha256 checksum,
// and returns it as a hex string.
func calculateSHA256(t *testing.T, filePath string) string {
	t.Helper()
	data, err := os.ReadFile(filePath) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("Failed to read file %q: %v", filePath, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

//nolint:gocognit,gocyclo // test: table-driven test with many cases
func TestRPackLockFileCheckIntegrity(t *testing.T) {
	// Create a temporary directory to simulate the file structure.
	tempDir := t.TempDir()

	t.Run("all files valid", func(t *testing.T) {
		// Create a valid file.
		fileName := "valid.txt"
		filePath := filepath.Join(tempDir, fileName)
		originalContent := []byte("original content")
		if err := os.WriteFile(filePath, originalContent, 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to create file %q: %v", filePath, err)
		}
		sha := calculateSHA256(t, filePath)

		// Create a lockfile entry with the correct checksum.
		lockFile := NewRPackLockFile()
		lockFile.AddFile(fileName, sha, "")

		// Check integrity.
		integrity, err := lockFile.CheckIntegrity(tempDir)
		if err != nil {
			t.Fatalf("CheckIntegrity failed: %v", err)
		}
		if len(integrity.Modified) != 0 {
			t.Errorf("Expected no modified files, got: %v", integrity.Modified)
		}
		if len(integrity.Removed) != 0 {
			t.Errorf("Expected no removed files, got: %v", integrity.Removed)
		}
	})

	t.Run("file missing", func(t *testing.T) {
		// Define a file that is not created.
		fileName := "missing.txt"
		// Provide a dummy checksum.
		dummySHA := "dummysha"

		lockFile := NewRPackLockFile()
		lockFile.AddFile(fileName, dummySHA, "")

		integrity, err := lockFile.CheckIntegrity(tempDir)
		if err != nil {
			t.Fatalf("CheckIntegrity failed: %v", err)
		}
		if len(integrity.Removed) != 1 || integrity.Removed[0] != fileName {
			t.Errorf("Expected removed file %q, got: %v", fileName, integrity.Removed)
		}
		if len(integrity.Modified) != 0 {
			t.Errorf("Expected no modified files, got: %v", integrity.Modified)
		}
	})

	t.Run("file modified", func(t *testing.T) {
		// Create a file that will be modified.
		fileName := "modified.txt"
		filePath := filepath.Join(tempDir, fileName)
		initialContent := []byte("initial")
		if err := os.WriteFile(filePath, initialContent, 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to create file %q: %v", filePath, err)
		}
		// Compute its initial checksum.
		sha := calculateSHA256(t, filePath)

		// Now modify the file.
		modifiedContent := []byte("modified content")
		if err := os.WriteFile(filePath, modifiedContent, 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to modify file %q: %v", filePath, err)
		}

		lockFile := NewRPackLockFile()
		lockFile.AddFile(fileName, sha, "")

		integrity, err := lockFile.CheckIntegrity(tempDir)
		if err != nil {
			t.Fatalf("CheckIntegrity failed: %v", err)
		}
		if len(integrity.Modified) != 1 || integrity.Modified[0] != fileName {
			t.Errorf("Expected modified file %q, got: %v", fileName, integrity.Modified)
		}
		if len(integrity.Removed) != 0 {
			t.Errorf("Expected no removed files, got: %v", integrity.Removed)
		}
	})

	t.Run("multiple files scenario", func(t *testing.T) {
		// valid file
		validFile := "valid2.txt"
		validPath := filepath.Join(tempDir, validFile)
		if err := os.WriteFile(validPath, []byte("content valid"), 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to create file %q: %v", validPath, err)
		}
		validSHA := calculateSHA256(t, validPath)

		// missing file (do not create)
		missingFile := "missing2.txt"

		// modified file: create then change it.
		modFile := "mod2.txt"
		modPath := filepath.Join(tempDir, modFile)
		if err := os.WriteFile(modPath, []byte("original mod"), 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to create file %q: %v", modPath, err)
		}
		modSHA := calculateSHA256(t, modPath)
		// Modify the file to simulate external change.
		if err := os.WriteFile(modPath, []byte("changed mod"), 0o644); err != nil { //nolint:gosec // test file
			t.Fatalf("Failed to modify file %q: %v", modPath, err)
		}

		// Build a lockfile with all three entries.
		lockFile := NewRPackLockFile()
		lockFile.AddFile(validFile, validSHA, "")
		lockFile.AddFile(missingFile, "dummy", "")
		lockFile.AddFile(modFile, modSHA, "")

		integrity, err := lockFile.CheckIntegrity(tempDir)
		if err != nil {
			t.Fatalf("CheckIntegrity failed: %v", err)
		}
		// Expect modFile to be flagged (modified) and missingFile to be flagged (removed)
		if len(integrity.Modified) != 1 || integrity.Modified[0] != modFile {
			t.Errorf("Expected modified file %q, got: %v", modFile, integrity.Modified)
		}
		if len(integrity.Removed) != 1 || integrity.Removed[0] != missingFile {
			t.Errorf("Expected removed file %q, got: %v", missingFile, integrity.Removed)
		}
	})
}

// sortStrings is a helper to sort a slice of strings.
func sortStrings(s []string) []string {
	sorted := append([]string(nil), s...)
	sort.Strings(sorted)
	return sorted
}

func TestRPackLockFileChanges(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		// Both old and new are identical.
		oldLF := NewRPackLockFile()
		oldLF.AddFile("a.txt", "sha1", "")
		oldLF.AddFile("b.txt", "sha2", "")

		newLF := NewRPackLockFile()
		newLF.AddFile("a.txt", "sha1", "")
		newLF.AddFile("b.txt", "sha2", "")

		changes := newLF.Changes(oldLF)

		if len(changes.Added) != 0 {
			t.Errorf("Expected no added files, got %v", changes.Added)
		}
		if len(changes.Removed) != 0 {
			t.Errorf("Expected no removed files, got %v", changes.Removed)
		}
	})

	t.Run("file added", func(t *testing.T) {
		// old lockfile has one file, new lockfile has that file plus one new file.
		oldLF := NewRPackLockFile()
		oldLF.AddFile("common.txt", "sha-common", "")

		newLF := NewRPackLockFile()
		newLF.AddFile("common.txt", "sha-common", "")
		newLF.AddFile("new.txt", "sha-new", "")

		changes := newLF.Changes(oldLF)
		added := sortStrings(changes.Added)
		removed := sortStrings(changes.Removed)

		expectedAdded := []string{"new.txt"}
		expectedRemoved := []string{}

		if !lo.ElementsMatch(added, expectedAdded) {
			t.Errorf("Expected added files %v, got %v", expectedAdded, added)
		}
		if !lo.ElementsMatch(removed, expectedRemoved) {
			t.Errorf("Expected removed files %v, got %v", expectedRemoved, removed)
		}
	})

	t.Run("file removed", func(t *testing.T) {
		// old lockfile has two files, new lockfile has only one.
		oldLF := NewRPackLockFile()
		oldLF.AddFile("a.txt", "sha-a", "")
		oldLF.AddFile("b.txt", "sha-b", "")

		newLF := NewRPackLockFile()
		newLF.AddFile("a.txt", "sha-a", "")

		changes := newLF.Changes(oldLF)
		added := sortStrings(changes.Added)
		removed := sortStrings(changes.Removed)

		expectedAdded := []string{}
		expectedRemoved := []string{"b.txt"}

		if !lo.ElementsMatch(added, expectedAdded) {
			t.Errorf("Expected added files %v, got %v", expectedAdded, added)
		}
		if !lo.ElementsMatch(removed, expectedRemoved) {
			t.Errorf("Expected removed files %v, got %v", expectedRemoved, removed)
		}
	})

	t.Run("files added and removed", func(t *testing.T) {
		// Old lockfile has files "a.txt" and "b.txt". New lockfile has "b.txt" (common)
		// plus "c.txt" as new.
		oldLF := NewRPackLockFile()
		oldLF.AddFile("a.txt", "sha-a", "")
		oldLF.AddFile("b.txt", "sha-b", "")

		newLF := NewRPackLockFile()
		newLF.AddFile("b.txt", "sha-b", "")
		newLF.AddFile("c.txt", "sha-c", "")

		changes := newLF.Changes(oldLF)
		added := sortStrings(changes.Added)
		removed := sortStrings(changes.Removed)

		expectedAdded := []string{"c.txt"}
		expectedRemoved := []string{"a.txt"}

		if !lo.ElementsMatch(added, expectedAdded) {
			t.Errorf("Expected added files %v, got %v", expectedAdded, added)
		}
		if !lo.ElementsMatch(removed, expectedRemoved) {
			t.Errorf("Expected removed files %v, got %v", expectedRemoved, removed)
		}
	})
}

// --- Mode integrity (ADR 0001) ----------------------------------------------

//nolint:gocognit,gocyclo // test: table of independent subtest scenarios
func TestRPackLockFileCheckIntegrity_Modes(t *testing.T) {
	// setup creates a file with an exact on-disk mode (chmod after write so
	// the process umask cannot silently narrow it, e.g. 0664 -> 0644).
	setup := func(t *testing.T, name string, mode os.FileMode) (dir, sha string) {
		t.Helper()
		dir = t.TempDir()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("content"), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		sha = calculateSHA256(t, p)
		if err := os.Chmod(p, mode); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		return dir, sha
	}

	t.Run("mode matches", func(t *testing.T) {
		dir, sha := setup(t, "f.sh", 0o755)
		lf := NewRPackLockFile()
		lf.AddFile("f.sh", sha, "755")
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(integrity.ModeModified) != 0 {
			t.Errorf("unexpected mode drift: %v", integrity.ModeModified)
		}
	})

	t.Run("mode drift reported as executable intent", func(t *testing.T) {
		dir, sha := setup(t, "f.sh", 0o644)
		lf := NewRPackLockFile()
		lf.AddFile("f.sh", sha, "755")
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(integrity.ModeModified) != 1 ||
			integrity.ModeModified[0] != "f.sh (expected executable (755), found non-executable (644))" {
			t.Errorf("ModeModified = %v, want executable-intent diagnostic", integrity.ModeModified)
		}
		if len(integrity.Modified) != 0 {
			t.Errorf("content-only change expected none, got %v", integrity.Modified)
		}
	})

	t.Run("owner execute added drift", func(t *testing.T) {
		dir, sha := setup(t, "f.txt", 0o744)
		lf := NewRPackLockFile()
		lf.AddFile("f.txt", sha, "644")
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(integrity.ModeModified) != 1 ||
			integrity.ModeModified[0] != "f.txt (expected non-executable (644), found executable (755))" {
			t.Errorf("ModeModified = %v, want executable-intent diagnostic", integrity.ModeModified)
		}
	})

	// Local read/write policy (umask, ACLs) and non-owner execute bits are
	// not drift: only owner-execute intent is compared (issue #15).
	t.Run("local permission policy drift accepted", func(t *testing.T) {
		cases := []struct { //nolint:govet // fieldalignment is not critical in tests
			name       string
			recorded   string
			onDiskMode os.FileMode
		}{
			{"record 644, on disk 664 (umask 0002)", "644", 0o664},
			{"record 644, on disk 600 (umask 0077)", "644", 0o600},
			{"record 644, on disk 654 (group-only execute)", "644", 0o654},
			{"record 755, on disk 700 (umask 0077)", "755", 0o700},
			{"record 755, on disk 775 (umask 0002)", "755", 0o775},
			{"record 755, on disk 751 (other-only execute)", "755", 0o751},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir, sha := setup(t, "f.bin", tc.onDiskMode)
				lf := NewRPackLockFile()
				lf.AddFile("f.bin", sha, tc.recorded)
				integrity, err := lf.CheckIntegrity(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(integrity.ModeModified) != 0 {
					t.Errorf("accepted drift flagged: %v", integrity.ModeModified)
				}
			})
		}
	})

	t.Run("absent mode skips check", func(t *testing.T) {
		dir, sha := setup(t, "f.sh", 0o600)
		lf := NewRPackLockFile()
		lf.AddFile("f.sh", sha, "") // pre-feature lockfile entry
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(integrity.ModeModified) != 0 {
			t.Errorf("absent mode must skip check, got %v", integrity.ModeModified)
		}
	})

	t.Run("removed wins over mode comparison", func(t *testing.T) {
		dir, _ := setup(t, "other.txt", 0o644)
		lf := NewRPackLockFile()
		lf.AddFile("ghost.sh", "dummysha", "755")
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(integrity.Removed) != 1 || integrity.Removed[0] != "ghost.sh" {
			t.Errorf("Removed = %v", integrity.Removed)
		}
		if len(integrity.ModeModified) != 0 {
			t.Errorf("removed file must not be mode-compared, got %v", integrity.ModeModified)
		}
	})

	t.Run("unreadable file classified as modified not error", func(t *testing.T) {
		dir, sha := setup(t, "secret.txt", 0o644)
		lf := NewRPackLockFile()
		lf.AddFile("secret.txt", sha, "644")
		// Out-of-band chmod 000: the file can be stat'ed but not hashed.
		if err := os.Chmod(filepath.Join(dir, "secret.txt"), 0o000); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "secret.txt"), 0o600) })
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatalf("unreadable file must not hard-error (force must be able to heal): %v", err)
		}
		if len(integrity.Modified) != 1 || integrity.Modified[0] != "secret.txt" {
			t.Errorf("Modified = %v, want secret.txt classified as drift", integrity.Modified)
		}
	})
}

// --- Legacy mode records (issue #15) ----------------------------------------

//nolint:gocognit // test: table of independent compatibility scenarios
func TestRPackLockFileCheckIntegrity_LegacyModes(t *testing.T) {
	setup := func(t *testing.T, name string, mode os.FileMode) (dir, sha string) {
		t.Helper()
		dir = t.TempDir()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("content"), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		return dir, calculateSHA256(t, p)
	}

	// Legacy records are classified by owner-execute just like canonical
	// records. Read/write and group/other execute differences are local policy.
	t.Run("legacy modes compare by executable intent", func(t *testing.T) {
		cases := []struct {
			recorded string
			onDisk   os.FileMode
		}{
			{"600", 0o664},
			{"640", 0o600},
			{"664", 0o644},
			{"700", 0o775},
			{"750", 0o700},
			{"0755", 0o751},
		}
		for _, tc := range cases {
			t.Run("recorded "+tc.recorded, func(t *testing.T) {
				dir, sha := setup(t, "legacy.conf", tc.onDisk)
				lf := NewRPackLockFile()
				lf.AddFile("legacy.conf", sha, tc.recorded)
				integrity, err := lf.CheckIntegrity(dir)
				if err != nil {
					t.Fatalf("legacy mode %q must remain compatible: %v", tc.recorded, err)
				}
				if len(integrity.ModeModified) != 0 {
					t.Errorf("legacy mode %q reported drift for %o: %v", tc.recorded, tc.onDisk, integrity.ModeModified)
				}
			})
		}
	})

	t.Run("legacy mode still detects executable drift", func(t *testing.T) {
		dir, sha := setup(t, "legacy.conf", 0o600)
		lf := NewRPackLockFile()
		lf.AddFile("legacy.conf", sha, "750")
		integrity, err := lf.CheckIntegrity(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := "legacy.conf (expected executable (755), found non-executable (644))"
		if len(integrity.ModeModified) != 1 || integrity.ModeModified[0] != want {
			t.Fatalf("ModeModified = %v, want %q", integrity.ModeModified, want)
		}
	})

	t.Run("invalid recorded mode rejected clearly", func(t *testing.T) {
		for _, mode := range []string{"abc", "999", "75", "4755", "000"} {
			t.Run("recorded "+mode, func(t *testing.T) {
				dir, sha := setup(t, "broken.conf", 0o600)
				lf := NewRPackLockFile()
				lf.AddFile("broken.conf", sha, mode)
				_, err := lf.CheckIntegrity(dir)
				if err == nil {
					t.Fatalf("invalid mode %q must be rejected", mode)
				}
				if !strings.Contains(err.Error(), "invalid mode") {
					t.Errorf("error must clearly reject the invalid value, got: %v", err)
				}
			})
		}
	})
}
