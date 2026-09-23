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

// resolvedIntent resolves a friendly path on fs and returns the handle's
// declared canonical intent ("644"/"755").
func resolvedIntent(t *testing.T, fs *RPackFS, name string) string {
	t.Helper()
	h, err := fs.resolve(name)
	if err != nil {
		t.Fatalf("resolve %s: %v", name, err)
	}
	return CanonicalMode(h.OutputMode())
}

// preCreateStaged writes a staged file with an exact mode (chmod after write
// so the process umask cannot narrow it) to prove mode isolation.
func preCreateStaged(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
}

// TestFileBackedFSHandleWrite_StagesPrivateInode pins the ADR 0002 staging
// rule: a staged target write always recreates a private 0600 inode, never
// inheriting a pre-existing staged file's mode and never exposing the final
// intent in staging.
func TestFileBackedFSHandleWrite_StagesPrivateInode(t *testing.T) {
	runDir := t.TempDir()
	h := NewFileBackedFSHandle(filepath.Join(runDir, "out.txt"), "out.txt", TargetResolver, "out.txt")

	// Pre-create the staged file with a wide mode: os.WriteFile would keep it
	// (perm is creation-only); the recreate-on-write must not.
	preCreateStaged(t, h.absPath, 0o777)
	if err := h.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := stagedMode(t, h.absPath); got != 0o600 {
		t.Fatalf("staged mode after write = %o, want 600 (private staging)", got)
	}
	if got := CanonicalMode(h.OutputMode()); got != "644" {
		t.Fatalf("intent after write = %q, want 644 (reset-on-write)", got)
	}
}

// TestFileBackedFSHandleWrite_TempAlsoPrivate pins that temp staging follows
// the same private-inode policy: temp files are never staged outputs, and a
// pre-existing temp file's mode never leaks.
func TestFileBackedFSHandleWrite_TempAlsoPrivate(t *testing.T) {
	tempDir := t.TempDir()
	h := NewFileBackedFSHandle(filepath.Join(tempDir, "scratch.txt"), "temp:scratch.txt", TempResolver, "")
	preCreateStaged(t, h.absPath, 0o777)
	if err := h.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := stagedMode(t, h.absPath); got != 0o600 {
		t.Fatalf("temp staged mode after write = %o, want 600 (private staging)", got)
	}
}

