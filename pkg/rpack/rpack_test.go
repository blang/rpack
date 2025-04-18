package rpack

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/samber/lo"
)

// calculateSHA256 reads the file at filePath, calculates its sha256 checksum,
// and returns it as a hex string.
func calculateSHA256(t *testing.T, filePath string) string {
	t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %q: %v", filePath, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

func TestRPackLockFileCheckIntegrity(t *testing.T) {
	// Create a temporary directory to simulate the file structure.
	tempDir := t.TempDir()

	t.Run("all files valid", func(t *testing.T) {
		// Create a valid file.
		fileName := "valid.txt"
		filePath := filepath.Join(tempDir, fileName)
		originalContent := []byte("original content")
		if err := os.WriteFile(filePath, originalContent, 0644); err != nil {
			t.Fatalf("Failed to create file %q: %v", filePath, err)
		}
		sha := calculateSHA256(t, filePath)

		// Create a lockfile entry with the correct checksum.
		lockFile := NewRPackLockFile()
		lockFile.AddFile(fileName, sha)

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
		lockFile.AddFile(fileName, dummySHA)

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
		if err := os.WriteFile(filePath, initialContent, 0644); err != nil {
			t.Fatalf("Failed to create file %q: %v", filePath, err)
		}
		// Compute its initial checksum.
		sha := calculateSHA256(t, filePath)

		// Now modify the file.
		modifiedContent := []byte("modified content")
		if err := os.WriteFile(filePath, modifiedContent, 0644); err != nil {
			t.Fatalf("Failed to modify file %q: %v", filePath, err)
		}

		lockFile := NewRPackLockFile()
		lockFile.AddFile(fileName, sha)

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
		if err := os.WriteFile(validPath, []byte("content valid"), 0644); err != nil {
			t.Fatalf("Failed to create file %q: %v", validPath, err)
		}
		validSHA := calculateSHA256(t, validPath)

		// missing file (do not create)
		missingFile := "missing2.txt"

		// modified file: create then change it.
		modFile := "mod2.txt"
		modPath := filepath.Join(tempDir, modFile)
		if err := os.WriteFile(modPath, []byte("original mod"), 0644); err != nil {
			t.Fatalf("Failed to create file %q: %v", modPath, err)
		}
		modSHA := calculateSHA256(t, modPath)
		// Modify the file to simulate external change.
		if err := os.WriteFile(modPath, []byte("changed mod"), 0644); err != nil {
			t.Fatalf("Failed to modify file %q: %v", modPath, err)
		}

		// Build a lockfile with all three entries.
		lockFile := NewRPackLockFile()
		lockFile.AddFile(validFile, validSHA)
		lockFile.AddFile(missingFile, "dummy")
		lockFile.AddFile(modFile, modSHA)

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
		oldLF.AddFile("a.txt", "sha1")
		oldLF.AddFile("b.txt", "sha2")

		newLF := NewRPackLockFile()
		newLF.AddFile("a.txt", "sha1")
		newLF.AddFile("b.txt", "sha2")

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
		oldLF.AddFile("common.txt", "sha-common")

		newLF := NewRPackLockFile()
		newLF.AddFile("common.txt", "sha-common")
		newLF.AddFile("new.txt", "sha-new")

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
		oldLF.AddFile("a.txt", "sha-a")
		oldLF.AddFile("b.txt", "sha-b")

		newLF := NewRPackLockFile()
		newLF.AddFile("a.txt", "sha-a")

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
		oldLF.AddFile("a.txt", "sha-a")
		oldLF.AddFile("b.txt", "sha-b")

		newLF := NewRPackLockFile()
		newLF.AddFile("b.txt", "sha-b")
		newLF.AddFile("c.txt", "sha-c")

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
