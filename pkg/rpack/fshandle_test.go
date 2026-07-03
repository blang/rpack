package rpack

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileBackedFSHandleStat is a regression test for the silent return-value
// order swap that existed between the FSHandle.Stat interface contract
// (exists, dir, err) and the FileBackedFSHandle implementation, which used to
// return (dir, exists, err). Go's structural interface matching only checks
// types, so the swapped values compiled and silently produced wrong
// results: ReadDir on a file path reported "path does not exist" instead of
// "path is not a directory".
func TestFileBackedFSHandleStat(t *testing.T) {
	tmp := t.TempDir()

	dirPath := filepath.Join(tmp, "mydir")
	if err := os.Mkdir(dirPath, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	filePath := filepath.Join(tmp, "myfile")
	if err := os.WriteFile(filePath, []byte("hi"), 0o600); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	missingPath := filepath.Join(tmp, "nope")

	tests := []struct {
		name      string
		path      string
		wantExist bool
		wantDir   bool
	}{
		{"file", filePath, true, false},
		{"directory", dirPath, true, true},
		{"missing", missingPath, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := NewFileBackedFSHandle(tc.path, tc.path, "map", "")
			exists, dir, err := h.Stat()
			if err != nil {
				t.Fatalf("Stat err: %v", err)
			}
			if exists != tc.wantExist {
				t.Errorf("exists = %v, want %v", exists, tc.wantExist)
			}
			if dir != tc.wantDir {
				t.Errorf("dir = %v, want %v", dir, tc.wantDir)
			}
		})
	}
}
