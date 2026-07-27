package getsource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// runGit runs a git command with a sanitized environment (no gitRepoEnvVars
// leaking in from a potentially poisoned test environment) and fixed
// author/committer identity.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cleanEnv := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.Contains(gitRepoEnvVars, name) {
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

// poisonGitEnv points all repository-locating git environment variables at
// foreignDir, simulating a rpack invocation from within a git hook or a
// bare-repo/dotfiles setup.
func poisonGitEnv(t *testing.T, foreignDir string) {
	t.Helper()
	t.Setenv("GIT_DIR", filepath.Join(foreignDir, ".git"))
	t.Setenv("GIT_WORK_TREE", foreignDir)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(foreignDir, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreignDir, ".git", "index"))
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(foreignDir, ".git", "objects"))
	t.Setenv("GIT_ALTERNATE_OBJECT_DIRECTORIES", filepath.Join(foreignDir, ".git", "objects", "pack"))
	t.Setenv("GIT_NAMESPACE", "poisoned")
	t.Setenv("GIT_QUARANTINE_PATH", filepath.Join(foreignDir, ".git", "objects"))
}

// TestFetcher_FetchGitSourceWithPoisonedGitEnv is a regression test for the
// bug where a rpack run executed with GIT_DIR/GIT_WORK_TREE set (e.g. from a
// git hook) made go-getter's embedded git commands operate on that foreign
// repository instead of the temporary source checkout, failing with
// "error: remote origin already exists" (or worse, mutating the foreign
// repository).
func TestFetcher_FetchGitSourceWithPoisonedGitEnv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	srcDir := t.TempDir()
	initGitRepo(t, srcDir, map[string]string{
		"rpack.yaml":     "name: test\n",
		"sub/inner.txt":  "inner-content\n",
		"sub/nested.txt": "nested-content\n",
	})

	// The foreign repository the poisoned environment points at. It has an
	// "origin" remote so a hijacked "git remote add origin" would fail with
	// "error: remote origin already exists".
	foreignDir := t.TempDir()
	initGitRepo(t, foreignDir, map[string]string{"keep.txt": "untouched\n"})
	runGit(t, foreignDir, "remote", "add", "origin", "https://example.invalid/foreign.git")
	foreignHeadBefore := runGit(t, foreignDir, "rev-parse", "HEAD")
	foreignRemotesBefore := runGit(t, foreignDir, "remote", "-v")

	poisonGitEnv(t, foreignDir)

	f := DefaultFetcher()
	srcAddr := "git::file://" + srcDir

	assertFile := func(t *testing.T, path, want string) {
		t.Helper()
		content, err := os.ReadFile(path) //nolint:gosec // test uses TempDir
		if err != nil {
			t.Fatalf("expected fetched file %s: %s", path, err)
		}
		if string(content) != want {
			t.Fatalf("unexpected content of %s: %q, want %q", path, content, want)
		}
	}

	// Clone path: destination does not exist yet. Fetching twice into fresh
	// destinations mirrors LoadRPack, which cleans its source cache dir
	// before every fetch (go-getter's update path for existing
	// destinations fails for ref-less sources).
	for i := range 2 {
		destDir := filepath.Join(t.TempDir(), "checkout")
		if err := f.Fetch(context.Background(), destDir, srcAddr); err != nil {
			t.Fatalf("Fetch %d (clone path) with poisoned git env failed: %s", i+1, err)
		}
		assertFile(t, filepath.Join(destDir, "rpack.yaml"), "name: test\n")
	}

	// Subdir path: go-getter redirects to an internal temp dir first and
	// copies the subdirectory over afterwards.
	subDestDir := filepath.Join(t.TempDir(), "checkout-sub")
	if err := f.Fetch(context.Background(), subDestDir, srcAddr+"//sub"); err != nil {
		t.Fatalf("Fetch (subdir) with poisoned git env failed: %s", err)
	}
	assertFile(t, filepath.Join(subDestDir, "inner.txt"), "inner-content\n")

	// The environment must have been restored after the fetches.
	if got := os.Getenv("GIT_DIR"); got != filepath.Join(foreignDir, ".git") {
		t.Fatalf("GIT_DIR was not restored after Fetch, got %q", got)
	}

	// The foreign repository must be completely untouched.
	if got := runGit(t, foreignDir, "rev-parse", "HEAD"); got != foreignHeadBefore {
		t.Fatalf("foreign repository HEAD changed: %s -> %s", foreignHeadBefore, got)
	}
	if got := runGit(t, foreignDir, "remote", "-v"); got != foreignRemotesBefore {
		t.Fatalf("foreign repository remotes changed:\nbefore: %s\nafter:  %s", foreignRemotesBefore, got)
	}
	if _, err := os.Stat(filepath.Join(foreignDir, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatal("foreign repository has a FETCH_HEAD: a fetch was redirected into it")
	}
	if got := runGit(t, foreignDir, "status", "--porcelain"); got != "" {
		t.Fatalf("foreign repository work tree modified:\n%s", got)
	}
}

