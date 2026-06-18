package rpack

import (
	"context"
	"log/slog"
	"strings"

	"fmt"
)

// Checker checks certain aspects of an rpack
type Checker struct {
	// Override for the execution path, optional
	// Must be absolute
	OverrideExecPath string
}

// CheckIntegrity verifies the integrity of an rpack installation.
func (c *Checker) CheckIntegrity(ctx context.Context, name string) error {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return fmt.Errorf("could not load rpack config: %s: %w", name, err)
	}

	execPath := ci.ConfigPath
	if c.OverrideExecPath != "" {
		execPath = c.OverrideExecPath
	}
	oldLockIntegrity, err := ci.LockFile.CheckIntegrity(execPath)
	if err != nil {
		return fmt.Errorf("failed to check lockfile integrity: %w", err)
	}
	// Require force flag if files were modified that should be controlled by lockfile
	if len(oldLockIntegrity.Modified) > 0 {
		modFilesStr := strings.Join(oldLockIntegrity.Modified, ",")
		slog.Warn("Some files in lockfile were modified outside of rpack", "files", modFilesStr)
		return fmt.Errorf("some locked files were modified outside of rpack, use force flag to ignore: %s", modFilesStr)
	}

	// Warn about files that are removed but still in the lockfile
	if len(oldLockIntegrity.Removed) > 0 {
		slog.Warn("Some files in lockfile were removed outside of rpack", "files", strings.Join(oldLockIntegrity.Removed, ","))
		return fmt.Errorf("some files in lockfile were removed: %s", strings.Join(oldLockIntegrity.Removed, ","))
	}
	return nil
}
