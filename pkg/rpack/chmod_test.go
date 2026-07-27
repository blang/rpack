package rpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagedMode stats path and returns its permission bits.
func stagedMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path) //nolint:gosec // test
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// newTestRPackFS builds an RPackFS with def/run/temp in separate temp dirs.
func newTestRPackFS(t *testing.T) (fs *RPackFS, runDir, tempDir string) {
	t.Helper()
	defDir := t.TempDir()
	runDir = t.TempDir()
	tempDir = t.TempDir()
	return NewRPackFS(true, defDir, runDir, tempDir, "", nil), runDir, tempDir
}

// TestFileBackedFSHandleWrite_NormalizesTargetMode pins the ADR 0001
// normalization rule: a staged target write always lands at exactly 0644,
// never umask-dependent, never inherited from a pre-existing staged file.
func TestFileBackedFSHandleWrite_NormalizesTargetMode(t *testing.T) {
	runDir := t.TempDir()
	h := NewFileBackedFSHandle(filepath.Join(runDir, "out.txt"), "out.txt", TargetResolver, "out.txt")

	// Pre-create the staged file with a non-default mode: os.WriteFile would
	// keep it (perm is creation-only), the normalization chmod must not.
	if err := os.WriteFile(h.absPath, []byte("old"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := h.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := stagedMode(t, h.absPath); got != 0o644 {
		t.Fatalf("mode after overwrite = %o, want 644 (reset-on-write)", got)
	}
}

// TestFileBackedFSHandleWrite_TempNotNormalized pins the temp exclusion:
// temp files keep umask-provided modes (0600 here), they are never staged
// target outputs (ADR 0001).
func TestFileBackedFSHandleWrite_TempNotNormalized(t *testing.T) {
	tempDir := t.TempDir()
	h := NewFileBackedFSHandle(filepath.Join(tempDir, "scratch.txt"), "temp:scratch.txt", TempResolver, "")
	if err := os.WriteFile(h.absPath, []byte("old"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := h.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := stagedMode(t, h.absPath); got != 0o600 {
		t.Fatalf("temp mode after overwrite = %o, want 600 (temp not normalized)", got)
	}
}

// TestBaseFSChmod covers the BaseFS.Chmod contract (ADR 0001): happy path,
// recorded-as-Write, missing file, directory refusal, resolver refusals.
//
//nolint:gocognit,gocyclo // test: table of independent subtest scenarios
func TestBaseFSChmod(t *testing.T) {
	t.Run("chmod target file succeeds and is recorded as write", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := fs.Write("./deploy.sh", []byte("#!/bin/sh\n")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./deploy.sh", 0o755); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		if got := stagedMode(t, filepath.Join(runDir, "deploy.sh")); got != 0o755 {
			t.Fatalf("staged mode = %o, want 755", got)
		}
		// The chmod must surface as a target Write record (relocation input).
		found := false
		for _, h := range fs.TargetWriteHandles() {
			if h.IndirectTargetPath() == "deploy.sh" {
				found = true
			}
		}
		if !found {
			t.Fatal("chmod not recorded as target write")
		}
	})

	t.Run("missing file errors with write-it-first", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		err := fs.Chmod("./missing.sh", 0o755)
		if err == nil || !strings.Contains(err.Error(), "does not exist (write it first)") {
			t.Fatalf("want write-it-first error, got %v", err)
		}
		// Phantom-record elimination: the failed chmod must leave no recorder
		// residue, or computeFilesToMove would later fail on a ghost path.
		if len(fs.TargetWriteHandles()) != 0 {
			t.Fatalf("failed chmod left recorder residue: %v", fs.TargetWriteHandles())
		}
	})

	t.Run("directory refused", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := os.MkdirAll(filepath.Join(runDir, "adir"), 0o750); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		err := fs.Chmod("./adir", 0o755)
		if err == nil || !strings.Contains(err.Error(), "directories is not supported") {
			t.Fatalf("want directory refusal, got %v", err)
		}
	})

	t.Run("rpack resolver refused with access error even when missing", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		// Writability runs before existence: a missing rpack: path must get
		// the access refusal, never the misleading "write it first".
		err := fs.Chmod("rpack:missing.sh", 0o755)
		if err == nil || !strings.Contains(err.Error(), "not allowed to write") {
			t.Fatalf("want access refusal, got %v", err)
		}
		if strings.Contains(err.Error(), "write it first") {
			t.Fatalf("error precedence inverted: %v", err)
		}
	})

	t.Run("map resolver refused", func(t *testing.T) {
		resolved := []*RPackResolvedInput{{Name: "data", UserPath: "data", ResolvedPath: "data", Type: RPackInputTypeFile}}
		fs := NewRPackFS(true, t.TempDir(), t.TempDir(), t.TempDir(), "", resolved)
		err := fs.Chmod("map:data", 0o755)
		if err == nil || !strings.Contains(err.Error(), "not allowed to write") {
			t.Fatalf("want access refusal, got %v", err)
		}
	})

	t.Run("temp chmod allowed", func(t *testing.T) {
		fs, _, tempDir := newTestRPackFS(t)
		if err := fs.Write("temp:scratch.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("temp:scratch.sh", 0o700); err != nil {
			t.Fatalf("temp Chmod: %v", err)
		}
		if got := stagedMode(t, filepath.Join(tempDir, "scratch.sh")); got != 0o700 {
			t.Fatalf("temp mode = %o, want 700", got)
		}
		// Temp chmods are purity-invisible and never relocated.
		if len(fs.TargetWriteHandles()) != 0 {
			t.Fatal("temp chmod leaked into target write handles")
		}
	})

	t.Run("last chmod wins", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := fs.Write("./f.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", 0o700); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", 0o755); err != nil {
			t.Fatal(err)
		}
		if got := stagedMode(t, filepath.Join(runDir, "f.sh")); got != 0o755 {
			t.Fatalf("mode = %o, want 755 (last wins)", got)
		}
	})

	t.Run("write after chmod resets to 644", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := fs.Write("./f.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.Write("./f.sh", []byte("y")); err != nil {
			t.Fatal(err)
		}
		if got := stagedMode(t, filepath.Join(runDir, "f.sh")); got != 0o644 {
			t.Fatalf("mode = %o, want 644 (reset-on-write)", got)
		}
	})
}