// TestFetcher_FetchGitSourceUpdatePathWithPoisonedGitEnv covers the
// originally reported failure mode: re-fetching into an EXISTING destination
// makes go-getter take its "update" path (git init/remote add/fetch/reset
// inside the destination). With GIT_DIR exported — as git does for hook
// processes — the embedded "git remote add origin" operated on the foreign
// repository and failed with "error: remote origin already exists" (and
// would have mutated it: git fetch into it, git reset --hard FETCH_HEAD).
//
// The source must pin a ref: the update path fails for ref-less sources with
// "invalid ref: \"\"", which is the separate defect that made LoadRPack clean
// its source cache dir before every fetch (see TestLoadRPack_RefetchSources).
func TestFetcher_FetchGitSourceUpdatePathWithPoisonedGitEnv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	srcDir := t.TempDir()
	initGitRepo(t, srcDir, map[string]string{
		"rpack.yaml": "name: test\n",
	})

	// The foreign repository GIT_DIR points at. It has an "origin" remote,
	// so a hijacked "git remote add origin" fails with
	// "error: remote origin already exists".
	foreignDir := t.TempDir()
	initGitRepo(t, foreignDir, map[string]string{"keep.txt": "untouched\n"})
	runGit(t, foreignDir, "remote", "add", "origin", "https://example.invalid/foreign.git")
	foreignHeadBefore := runGit(t, foreignDir, "rev-parse", "HEAD")
	foreignRemotesBefore := runGit(t, foreignDir, "remote", "-v")

	// GIT_DIR only: this is what git exports to hook processes, and it does
	// not break the initial clone (git clone ignores GIT_DIR), so the second
	// fetch reaches the update path — the exact reported failure.
	t.Setenv("GIT_DIR", filepath.Join(foreignDir, ".git"))

	f := DefaultFetcher()
	destDir := filepath.Join(t.TempDir(), "checkout")
	srcAddr := "git::file://" + srcDir + "?ref=main"
	for i := range 2 {
		if err := f.Fetch(context.Background(), destDir, srcAddr); err != nil {
			t.Fatalf("Fetch %d (second fetch takes the update path) failed: %s", i+1, err)
		}
		content, err := os.ReadFile(filepath.Join(destDir, "rpack.yaml")) //nolint:gosec // test uses TempDir
		if err != nil {
			t.Fatalf("expected fetched file: %s", err)
		}
		if string(content) != "name: test\n" {
			t.Fatalf("unexpected content: %q, want %q", content, "name: test\n")
		}
	}

	// The environment must have been restored after the fetches.
	if got := os.Getenv("GIT_DIR"); got != filepath.Join(foreignDir, ".git") {
		t.Fatalf("GIT_DIR was not restored after Fetch, got %q", got)
	}

	// The foreign repository must be completely untouched.
	if got := runGit(t, foreignDir, "rev-parse", "HEAD"); got != foreignHeadBefore {
		t.Fatalf("foreign repository HEAD changed: %s -> %s", foreignHeadBefore, got)
	}
	if got := runGit(t, foreignDir, "remote", "-v"); got != foreignRemotesBefore {
		t.Fatalf("foreign repository remotes changed:\nbefore: %s\nafter:  %s", foreignRemotesBefore, got)
	}
	if _, err := os.Stat(filepath.Join(foreignDir, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatal("foreign repository has a FETCH_HEAD: a fetch was redirected into it")
	}
	if got := runGit(t, foreignDir, "status", "--porcelain"); got != "" {
		t.Fatalf("foreign repository work tree modified:\n%s", got)
	}
}

func TestWithoutGitRepoEnv(t *testing.T) {
	t.Setenv("GIT_DIR", "/poisoned/.git")
	// Ensure GIT_NAMESPACE is not set beforehand.
	if _, set := os.LookupEnv("GIT_NAMESPACE"); set {
		t.Skip("GIT_NAMESPACE is set in the test environment")
	}

	fnCalled := false
	err := withoutGitRepoEnv(func() error {
		fnCalled = true
		if _, set := os.LookupEnv("GIT_DIR"); set {
			t.Error("GIT_DIR still set inside withoutGitRepoEnv")
		}
		if _, set := os.LookupEnv("GIT_NAMESPACE"); set {
			t.Error("GIT_NAMESPACE unexpectedly set inside withoutGitRepoEnv")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withoutGitRepoEnv returned error: %s", err)
	}
	if !fnCalled {
		t.Fatal("withoutGitRepoEnv did not call fn")
	}

	if got := os.Getenv("GIT_DIR"); got != "/poisoned/.git" {
		t.Errorf("GIT_DIR not restored, got %q", got)
	}
	if _, set := os.LookupEnv("GIT_NAMESPACE"); set {
		t.Error("GIT_NAMESPACE was set by withoutGitRepoEnv but not unset afterwards")
	}
}

func TestWithoutGitRepoEnvPassesThroughConfigVars(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.git@github.com:.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/")
	t.Setenv("GIT_SSH_COMMAND", "ssh -i /tmp/key")

	err := withoutGitRepoEnv(func() error {
		for _, name := range []string{"GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_SSH_COMMAND"} {
			if os.Getenv(name) == "" {
				t.Errorf("%s was scrubbed but must be kept for authentication/config", name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withoutGitRepoEnv returned error: %s", err)
	}
}
