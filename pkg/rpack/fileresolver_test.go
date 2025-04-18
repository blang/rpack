package rpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTempDirOrFail creates a temporary directory and returns its absolute path.
func createTempDirOrFail(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}
	return abs
}

// createTempFile creates a temporary file in a given directory.
func createTempFile(t *testing.T, dir string) string {
	t.Helper()
	filepath := filepath.Join(dir, "notadir")
	f, err := os.Create(filepath)
	if err != nil {
		t.Fatalf("failed to create a file: %v", err)
	}
	_ = f.Close()
	return filepath
}

// TestNewFileResolver tests the constructor for FileResolver.
func TestNewFileResolver(t *testing.T) {
	// Success case: all directories provided are valid.
	defDir := createTempDirOrFail(t)
	runDir := createTempDirOrFail(t)
	tempDir := createTempDirOrFail(t)
	execDir := createTempDirOrFail(t)

	// Dummy resolved inputs (can be empty for this test).
	resolvedInputs := []*RPackResolvedInput{}

	// Should succeed.
	_, err := NewFileResolver(defDir, runDir, tempDir, execDir, resolvedInputs)
	if err != nil {
		t.Fatalf("expected no error with valid directories, got: %v", err)
	}

	// Failure case: pass a file instead of a directory.
	notADir := createTempFile(t, defDir)
	_, err = NewFileResolver(notADir, runDir, tempDir, execDir, resolvedInputs)
	if err == nil || !strings.Contains(err.Error(), "Failed to use defSourcePath") {
		t.Errorf("expected error when defSourcePath is not a directory, got: %v", err)
	}
}

