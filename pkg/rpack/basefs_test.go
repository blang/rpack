package rpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBaseFSReadDirStatSwap is a consumer-side regression test for the
// FileBackedFSHandle.Stat() return-value order swap (item #3 of the top-10
// proposal). The unit test fshandle_test.go pins Stat's contract directly;
// this test pins the contract through the BaseFS.ReadDir caller that
// destructures `exists, dir, err := handle.Stat()`.
//
// Before the fix, Stat returned (dir, exists) instead of (exists, dir), so
// ReadDir on a file path destructured exists=IsDir()=false and fell into the
// "path does not exist" branch, masking the real "path is not a directory"
// error. This test locks both error distinctions in place.
func TestBaseFSReadDirStatSwap(t *testing.T) {
	tmp := t.TempDir()

	dirPath := filepath.Join(tmp, "mydir")
	if err := os.Mkdir(dirPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	filePath := filepath.Join(tmp, "myfile")
	if err := os.WriteFile(filePath, []byte("hi"), 0o600); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	// No hooks: isolate the ReadDir->Stat contract from access control so the
	// asserted errors come solely from the Stat destructure in ReadDir.
	fs := &BaseFS{
		Resolvers: []FSResolver{
			NewFileBackedFSResolver("test", "", tmp),
		},
	}

	// Directory path succeeds with empty (or nil) entry slices.
	if files, dirs, err := fs.ReadDir("mydir"); err != nil {
		t.Fatalf("ReadDir dir: unexpected err: %v", err)
	} else if len(files) != 0 || len(dirs) != 0 {
		t.Errorf("ReadDir dir: files=%v dirs=%v, want empty", files, dirs)
	}

	// File path must report "not a directory" — NOT "does not exist".
	_, _, err := fs.ReadDir("myfile")
	if err == nil {
		t.Fatal("ReadDir file: expected error, got nil")
	}
	if strings.Contains(err.Error(), "does not exist") {
		t.Errorf("ReadDir file: got misleading %q (Stat values still swapped), want 'path is not a directory'", err.Error())
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("ReadDir file: got %q, want an error mentioning 'is not a directory'", err.Error())
	}

	// Missing path must report "does not exist".
	_, _, err = fs.ReadDir("nope")
	if err == nil {
		t.Fatal("ReadDir missing: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("ReadDir missing: got %q, want an error mentioning 'does not exist'", err.Error())
	}
}
