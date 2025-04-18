package rpack

import (
	"context"
	"log/slog"
	"strings"

	"github.com/pkg/errors"
)

// Checker checks certain aspects of an rpack
type Checker struct {
	// Override for the execution path, optional
	// Must be absolute
	OverrideExecPath string
}

func (c *Checker) CheckIntegrity(ctx context.Context, name string) error {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return errors.Wrapf(err, "Could not load rpack config: %s", name)
	}

	execPath := ci.ConfigPath
	if c.OverrideExecPath != "" {
		execPath = c.OverrideExecPath
	}
	oldLockIntegrity, err := ci.LockFile.CheckIntegrity(execPath)
	if err != nil {
		return errors.Wrap(err, "Failed to check lockfile integrity")
	}
	// Require force flag if files were modified that should be controlled by lockfile
	if len(oldLockIntegrity.Modified) > 0 {
		modFilesStr := strings.Join(oldLockIntegrity.Modified, ",")
		slog.Warn("Some files in lockfile were modified outside of rpack", "files", modFilesStr)
		return errors.Errorf("Some locked files were modified outside of rpack, use force flag to ignore: %s", modFilesStr)
	}

	// Warn about files that are removed but still in the lockfile
	if len(oldLockIntegrity.Removed) > 0 {
		slog.Warn("Some files in lockfile were removed outside of rpack", "files", strings.Join(oldLockIntegrity.Removed, ","))
		return errors.Errorf("Some files in lockfile were removed: %s", strings.Join(oldLockIntegrity.Removed, ","))
	}
	return nil
}
