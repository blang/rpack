package rpack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// gitRepoLocatorEnvVars mirrors the variables scrubbed by the getsource git
// getter; duplicated here so this test also proves the scrub covers the
// variables a poisoned environment would realistically set.
var gitRepoLocatorEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_QUARANTINE_PATH",
}

// runGit runs a git command with a sanitized environment (no repository
// locator variables leaking in) and a fixed author/committer identity.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cleanEnv := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.Contains(gitRepoLocatorEnvVars, name) {
			cleanEnv = append(cleanEnv, entry)
		}
	}
	cleanEnv = append(cleanEnv,
		// Ignore user/system gitconfig so settings like commit.gpgsign
		// cannot break the test fixtures.
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=rpack-test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=rpack-test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	cmd := exec.Command("git", args...) //nolint:gosec // test helper with fixed args
	cmd.Dir = dir
	cmd.Env = cleanEnv
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %s\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// initGitRepo creates a git repository in dir with one commit containing the
// given files (path -> content).
func initGitRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	runGit(t, dir, "init", "-b", "main")
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")
}

func TestExtractPackageAddrSubDir_LocalPath(t *testing.T) {
	pkgDir, subDir, err := extractPackageAddrSubDir("./some/local/module")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if pkgDir == "" {
		t.Fatal("expected non-empty package dir")
	}
	t.Logf("pkgDir=%s subDir=%s", pkgDir, subDir)
}

func TestExtractPackageAddrSubDir_GitHub(t *testing.T) {
	pkgDir, subDir, err := extractPackageAddrSubDir("github.com/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if pkgDir == "" {
		t.Fatal("expected non-empty package dir")
	}
	t.Logf("pkgDir=%s subDir=%s", pkgDir, subDir)
}

func TestExtractPackageAddrSubDir_GitHubWithSubdir(t *testing.T) {
	pkgDir, subDir, err := extractPackageAddrSubDir("github.com/user/repo//subdir")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if subDir != "subdir" {
		t.Fatalf("expected subdir 'subdir', got %q", subDir)
	}
	t.Logf("pkgDir=%s subDir=%s", pkgDir, subDir)
}

func TestExtractPackageAddrSubDir_RelativePathError(t *testing.T) {
	_, _, err := extractPackageAddrSubDir("some/naked/relative/path")
	if err == nil {
		t.Fatal("expected error for bare relative path")
	}
	t.Logf("expected error: %s", err)
}

// TestResolveRPackInputs tests the ResolveRPackInputs function.
//
//nolint:gocognit,gocyclo // test: table-driven test with many cases
func TestResolveRPackInputs(t *testing.T) {
	// Create a temporary directory to act as the execution path.
	execPath := t.TempDir()

	// Prepare a file and a directory in execPath.
	// Create a file "file.txt" inside execPath.
	filePath := filepath.Join(execPath, "file.txt")
	err := os.WriteFile(filePath, []byte("dummy file content"), 0o644) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("failed to write file: %s", err)
	}

	// Create a directory "dir" inside execPath.
	dirPath := filepath.Join(execPath, "dir")
	err = os.Mkdir(dirPath, 0o755) //nolint:gosec // test file
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
				if actual.Name != exp.Name {
					continue
				}
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