// TestResolveInput tests the ResolveInput method for all supported prefixes.
func TestResolveInput(t *testing.T) {
	// Create temporary directories for all base paths.
	defDir := createTempDirOrFail(t)
	runDir := createTempDirOrFail(t)
	tempDir := createTempDirOrFail(t)
	execDir := createTempDirOrFail(t)

	// Prepare some dummy resolvedInputs for mapping:
	// One input for a file and one for a directory.
	resolvedInputs := []*RPackResolvedInput{
		{
			Name:         "inputFile",
			UserPath:     "file.txt",
			ResolvedPath: filepath.Join("/dummy/path", "file.txt"),
			Type:         RPackInputTypeFile,
		},
		{
			Name:         "inputDir",
			UserPath:     "dir",
			ResolvedPath: filepath.Join("/dummy/path", "dir"),
			Type:         RPackInputTypeDirectory,
		},
	}

	fr, err := NewFileResolver(defDir, runDir, tempDir, execDir, resolvedInputs)
	if err != nil {
		t.Fatalf("failed to create FileResolver: %v", err)
	}

	t.Run("map: without extra subpath", func(t *testing.T) {
		// For mapping, using prefix "map:"; if no slash is given then returns the base mapped path.
		// For inputFile (which is a file) there is no extra subpath. This should work if the mapped input is file.
		// However note: resolveMapInput only allows extra subpath if the mapping is a directory.
		got, err := fr.ResolveInput("map:inputFile")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.AbsPath != resolvedInputs[0].ResolvedPath {
			t.Errorf("expected abspath %q, got %q", resolvedInputs[0].ResolvedPath, got.AbsPath)
		}
		if got.Path != "file.txt" {
			t.Errorf("expected path %q, got %q", "file.txt", got.Path)
		}
		if got.Location != FileResolverLocationMapped {
			t.Errorf("expected location %q, got %q", FileResolverLocationMapped, got.Location)
		}
	})
	t.Run("map: with extra subpath on directory", func(t *testing.T) {
		// For inputDir, add a relative subpath.
		got, err := fr.ResolveInput("map:inputDir/subdir/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(resolvedInputs[1].ResolvedPath, filepath.Clean("subdir/file.txt"))
		expectedRel := "dir/subdir/file.txt"
		if got.AbsPath != expected {
			t.Errorf("expected abspath %q, got %q", expected, got.AbsPath)
		}
		if got.Path != expectedRel {
			t.Errorf("expected path %q, got %q", expectedRel, got.Path)
		}
		if got.Location != FileResolverLocationMapped {
			t.Errorf("expected location %q, got %q", FileResolverLocationMapped, got.Location)
		}
	})
	t.Run("map: extra subpath on non-directory", func(t *testing.T) {
		// For inputFile (a non-directory) adding an extra subpath should error.
		_, err := fr.ResolveInput("map:inputFile/subdir")
		if err == nil || !strings.Contains(err.Error(), "is not a directory") {
			t.Errorf("expected error for map: inputFile with subpath, got: %v", err)
		}
	})
	t.Run("map: invalid mapping", func(t *testing.T) {
		_, err := fr.ResolveInput("map:nonexistent")
		if err == nil || !strings.Contains(err.Error(), "Could not find mapped input") {
			t.Errorf("expected error for unknown mapping, got: %v", err)
		}
	})

	t.Run("rpack: valid relative path", func(t *testing.T) {
		got, err := fr.ResolveInput("rpack:subdir/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(filepath.Clean(defDir), filepath.Clean("subdir/file.txt"))
		expectedRel := "subdir/file.txt"
		if got.AbsPath != expected {
			t.Errorf("expected abspath %q, got %q", expected, got.AbsPath)
		}
		if got.Path != expectedRel {
			t.Errorf("expected relpath %q, got %q", expectedRel, got.Path)
		}
		if got.Location != FileResolverLocationRPack {
			t.Errorf("expected location %q, got %q", FileResolverLocationRPack, got.Location)
		}
	})
	t.Run("rpack: absolute path error", func(t *testing.T) {
		_, err := fr.ResolveInput("rpack:" + filepath.Join(string(os.PathSeparator), "abs", "file.txt"))
		if err == nil || !strings.Contains(err.Error(), "needs to be relative") {
			t.Errorf("expected error for absolute rpack path, got: %v", err)
		}
	})
	t.Run("temp: valid relative path", func(t *testing.T) {
		got, err := fr.ResolveInput("temp:tempfile.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(filepath.Clean(tempDir), filepath.Clean("tempfile.txt"))
		expectedRel := "tempfile.txt"
		if got.AbsPath != expected {
			t.Errorf("expected %q, got %q", expected, got.AbsPath)
		}
		if got.Path != expectedRel {
			t.Errorf("expected relpath %q, got %q", expectedRel, got.Path)
		}
		if got.Location != FileResolverLocationTemp {
			t.Errorf("expected location %q, got %q", FileResolverLocationTemp, got.Location)
		}
	})
	t.Run("temp: absolute path error", func(t *testing.T) {
		absPath := filepath.Join(string(os.PathSeparator), "temp", "file.txt")
		_, err := fr.ResolveInput("temp:" + absPath)
		if err == nil || !strings.Contains(err.Error(), "needs to be relative") {
			t.Errorf("expected error for absolute temp path, got: %v", err)
		}
	})
	t.Run("invalid prefix", func(t *testing.T) {
		_, err := fr.ResolveInput("unknown:foo")
		if err == nil || !strings.Contains(err.Error(), "not valid") {
			t.Errorf("expected error for invalid prefix, got: %v", err)
		}
	})
}

// TestResolveOutput tests the ResolveOutput method.
func TestResolveOutput(t *testing.T) {
	// Create temporary directories for run and temp – the only two output spaces.
	defDir := createTempDirOrFail(t)
	runDir := createTempDirOrFail(t)
	tempDir := createTempDirOrFail(t)
	execDir := createTempDirOrFail(t)

	// No resolvedInputs needed for output.
	fr, err := NewFileResolver(defDir, runDir, tempDir, execDir, nil)
	if err != nil {
		t.Fatalf("failed to create FileResolver: %v", err)
	}

	t.Run("output with no prefix (maps to runPath)", func(t *testing.T) {
		// Input is a relative local path (without colon) so that it maps to run directory.
		got, err := fr.ResolveOutput("subdir/file.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(filepath.Clean(runDir), filepath.Clean("subdir/file.out"))
		expectedRel := "subdir/file.out"
		if got.AbsPath != expected {
			t.Errorf("expected abspath %q, got %q", expected, got.AbsPath)
		}
		if got.Path != expectedRel {
			t.Errorf("expected relpath %q, got %q", expectedRel, got.Path)
		}
		if got.Location != FileResolverLocationSource {
			t.Errorf("expected location %q, got %q", FileResolverLocationSource, got.Location)
		}
	})
	t.Run("output with temp: prefix", func(t *testing.T) {
		got, err := fr.ResolveOutput("temp:tempfile.out")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Join(filepath.Clean(tempDir), filepath.Clean("tempfile.out"))
		expectedRel := "tempfile.out"
		if got.AbsPath != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
		if got.Path != expectedRel {
			t.Errorf("expected relpath %q, got %q", expectedRel, got.Path)
		}
		if got.Location != FileResolverLocationTemp {
			t.Errorf("expected location %q, got %q", FileResolverLocationTemp, got.Location)
		}
	})
	t.Run("output with unknown prefix error", func(t *testing.T) {
		_, err := fr.ResolveOutput("map:somepath")
		if err == nil || !strings.Contains(err.Error(), "Output path needs to use temp: prefix") {
			t.Errorf("expected error for unknown output prefix, got: %v", err)
		}
	})
	t.Run("output with absolute path error", func(t *testing.T) {
		// Since ResolveOutput without colon should be relative, supply an absolute input.
		absPath := filepath.Join(string(os.PathSeparator), "abs", "file.out")
		_, err := fr.ResolveOutput(absPath)
		if err == nil || !strings.Contains(err.Error(), "needs to be relative") {
			t.Errorf("expected error for absolute output path, got: %v", err)
		}
	})
}
