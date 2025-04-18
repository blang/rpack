package util

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestSha256File tests the Sha256File function for various scenarios.
func TestSha256File(t *testing.T) {

	t.Run("KnownContent", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a file with known content.
		content := []byte("hello world")
		tempFilePath := filepath.Join(tmpDir, "testfile.txt")
		if err := os.WriteFile(tempFilePath, content, 0644); err != nil {
			t.Fatalf("Failed to write temporary file: %v", err)
		}

		// Calculate expected checksum.
		expectedSum := sha256.Sum256(content)
		expected := hex.EncodeToString(expectedSum[:])

		// Compute checksum using our function.
		checksum, err := Sha256File(tempFilePath)
		if err != nil {
			t.Fatalf("Sha256File returned error: %v", err)
		}
		if checksum != expected {
			t.Errorf("Checksum mismatch. Expected: %s, got: %s", expected, checksum)
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		nonExistentFile := filepath.Join(tmpDir, "does_not_exist.txt")
		_, err := Sha256File(nonExistentFile)
		if err == nil {
			t.Errorf("Expected an error for a non-existent file, but got nil")
		}
	})

	t.Run("EmptyFile", func(t *testing.T) {
		tmpDir := t.TempDir()

		emptyFilePath := filepath.Join(tmpDir, "empty.txt")
		if err := os.WriteFile(emptyFilePath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create empty temporary file: %v", err)
		}
		expectedEmpty := sha256.Sum256([]byte(""))
		expectedEmptyHex := hex.EncodeToString(expectedEmpty[:])
		checksumEmpty, err := Sha256File(emptyFilePath)
		if err != nil {
			t.Fatalf("Sha256File returned error for empty file: %v", err)
		}
		if checksumEmpty != expectedEmptyHex {
			t.Errorf("Empty file checksum mismatch. Expected: %s, got: %s", expectedEmptyHex, checksumEmpty)
		}
	})
}
