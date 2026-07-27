package getsource

import (
	"net/url"
	"os"

	getter "github.com/hashicorp/go-getter"
)

// gitRepoEnvVars lists environment variables that redirect which repository,
// work tree, index, or object store a git command operates on.
//
// go-getter's GitGetter shells out to the git binary without scrubbing the
// environment, so these variables leak from rpack's own environment into the
// embedded git commands (git init/remote/fetch/reset/checkout/...). When they
// are set — e.g. by a surrounding git hook (git exports GIT_DIR) or a
// bare-repo/dotfiles setup exporting GIT_DIR and GIT_WORK_TREE — every
// embedded git command operates on that foreign repository instead of the
// temporary source checkout. This breaks fetches ("error: remote origin
// already exists", "fatal: working tree ... already exists") and can even
// mutate the foreign repository (git fetch into it, git reset --hard
// FETCH_HEAD).
//
// Configuration and credential variables (GIT_CONFIG_*, GIT_SSH_COMMAND,
// GIT_ASKPASS, ...) are intentionally NOT listed: users legitimately rely on
// them for URL rewriting (insteadOf) and authentication.
var gitRepoEnvVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_QUARANTINE_PATH",
}

// gitGetter wraps go-getter's GitGetter and runs every embedded git command
// with the repository-locating environment variables (see gitRepoEnvVars)
// removed, so a poisoned parent environment cannot redirect git operations
// to a foreign repository.
type gitGetter struct {
	inner *getter.GitGetter
}

func (g *gitGetter) Get(dst string, u *url.URL) error {
	return withoutGitRepoEnv(func() error {
		return g.inner.Get(dst, u)
	})
}

func (g *gitGetter) GetFile(dst string, u *url.URL) error {
	return withoutGitRepoEnv(func() error {
		return g.inner.GetFile(dst, u)
	})
}

func (g *gitGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return g.inner.ClientMode(u)
}

func (g *gitGetter) SetClient(c *getter.Client) {
	g.inner.SetClient(c)
}

// withoutGitRepoEnv runs fn with all gitRepoEnvVars removed from the process
// environment and restores their previous values afterwards.
//
// The mutation is process-global for the duration of fn. Fetching is a
// synchronous, non-concurrent operation in rpack, so this is safe here; do
// not fetch concurrently with other process-spawning code.
func withoutGitRepoEnv(fn func() error) error {
	type savedEnv struct {
		value string
		set   bool
	}
	saved := make(map[string]savedEnv, len(gitRepoEnvVars))
	for _, name := range gitRepoEnvVars {
		value, set := os.LookupEnv(name)
		saved[name] = savedEnv{value: value, set: set}
		if set {
			_ = os.Unsetenv(name)
		}
	}
	defer func() {
		for name, s := range saved {
			if s.set {
				_ = os.Setenv(name, s.value)
			}
		}
	}()
	return fn()
}
