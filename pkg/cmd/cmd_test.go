package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// runRoot is a helper that executes the root command with the given args,
// capturing stdout and stderr. It returns the error from Execute (without
// os.Exit, which is handled by the package-level Execute) so tests can assert
// on exit-worthy conditions. Error is returned last per Go convention.
func runRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	rootCmd.SetArgs(args)
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)

	// SilenceUsage/SilenceErrors are per-command; ensure each test sees the
	// values configured on each subcommand, not a leak from a prior test.
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	err = rootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// TestVersionNonEmpty asserts that a build without ldflags still reports a
// non-empty version (BuildVersion defaults to "dev") so users can identify
// their build when filing bugs.
func TestVersionNonEmpty(t *testing.T) {
	stdout, _, err := runRoot(t, "--version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "rpack version") {
		t.Fatalf("expected version output, got: %q", stdout)
	}
	// "rpack version " followed by at least one non-whitespace char.
	rest := strings.TrimPrefix(strings.TrimSpace(stdout), "rpack version ")
	if rest == "" || strings.TrimSpace(rest) == "" {
		t.Fatalf("version string is empty, got: %q", stdout)
	}
}

// TestRequiredFlagsFail asserts that commands with required flags exit non-zero
// and print a single-line Cobra "required flag(s)" error rather than the full
// help (the old behaviour called cmd.Usage() and returned nil, exiting 0).
func TestRequiredFlagsFail(t *testing.T) {
	cases := []struct { //nolint:govet // fieldalignment is not critical in table-driven tests
		name string
		args []string
		flag string
	}{
		{"validate", []string{"validate"}, "def"},
		{"bundle", []string{"bundle"}, "def"},
		{"publish", []string{"publish"}, "def"},
		{"test", []string{"test"}, "def"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := runRoot(t, tc.args...)
			if err == nil {
				t.Fatalf("expected non-zero error for `%s` without --%s, got nil", tc.name, tc.flag)
			}
			// Cobra reports a missing required flag, not a usage dump.
			if !strings.Contains(stderr, "required flag") {
				t.Fatalf("expected 'required flag' in stderr, got: %q", stderr)
			}
			// With SilenceUsage enabled, the multi-page help must NOT be dumped.
			if strings.Contains(stderr, "Usage:") && strings.Contains(stderr, "Available Commands") {
				t.Fatalf("stderr should not contain full help dump, got: %q", stderr)
			}
		})
	}
}

// TestBundleMissingPartialFlags asserts that omitting only some required flags
// still fails (Cobra lists every missing flag).
func TestBundleMissingPartialFlags(t *testing.T) {
	// Only --def provided, --format and --output missing.
	_, stderr, err := runRoot(t, "bundle", "-d", "./does-not-matter-for-flag-validation")
	if err == nil {
		t.Fatalf("expected non-zero error, got nil")
	}
	if !strings.Contains(stderr, `"format"`) {
		t.Fatalf("expected 'format' in missing-flags error, got: %q", stderr)
	}
	if !strings.Contains(stderr, `"output"`) {
		t.Fatalf("expected 'output' in missing-flags error, got: %q", stderr)
	}
}

// TestRunDefShorthand asserts run registers -d/-o/-n shorthands, matching the
// -d sibling shorthand on validate/bundle/publish/test. Inspects the flagset
// directly to avoid executing the run pipeline and polluting singleton command
// state (a shared -h Execute would persist the help flag and silently skip
// RunE for every later run/test Execute in the suite).
func TestRunDefShorthand(t *testing.T) {
	// def and output-dir are local flags; dry-run is persistent. Both flagsets
	// are queried via the matching accessor.
	want := []struct {
		name, sh, accessor string
	}{
		{"def", "d", "local"},
		{"output-dir", "o", "local"},
		{"dry-run", "n", "persistent"},
	}
	for _, w := range want {
		var f *pflag.Flag
		switch w.accessor {
		case "local":
			f = runCmd.Flags().Lookup(w.name)
		case "persistent":
			f = runCmd.PersistentFlags().Lookup(w.name)
		}
		if f == nil {
			t.Errorf("expected flag --%s registered on run", w.name)
			continue
		}
		if f.Shorthand != w.sh {
			t.Errorf("flag --%s: expected shorthand %q, got %q", w.name, w.sh, f.Shorthand)
		}
	}
}

// TestCheckHasNoForceFlag asserts that `rpack check --force` is rejected: the
// command intentionally has no --force flag (force lives only on `rpack run`).
// The check error message directs users to `rpack run --force` instead.
func TestCheckHasNoForceFlag(t *testing.T) {
	_, stderr, err := runRoot(t, "check", "--force", "config.rpack.yaml")
	if err == nil {
		t.Fatalf("expected unknown-flag error, got nil")
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("expected 'unknown flag' error, got: %q", stderr)
	}
}

// TestTestStrictFlagRegistered asserts the --strict flag exists on `rpack test`.
// Inspected directly (not via `test -h`) to avoid poisoning testCmd's help flag
// for TestTestStrictFailsOnEmpty, which must run RunE.
func TestTestStrictFlagRegistered(t *testing.T) {
	if f := testCmd.Flags().Lookup("strict"); f == nil {
		t.Fatalf("expected --strict flag registered on test")
	}
}

// TestTestStrictFailsOnEmpty asserts that `rpack test --strict` against a
// definition with no tests/ directory exits non-zero (CI gating), while the
// default (no --strict) keeps the historical warn-and-succeed behaviour.
func TestTestStrictFailsOnEmpty(t *testing.T) {
	// Use a non-existent def dir so ReadDir fails; --strict must turn that into
	// an error. Without --strict, runTests warns and returns nil.
	missing := "/tmp/rpack-test-no-such-def-xyz"

	// Default: warn + succeed (no behaviour change for existing users).
	_, _, err := runRoot(t, "test", "-d", missing)
	if err != nil {
		t.Fatalf("expected nil error without --strict, got: %v", err)
	}

	// --strict: should fail because no tests are found.
	_, _, err = runRoot(t, "test", "-d", missing, "--strict")
	if err == nil {
		t.Fatalf("expected non-zero error with --strict on empty tests, got nil")
	}
}

// TestRunForceFlagExists asserts that `rpack run` registers --force. The
// `rpack check` error message directs users to `rpack run --force`, so this
// guards the other half of that contract: the referenced flag must exist on
// run (it is intentionally absent on check).
func TestRunForceFlagExists(t *testing.T) {
	if f := runCmd.PersistentFlags().Lookup("force"); f == nil {
		t.Fatalf("expected --force flag registered on run")
	}
}
