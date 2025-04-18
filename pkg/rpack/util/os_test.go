package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCopyFile tests the CopyFile function.
func TestCopyFile(t *testing.T) {
	// Create a temporary directory for the test files.
	dir := t.TempDir()

	// Create a source file with some content.
	srcPath := filepath.Join(dir, "testfile")
	content := []byte("Testing filecontent")
	if err := os.WriteFile(srcPath, content, 0666); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// Define the destination path.
	dstPath := filepath.Join(dir, "newfile")

	// Call CopyFile to copy the content from srcPath to dstPath.
	if err := CopyFile(dstPath, srcPath); err != nil {
		t.Fatalf("CopyFile returned error: %v", err)
	}

	// Verify the content of the destination file.
	gotContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}
	if string(gotContent) != string(content) {
		t.Errorf("file content mismatch: expected %q, got %q", string(content), string(gotContent))
	}

	// Verify that file mode (permissions) are the same.
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("failed to stat destination file: %v", err)
	}
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("file mode mismatch: expected %v, got %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestCheckFileExists(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		nonExistentPath := "nonexistentfile.txt"
		err := CheckFileExists(nonExistentPath)
		if err == nil {
			t.Errorf("Expected error for non-existent file %s, got nil", nonExistentPath)
		}
		// Check for the expected error substring.
		if !strings.Contains(err.Error(), "File does not exist") {
			t.Errorf("Expected error to contain 'File does not exist', got: %v", err)
		}
	})

	t.Run("directory path", func(t *testing.T) {
		// t.TempDir() creates a temporary directory and returns its path.
		tempDir := t.TempDir()
		err := CheckFileExists(tempDir)
		if err == nil {
			t.Errorf("Expected error for directory path %s, got nil", tempDir)
		}
		if !strings.Contains(err.Error(), "Path is a directory") {
			t.Errorf("Expected error to contain 'Path is a directory', got: %v", err)
		}
	})

	t.Run("valid file", func(t *testing.T) {
		// Create a temporary directory to hold the file.
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "tempfile.txt")

		// Create the file.
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create temporary file: %v", err)
		}
		_ = f.Close()

		// The file should exist and be a regular file.
		err = CheckFileExists(filePath)
		if err != nil {
			t.Errorf("Did not expect error for valid file %s, got: %v", filePath, err)
		}
	})
}
