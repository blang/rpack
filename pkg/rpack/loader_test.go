package rpack

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestResolveRPackInputs tests the ResolveRPackInputs function.
func TestResolveRPackInputs(t *testing.T) {
	// Create a temporary directory to act as the execution path.
	execPath := t.TempDir()

	// Prepare a file and a directory in execPath.
	// Create a file "file.txt" inside execPath.
	filePath := filepath.Join(execPath, "file.txt")
	err := os.WriteFile(filePath, []byte("dummy file content"), 0644)
	if err != nil {
		t.Fatalf("failed to write file: %s", err)
	}

	// Create a directory "dir" inside execPath.
	dirPath := filepath.Join(execPath, "dir")
	err = os.Mkdir(dirPath, 0755)
	if err != nil {
		t.Fatalf("failed to create directory: %s", err)
	}

	t.Run("happy path", func(t *testing.T) {
		// Prepare a config map with relative paths.
		configInputs := map[string]string{
			"file1": "file.txt",
			"dir1":  "dir",
		}
		resolved, err := ResolveRPackInputs(configInputs, execPath)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		// Expected resolution values:
		expected := []*RPackResolvedInput{
			{
				Name:         "file1",
				UserPath:     "file.txt",
				ResolvedPath: filepath.Clean(filePath),
				Type:         RPackInputTypeFile,
			},
			{
				Name:         "dir1",
				UserPath:     "dir",
				ResolvedPath: filepath.Clean(dirPath),
				Type:         RPackInputTypeDirectory,
			},
		}

		// Because maps iterate in arbitrary order, look up each expected result by Name.
		for _, exp := range expected {
			var found bool
			for _, actual := range resolved {
				if actual.Name == exp.Name {
					found = true
					if exp.UserPath != actual.UserPath {
						t.Errorf("For %s, expected user path %q, got %q", exp.Name, exp.UserPath, actual.UserPath)
					}
					if exp.ResolvedPath != actual.ResolvedPath {
						t.Errorf("For %s, expected resolved path %q, got %q", exp.Name, exp.ResolvedPath, actual.ResolvedPath)
					}
					if exp.Type != actual.Type {
						t.Errorf("For %s, expected type %q, got %q", exp.Name, exp.Type, actual.Type)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected resolution for %s not found", exp.Name)
			}
		}

		// Additionally, verify that the number of resolved inputs matches.
		if !reflect.DeepEqual(len(expected), len(resolved)) {
			t.Errorf("expected %d results, got %d", len(expected), len(resolved))
		}
	})

	t.Run("absolute path error", func(t *testing.T) {
		// Provide an absolute path. This should return an error.
		configInputs := map[string]string{
			"abs": "/some/absolute/path",
		}
		_, err := ResolveRPackInputs(configInputs, execPath)
		if err == nil {
			t.Fatalf("expected error for absolute path but got none")
		}
	})

	t.Run("non-existent path error", func(t *testing.T) {
		// Provide a relative path that does not exist.
		configInputs := map[string]string{
			"missing": "nonexistent.txt",
		}
		_, err := ResolveRPackInputs(configInputs, execPath)
		if err == nil {
			t.Fatalf("expected error for missing file but got none")
		}
	})

	t.Run("non-local path error", func(t *testing.T) {
		// Provide a non-local path. For example, a URL can be considered non-local.
		configInputs := map[string]string{
			"nonlocal": "http://example.com/resource",
		}
		_, err := ResolveRPackInputs(configInputs, execPath)
		if err == nil {
			t.Fatalf("expected error for non-local path but got none")
		}
	})

	t.Run("directory boundary violation error", func(t *testing.T) {
		// Provide user paths that attempt to traverse outside the execPath.
		testCases := map[string]string{
			"violate1": "../outside.txt",
			"violate2": "./../../../file.txt",
		}
		for name, userPath := range testCases {
			configInputs := map[string]string{
				name: userPath,
			}
			_, err := ResolveRPackInputs(configInputs, execPath)
			if err == nil {
				t.Errorf("expected error for path %q, but got none", userPath)
			}
		}
	})
}