// TestBaseFSChmod_PurityConflict proves chmod is purity-tracked exactly like
// a write (ADR 0001): reading a map: input and chmod'ing the target file it
// resolves to conflicts in both orders.
func TestBaseFSChmod_PurityConflict(t *testing.T) {
	newFS := func(t *testing.T) *RPackFS {
		inputFile := filepath.Join(t.TempDir(), "data")
		if err := os.WriteFile(inputFile, []byte("payload"), 0o600); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		resolved := []*RPackResolvedInput{{Name: "data", UserPath: "data", ResolvedPath: inputFile, Type: RPackInputTypeFile}}
		return NewRPackFS(true, t.TempDir(), t.TempDir(), t.TempDir(), "", resolved)
	}

	t.Run("read then chmod", func(t *testing.T) {
		fs := newFS(t)
		if _, err := fs.Read("map:data"); err != nil {
			t.Fatal(err)
		}
		if err := fs.Write("./data", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./data", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.Check(); err == nil {
			t.Fatal("expected purity conflict for read map:data + chmod ./data")
		}
	})

	t.Run("chmod then read", func(t *testing.T) {
		fs := newFS(t)
		if err := fs.Write("./data", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./data", 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := fs.Read("map:data"); err != nil {
			t.Fatal(err)
		}
		if err := fs.Check(); err == nil {
			t.Fatal("expected purity conflict for chmod ./data + read map:data")
		}
	})
}

// TestInMemoryFSChmod pins the test-double contract: mode tracked per entry,
// write resets, missing/directory errors mirror BaseFS.Chmod's messages.
func TestInMemoryFSChmod(t *testing.T) {
	fs := NewInMemoryFS()
	if err := fs.Write("f.sh", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != 0o644 {
		t.Fatalf("mode after write = %o, want 644", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Chmod("f.sh", 0o755); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != 0o755 {
		t.Fatalf("mode after chmod = %o, want 755", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Write("f.sh", []byte("y")); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != 0o644 {
		t.Fatalf("mode after rewrite = %o, want 644 (reset-on-write)", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Chmod("gone.sh", 0o755); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing: want does-not-exist, got %v", err)
	}
	fs.Mkdir("adir")
	if err := fs.Chmod("adir", 0o755); err == nil || !strings.Contains(err.Error(), "directories") {
		t.Fatalf("dir: want refusal, got %v", err)
	}
}
