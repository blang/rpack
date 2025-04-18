package rpack

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/rpack/pkg/rpack/util"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type Executor struct {

	// Override for the execution path, optional
	// Must be absolute
	OverrideExecPath string

	// Do not copy files at the end
	DryRun bool

	// Force the overwrite or removal of modified file
	// based on tracking using the lockfile
	Force bool
}

// ExecRPack loads and executes an rpack from the
// source file specified in `name`.
func (e *Executor) ExecRPack(ctx context.Context, name string) error {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return errors.Wrapf(err, "Could not load rpack config: %s", name)
	}

	execPath := ci.ConfigPath
	if e.OverrideExecPath != "" {
		execPath = e.OverrideExecPath
	}
	pi, err := LoadRPack(ci, execPath)
	if err != nil {
		return errors.Wrapf(err, "Could not load rpack: %s", name)
	}
	slog.Info("PI debug", "rpack", pi)

	definst, err := SetupRPackDefInstance(pi.SourcePath)
	if err != nil {
		return errors.Wrapf(err, "Could not setup RPackDef: %s", name)
	}

	// Validate config
	err = definst.ValidateConfig(ci.Config)
	if err != nil {
		return errors.Wrap(err, "Failed to validate rpack user config (inputs and values) against rpack definition schema")
	}

	// Validate inputs
	err = ValidateRPackInputs(pi.ResolvedInputs, definst.Def.Inputs)
	if err != nil {
		return errors.Wrap(err, "Validation of inputs failed")
	}

	// Setup filesystem for file access
	fs := NewRPackFS(true, pi.SourcePath, pi.RunPath, pi.TempPath, pi.ExecPath, pi.ResolvedInputs)

	// Setup external data
	externalData := make(map[string]interface{})
	externalData["values"] = pi.ConfigInstance.Config.Config.Values

	// Only supply a list of available input mappings to the script, instead of the users specified path.
	externalData["inputs"] = lo.Keys(pi.ConfigInstance.Config.Config.Inputs)

	// Read script file to string
	scriptBytes, err := os.ReadFile(definst.ScriptPath)
	if err != nil {
		return errors.Wrapf(err, "Failed to open script file: %s", definst.ScriptPath)
	}
	// Execute lua in context and capture changed files
	err = ExecuteLuaWithData(ctx, string(scriptBytes), fs, externalData)
	if err != nil {
		return errors.Wrap(err, "Failed to execute script")
	}
	slog.Info("Script execution successful")

	if err := fs.Check(); err != nil {
		return errors.Wrap(err, "File access check failed")
	}
	// Print files to be written
	fsRecords := fs.Recorder().Records()

	if slog.Default().Enabled(ctx, slog.LevelInfo) {
		type userRecord struct {
			Typ          string
			Resolver     string
			FriendlyPath string
		}
		var userRecords []userRecord
		for _, record := range fsRecords {
			userRecords = append(userRecords, userRecord{
				Typ:          record.Typ.String(),
				Resolver:     record.Handle.Resolver(),
				FriendlyPath: record.Handle.FriendlyPath(),
			})
		}
		slog.Info("Filesystem interactions:", "count", len(fsRecords), "records", userRecords)
	}

	if e.DryRun {
		// We are done, no copying or moving of files
		slog.Info("Dry-run output", "output-dir", pi.RunPath)
		return nil
	}

	// Steps prior for lockfile handling
	// - Traverse lockfile and check if files still exist
	// - If file does not exist but is in lockfile, print warning
	// - If file exists but has different checksum, abort unless force flag

	// Steps:
	// - Generate slice of files to actually move
	// - Calculate sha256 of file
	// - Move files
	// - Add files to lockfile
	// - Determine which files were not changed and remove those
	// - Remove files from lockfile
	// - Write lockfile
	// While moving files and creating dictories, we need to ensure that the lockfile stays
	// consistent. If the move fails at any point, all moved files should still be captured
	// in the lockfile. Otherwise we end up with orphaned resources.

	// Copy/Rename files from run directory to execPath
	// Since files can be written to multiple times, they actually might occur
	// multiple times in the WrittenFiles slice, but can only be moved once.
	// We keep track of the absolute paths so we move only once.
	visitedPaths := make(map[string]struct{})
	checksums := make(map[string]string)
	var filesToMove []*ControlledFile
	for _, handle := range fs.TargetWriteHandles() {

		relPath := handle.IndirectTargetPath()
		absPath := filepath.Clean(filepath.Join(pi.RunPath, relPath))
		c := &ControlledFile{
			Path:    relPath,
			AbsPath: absPath,
		}

		if _, ok := visitedPaths[absPath]; ok {
			slog.Debug("File was already moved, but written multiple times, skipping", "path", handle.FriendlyPath())
			continue
		}

		// Calculate checksum
		chsum, err := util.Sha256File(absPath)
		if err != nil {
			return errors.Wrapf(err, "Failed to calculate checksum of: %s", absPath)
		}
		checksums[absPath] = chsum

		filesToMove = append(filesToMove, c)

		visitedPaths[absPath] = struct{}{}
	}

	oldLock := ci.LockFile
	oldLockIntegrity, err := oldLock.CheckIntegrity(execPath)
	if err != nil {
		return errors.Wrap(err, "Failed to check lockfile integrity")
	}
	// Require force flag if files were modified that should be controlled by lockfile
	if len(oldLockIntegrity.Modified) > 0 {
		modFilesStr := strings.Join(oldLockIntegrity.Modified, ",")
		slog.Warn("Some files in lockfile were modified outside of rpack", "files", modFilesStr)
		if !e.Force {
			return errors.Errorf("Some locked files were modified outside of rpack, use force flag to ignore: %s", modFilesStr)
		}
	}

	// Warn about files that are removed but still in the lockfile
	if len(oldLockIntegrity.Removed) > 0 {
		slog.Warn("Some files in lockfile were removed outside of rpack", "files", strings.Join(oldLockIntegrity.Removed, ","))
	}

	// Build new Lockfile
	newLockfile := NewRPackLockFile()
	for _, wFile := range filesToMove {
		chsum, ok := checksums[wFile.AbsPath]
		if !ok {
			panic("Can't find checksum for file")
		}
		newLockfile.AddFile(wFile.Path, chsum)
	}

	// Compare lockfiles and remove files no longer under control
	changes := newLockfile.Changes(oldLock)
	slog.Info("New files in lockfile", "files", changes.Added)
	slog.Info("Files no longer maintained by rpack, removing", "files", changes.Removed)

	// Check overwrite of existing files
	for _, added := range changes.Added {
		targetFile := filepath.Clean(filepath.Join(execPath, added))
		if exists, err := util.FileExists(targetFile); exists {
			slog.Warn("File is not managed by rdef but will be overwritten", "file", added)
			if !e.Force {
				return errors.Errorf("Existing file would need to be overwritten, use force flag to ignore: %s", added)
			}
		} else if err != nil {
			return errors.Wrapf(err, "Failed to check file exists: %s", added)
		}
	}

	// Actually move files
	for _, wFile := range filesToMove {
		targetFile := filepath.Clean(filepath.Join(execPath, wFile.Path))

		// Ensure directory
		if err = os.MkdirAll(filepath.Dir(targetFile), 0755); err != nil {
			return errors.Wrapf(err, "Failed to create dirs for: %s", targetFile)
		}
		err := os.Rename(wFile.AbsPath, targetFile)
		// TODO: Somehow capture this moment and write lockfile still
		if err != nil {
			return errors.Wrapf(err, "Failed to move file %s to exec path %s", wFile.Path, execPath)
		}
	}

	// Remove removed files
	for _, removedFile := range changes.Removed {
		p := filepath.Join(execPath, removedFile)
		exists, err := util.FileExists(p)
		if err != nil {
			return errors.Wrapf(err, "Could not check deprecated file: %s", removedFile)
		}
		if exists {
			err := os.Remove(p)
			if err != nil {
				return errors.Wrapf(err, "Could not remove deprecated file: %s", removedFile)
			}
		} else {
			slog.Warn("File managed by rpack but marked for removal, does no longer exist, ignoring", "file", removedFile)
		}
	}

	err = newLockfile.WriteFile(ci.LockFilePath)
	if err != nil {
		return errors.Wrapf(err, "Could not write lockfile to %s", ci.LockFilePath)
	}

	return nil
}
