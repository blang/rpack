package getsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultFetcher(t *testing.T) {
	f := DefaultFetcher()
	if f == nil {
		t.Fatal("expected non-nil Fetcher")
	}
}

func TestFetcher_FetchLocalDir(t *testing.T) {
	// Create a temporary source directory with some content
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	destDir := t.TempDir()
	// Remove destDir since go-getter expects to create it
	if rErr := os.RemoveAll(destDir); rErr != nil {
		t.Fatal(rErr)
	}

	// Normalize the source to file:// URL
	srcAddr, err := NormalizeSource(srcDir)
	if err != nil {
		t.Fatal(err)
	}

	f := DefaultFetcher()
	err = f.Fetch(context.Background(), destDir, srcAddr)
	if err != nil {
		t.Fatalf("Fetch failed: %s", err)
	}

	// Verify the file was copied
	content, err := os.ReadFile(filepath.Join(destDir, "test.txt")) //nolint:gosec // test uses TempDir
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected content: %s", content)
	}
}

func TestFetcher_FetchNonexistentSource(t *testing.T) {
	destDir := t.TempDir()
	if err := os.RemoveAll(destDir); err != nil {
		t.Fatal(err)
	}

	f := DefaultFetcher()
	err := f.Fetch(context.Background(), destDir, "file:///nonexistent/path/12345")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
	t.Logf("expected error: %s", err)
}