// TestBaseFSChmod covers the BaseFS.Chmod contract (ADR 0002): metadata-only
// intent declaration, recorded-as-Write, legacy-mode reduction, and
// resolver refusals. Physical staging permissions never change.
//
//nolint:gocognit,gocyclo // test: table of independent subtest scenarios
func TestBaseFSChmod(t *testing.T) {
	t.Run("chmod records intent without physical chmod", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := fs.Write("./deploy.sh", []byte("#!/bin/sh\n")); err != nil {
			t.Fatal(err)
		}
		if got := stagedMode(t, filepath.Join(runDir, "deploy.sh")); got != 0o600 {
			t.Fatalf("staged mode after write = %o, want 600", got)
		}
		if err := fs.Chmod("./deploy.sh", ExecutableMode); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		// Chmod is metadata-only: staging stays private at 0600.
		if got := stagedMode(t, filepath.Join(runDir, "deploy.sh")); got != 0o600 {
			t.Fatalf("staged mode after chmod = %o, want 600 (no physical chmod)", got)
		}
		if got := resolvedIntent(t, fs, "./deploy.sh"); got != "755" {
			t.Fatalf("declared intent after chmod = %q, want 755", got)
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
		err := fs.Chmod("./missing.sh", ExecutableMode)
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
		err := fs.Chmod("./adir", ExecutableMode)
		if err == nil || !strings.Contains(err.Error(), "directories is not supported") {
			t.Fatalf("want directory refusal, got %v", err)
		}
	})

	t.Run("rpack resolver refused with access error even when missing", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		// Writability runs before existence: a missing rpack: path must get
		// the access refusal, never the misleading "write it first".
		err := fs.Chmod("rpack:missing.sh", ExecutableMode)
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
		err := fs.Chmod("map:data", ExecutableMode)
		if err == nil || !strings.Contains(err.Error(), "not allowed to write") {
			t.Fatalf("want access refusal, got %v", err)
		}
	})

	t.Run("legacy Go modes reduce to executable intent", func(t *testing.T) {
		fs, runDir, _ := newTestRPackFS(t)
		if err := fs.Write("./f.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		for mode, want := range map[os.FileMode]string{0o600: "644", 0o700: "755", 0o777: "755"} {
			if err := fs.Chmod("./f.sh", mode); err != nil {
				t.Fatalf("mode %o: %v", mode, err)
			}
			if got := resolvedIntent(t, fs, "./f.sh"); got != want {
				t.Fatalf("mode %o: intent = %q, want %q", mode, got, want)
			}
		}
		// Intent changes remain metadata-only: staging stays private.
		if got := stagedMode(t, filepath.Join(runDir, "f.sh")); got != 0o600 {
			t.Fatalf("staged mode after declarations = %o, want 600", got)
		}
		writes := 0
		for _, h := range fs.TargetWriteHandles() {
			if h.IndirectTargetPath() == "f.sh" {
				writes++
			}
		}
		if writes != 4 { // one write plus three successful declarations
			t.Fatalf("target write records for f.sh = %d, want 4", writes)
		}
	})

	t.Run("temp chmod allowed, metadata only", func(t *testing.T) {
		fs, _, tempDir := newTestRPackFS(t)
		if err := fs.Write("temp:scratch.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("temp:scratch.sh", ExecutableMode); err != nil {
			t.Fatalf("temp Chmod: %v", err)
		}
		if got := stagedMode(t, filepath.Join(tempDir, "scratch.sh")); got != 0o600 {
			t.Fatalf("temp staged mode after chmod = %o, want 600 (no physical chmod)", got)
		}
		if got := resolvedIntent(t, fs, "temp:scratch.sh"); got != "755" {
			t.Fatalf("temp intent after chmod = %q, want 755", got)
		}
		// Temp chmods are purity-invisible and never relocated.
		if len(fs.TargetWriteHandles()) != 0 {
			t.Fatal("temp chmod leaked into target write handles")
		}
	})

	t.Run("friendly spellings share one intent", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		if err := fs.Write("deploy.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./deploy.sh", ExecutableMode); err != nil {
			t.Fatal(err)
		}
		// "deploy.sh" and "./deploy.sh" resolve to the same stable handle, so
		// intent declared through one spelling is visible through the other.
		if got := resolvedIntent(t, fs, "deploy.sh"); got != "755" {
			t.Fatalf("intent via bare spelling = %q, want 755 (shared handle)", got)
		}
		// A rewrite through either spelling resets the shared intent.
		if err := fs.Write("./deploy.sh", []byte("y")); err != nil {
			t.Fatal(err)
		}
		if got := resolvedIntent(t, fs, "deploy.sh"); got != "644" {
			t.Fatalf("intent after rewrite via ./-spelling = %q, want 644 (shared reset)", got)
		}
	})

	t.Run("last declared intent wins", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		if err := fs.Write("./f.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", NonExecutableMode); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", ExecutableMode); err != nil {
			t.Fatal(err)
		}
		if got := resolvedIntent(t, fs, "./f.sh"); got != "755" {
			t.Fatalf("intent = %q, want 755 (last wins)", got)
		}
	})

	t.Run("write after chmod resets to non-executable", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		if err := fs.Write("./f.sh", []byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("./f.sh", ExecutableMode); err != nil {
			t.Fatal(err)
		}
		if err := fs.Write("./f.sh", []byte("y")); err != nil {
			t.Fatal(err)
		}
		if got := resolvedIntent(t, fs, "./f.sh"); got != "644" {
			t.Fatalf("intent after rewrite = %q, want 644 (reset-on-write)", got)
		}
	})

	t.Run("temp intent never propagates through a copy", func(t *testing.T) {
		fs, _, _ := newTestRPackFS(t)
		if err := fs.Write("temp:src.sh", []byte("#!/bin/sh\n")); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod("temp:src.sh", ExecutableMode); err != nil {
			t.Fatal(err)
		}
		// A copy is read+write: the target write establishes its own default
		// intent; the temp file's declaration is never inherited.
		b, err := fs.Read("temp:src.sh")
		if err != nil {
			t.Fatal(err)
		}
		if err := fs.Write("./copied.sh", b); err != nil {
			t.Fatal(err)
		}
		if got := resolvedIntent(t, fs, "./copied.sh"); got != "644" {
			t.Fatalf("copied intent = %q, want 644 (temp intent never propagates)", got)
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
		if err := fs.Chmod("./data", ExecutableMode); err != nil {
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
		if err := fs.Chmod("./data", ExecutableMode); err != nil {
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

// TestInMemoryFSChmod pins the test-double contract: canonical intent tracked
// per entry, legacy modes reduced, write resets, and missing/directory errors
// mirror BaseFS.Chmod's behavior.
//
//nolint:gocyclo // test: linear scenario sequence
func TestInMemoryFSChmod(t *testing.T) {
	fs := NewInMemoryFS()
	if err := fs.Write("f.sh", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != NonExecutableMode {
		t.Fatalf("mode after write = %o, want 644", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Chmod("f.sh", ExecutableMode); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != ExecutableMode {
		t.Fatalf("mode after chmod = %o, want 755", fs.Tree["f.sh"].Mode)
	}
	// Legacy exact modes are reduced like on the real FS.
	if err := fs.Chmod("f.sh", 0o600); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != NonExecutableMode {
		t.Fatalf("legacy 600 mode = %o, want canonical 644 intent", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Chmod("f.sh", 0o700); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != ExecutableMode {
		t.Fatalf("legacy 700 mode = %o, want canonical 755 intent", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Write("f.sh", []byte("y")); err != nil {
		t.Fatal(err)
	}
	if fs.Tree["f.sh"].Mode != NonExecutableMode {
		t.Fatalf("mode after rewrite = %o, want 644 (reset-on-write)", fs.Tree["f.sh"].Mode)
	}
	if err := fs.Chmod("gone.sh", ExecutableMode); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing: want does-not-exist, got %v", err)
	}
	fs.Mkdir("adir")
	if err := fs.Chmod("adir", ExecutableMode); err == nil || !strings.Contains(err.Error(), "directories") {
		t.Fatalf("dir: want refusal, got %v", err)
	}
}