// TestLoadRPack_RefetchSources guards two regressions in the source cache
// re-fetch path of LoadRPack:
//
//  1. Poisoned environment: with GIT_DIR/GIT_WORK_TREE set (e.g. rpack
//     invoked from a git hook), go-getter's embedded git commands operated
//     on that foreign repository instead of the source checkout, failing
//     with "error: remote origin already exists" (or worse, mutating the
//     foreign repository).
//  2. Ref-less re-fetch: with the source cache dir present from a previous
//     run, go-getter's git getter took its "update" code path, which fails
//     for sources without a pinned ref with "invalid ref: \"\"". LoadRPack
//     now cleans the source cache dir before every fetch so the clone path
//     is always taken.
//
//nolint:gocognit // test: sequential scenario with subtests
func TestLoadRPack_RefetchSources(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Source git repository containing a rpack definition and a subdirectory
	// variant of it.
	srcDir := t.TempDir()
	initGitRepo(t, srcDir, map[string]string{
		"rpack.yaml":     "\"@schema_version\": \"v1\"\nname: gitsrc\n",
		"sub/rpack.yaml": "\"@schema_version\": \"v1\"\nname: gitsub\n",
		"sub/data.txt":   "sub-data\n",
	})

	// The foreign repository the poisoned environment points at. It has an
	// "origin" remote, so a hijacked "git remote add origin" would fail
	// with "error: remote origin already exists".
	foreignDir := t.TempDir()
	initGitRepo(t, foreignDir, map[string]string{"keep.txt": "untouched\n"})
	runGit(t, foreignDir, "remote", "add", "origin", "https://example.invalid/foreign.git")
	foreignHead := runGit(t, foreignDir, "rev-parse", "HEAD")

	// Simulate rpack being invoked from a git hook or a bare-repo/dotfiles
	// setup exporting the repository locator variables.
	t.Setenv("GIT_DIR", filepath.Join(foreignDir, ".git"))
	t.Setenv("GIT_WORK_TREE", foreignDir)

	newCI := func(execPath, source string) *RPackConfigInstance {
		return &RPackConfigInstance{
			ConfigPath:   filepath.Join(execPath, "app.rpack.yaml"),
			Config:       &RPackConfig{SchemaVersion: RPackConfigCurrentSchemaVersion, Source: source, Config: &RPackConfigConfig{}},
			LockFile:     NewRPackLockFile(),
			LockFilePath: filepath.Join(execPath, "app.rpack.lock.yaml"),
		}
	}

	assertFileContent := func(t *testing.T, path, want string) {
		t.Helper()
		content, err := os.ReadFile(path) //nolint:gosec // test uses TempDir
		if err != nil {
			t.Fatalf("expected fetched file %s: %s", path, err)
		}
		if string(content) != want {
			t.Fatalf("unexpected content of %s: %q, want %q", path, content, want)
		}
	}

	t.Run("git source refetch", func(t *testing.T) {
		execPath := t.TempDir()
		ci := newCI(execPath, "git::file://"+srcDir)
		for i := range 2 {
			inst, err := LoadRPack(context.Background(), ci, execPath)
			if err != nil {
				t.Fatalf("LoadRPack run %d failed: %s", i+1, err)
			}
			assertFileContent(t, filepath.Join(inst.SourcePath, "rpack.yaml"),
				"\"@schema_version\": \"v1\"\nname: gitsrc\n")
		}
	})

	t.Run("git source with subdir refetch", func(t *testing.T) {
		execPath := t.TempDir()
		ci := newCI(execPath, "git::file://"+srcDir+"//sub")
		for i := range 2 {
			inst, err := LoadRPack(context.Background(), ci, execPath)
			if err != nil {
				t.Fatalf("LoadRPack run %d failed: %s", i+1, err)
			}
			assertFileContent(t, filepath.Join(inst.SourcePath, "data.txt"), "sub-data\n")
		}
	})

	t.Run("local source refetch keeps link target", func(t *testing.T) {
		execPath := t.TempDir()
		defDir := t.TempDir()
		defFile := filepath.Join(defDir, "rpack.yaml")
		defContent := "\"@schema_version\": \"v1\"\nname: localdef\n"
		if err := os.WriteFile(defFile, []byte(defContent), 0o600); err != nil {
			t.Fatal(err)
		}
		ci := newCI(execPath, defDir)
		for i := range 2 {
			inst, err := LoadRPack(context.Background(), ci, execPath)
			if err != nil {
				t.Fatalf("LoadRPack run %d failed: %s", i+1, err)
			}
			assertFileContent(t, filepath.Join(inst.SourcePath, "rpack.yaml"), defContent)
		}
		// The cache cleanup must remove only the symlink, never the target.
		assertFileContent(t, defFile, defContent)
	})

	// The poisoned environment must have been restored.
	if got := os.Getenv("GIT_DIR"); got != filepath.Join(foreignDir, ".git") {
		t.Fatalf("GIT_DIR was not restored after LoadRPack, got %q", got)
	}

	// The foreign repository must be completely untouched.
	if got := runGit(t, foreignDir, "rev-parse", "HEAD"); got != foreignHead {
		t.Fatalf("foreign repository HEAD changed: %s -> %s", foreignHead, got)
	}
	if _, err := os.Stat(filepath.Join(foreignDir, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatal("foreign repository has a FETCH_HEAD: a fetch was redirected into it")
	}
	if got := runGit(t, foreignDir, "status", "--porcelain"); got != "" {
		t.Fatalf("foreign repository work tree modified:\n%s", got)
	}
}

// TestLoadRPackFile_ConfigLessDoesNotPanic guards the nil-deref fixed in
// loadRPackFile: a schema-valid rpack.yaml that omits the optional `config:`
// block must leave Config as a non-nil empty struct, not panic on later
// dereference of ci.Config.Config.Inputs in LoadRPack.
func TestLoadRPackFile_ConfigLessDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.rpack.yaml")
	if err := os.WriteFile(path, []byte("\"@schema_version\": \"v1\"\nsource: \"./rpackdef\"\n"), 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write: %v", err)
	}
	c, err := loadRPackFile(path)
	if err != nil {
		t.Fatalf("loadRPackFile: %v", err)
	}
	if c.Config == nil {
		t.Fatal("Config must be non-nil even when `config:` is omitted")
	}
	// Dereference Inputs exactly as LoadRPack does — must not panic.
	_ = c.Config.Inputs
}
