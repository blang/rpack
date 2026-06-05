// Package cmd implements the test command.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const testScriptTemplate = `#!/bin/bash
# Test: %s
# Add your --set and --set-input flags below, then add assertions.
set -e
DEFDIR="$1"
OUTDIR="$2"

rpack run --def "$DEFDIR" \
  --output-dir "$OUTDIR"
  # --set key=value
  # --set-input name=./fixture.yaml

# Check execution succeeded
if command -v jq &>/dev/null; then
    jq -e '.success' "$OUTDIR/meta.json" || {
        echo "FAIL: $(jq -r '.error' "$OUTDIR/meta.json")"
        exit 1
    }
fi

# Add assertions here:
# grep -q 'expected' "$OUTDIR/output.txt" || { echo "FAIL: ..."; exit 1; }
# test ! -f "$OUTDIR/unwanted.txt" || { echo "FAIL: ..."; exit 1; }
`

var testCmd = &cobra.Command{
	Use:   "test --def <dir> [--filter <name>] [--init <name>]",
	Short: "Run rpack definition tests",
	Long: `Discover and run test scripts in a definition's tests/ directory.

Each test is a subdirectory of tests/ containing an executable script
(run.sh, run.py, or run). The script receives two arguments:
  $1 = path to the definition directory
  $2 = path to a temp output directory
Exit 0 for pass, non-zero for fail.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defDir, err := cmd.Flags().GetString("def")
		if err != nil {
			return err
		}
		if defDir == "" {
			return cmd.Usage()
		}

		filter, _ := cmd.Flags().GetString("filter")
		initName, _ := cmd.Flags().GetString("init")

		if initName != "" {
			return initTest(defDir, initName)
		}

		return runTests(defDir, filter)
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.Flags().StringP("def", "d", "", "Path to rpack definition directory (required)")
	testCmd.Flags().StringP("filter", "", "", "Run only tests whose name contains this substring")
	testCmd.Flags().StringP("init", "", "", "Scaffold a new test directory")
}

// runTests discovers and executes all test scripts in tests/*/.
func runTests(defDir, filter string) error {
	testsDir := filepath.Join(defDir, "tests")
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no tests found in %s\n", testsDir)
		return nil //nolint:nilerr // not every definition needs tests
	}

	type testCase struct {
		name   string
		script string
	}
	var tests []testCase

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filter != "" && !strings.Contains(name, filter) {
			continue
		}
		script := findScript(filepath.Join(testsDir, name))
		if script == "" {
			continue
		}
		tests = append(tests, testCase{name: name, script: script})
	}

	if len(tests) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: no tests found in %s\n", testsDir)
		return nil
	}

	passed := 0
	failed := 0

	for _, tc := range tests {
		outDir, tmpErr := os.MkdirTemp("", "rpack-test-*")
		if tmpErr != nil {
			fmt.Fprintf(os.Stderr, "FAIL  %s (could not create temp dir: %v)\n", tc.name, tmpErr)
			failed++
			continue
		}

		start := time.Now()
		cmd := exec.Command(tc.script, defDir, outDir) //nolint:gosec // script path from trusted test discovery, defDir/outDir from CLI
		cmd.Dir = filepath.Join(testsDir, tc.name)
		output, runErr := cmd.CombinedOutput()
		elapsed := time.Since(start)

		_ = os.RemoveAll(outDir)

		if runErr != nil {
			fmt.Printf("FAIL  %-40s (%s)\n", tc.name, elapsed.Round(time.Millisecond))
			if len(output) > 0 {
				outStr := strings.TrimSpace(string(output))
				fmt.Printf("      %s\n", strings.ReplaceAll(outStr, "\n", "\n      "))
			}
			failed++
		} else {
			fmt.Printf("PASS  %-40s (%s)\n", tc.name, elapsed.Round(time.Millisecond))
			passed++
		}
	}

	total := passed + failed
	fmt.Printf("\n%d tests: %d passed, %d failed\n", total, passed, failed)

	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}
	return nil
}

// findScript finds an executable test script in a directory.
// Tries run, run.sh, run.py in order.
func findScript(dir string) string {
	for _, name := range []string{"run", "run.sh", "run.py"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode()&0o111 != 0 { // executable bit set
			return path
		}
	}
	return ""
}

// initTest scaffolds a new test directory with a template run.sh.
func initTest(defDir, name string) error {
	testsDir := filepath.Join(defDir, "tests")
	testDir := filepath.Join(testsDir, name)

	if err := os.MkdirAll(testDir, 0o755); err != nil { //nolint:gosec // standard permissions
		return fmt.Errorf("could not create test directory: %w", err)
	}

	scriptPath := filepath.Join(testDir, "run.sh")
	content := fmt.Sprintf(testScriptTemplate, name)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil { //nolint:gosec // executable script
		return fmt.Errorf("could not write test script: %w", err)
	}

	fmt.Printf("Created %s\n", scriptPath)
	return nil
}
