package rpack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pkgerrors "github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/blang/rpack/pkg/rpack/util"
)

// Sentinel errors for execution phases.
// These are used by classifyError to categorize failures.
var (
	ErrSchemaValidation = errors.New("schema validation failed")
	ErrInputValidation  = errors.New("input validation failed")
	ErrLuaExecution     = errors.New("lua execution failed")
	ErrPurityCheck      = errors.New("purity check failed")
)

// Executor runs rpack operations.
type Executor struct {
	// OutputDir overrides the target directory for output files.
	OutputDir string

	// Override for the execution path, optional
	OverrideExecPath string

	// Do not copy files at the end
	DryRun bool

	// Force the overwrite or removal of modified file
	// based on tracking using the lockfile
	Force bool
}

// execResult holds metadata about a completed execution.
type execResult struct {
	FilesRead    []string
	FilesWritten []string
	InputsUsed   []string
}

// classifyError determines the execution phase from an error.
// Uses sentinel errors for reliable classification.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrSchemaValidation) {
		return "schema_validation"
	}
	if errors.Is(err, ErrInputValidation) {
		return "input_validation"
	}
	if errors.Is(err, ErrPurityCheck) {
		return "purity_check"
	}
	if errors.Is(err, ErrLuaExecution) {
		return "lua_execution"
	}
	return "unknown"
}

// execCore runs the shared validation→execution→checks pipeline.
// It returns the RPackFS so the caller can access TargetWriteHandles()
// for file relocation and drain the recorder for metadata.
//
//nolint:gocognit,gocyclo // intentional: complex orchestration logic
func (e *Executor) execCore(ctx context.Context,
	defDir string,
	runDir string,
	tempDir string,
	resolvedInputs []*RPackResolvedInput,
	values map[string]any,
	inputNames []string,
	configValues map[string]any,
) (*RPackFS, *execResult, error) {
	definst, err := SetupRPackDefInstance(defDir)
	if err != nil {
		return nil, nil, pkgerrors.Wrapf(err, "Could not setup RPackDef")
	}

	// Validate config values against schema.cue if present.
	// Note: For direct execution (--def mode), we construct a synthetic config
	// where Inputs maps name→name. This satisfies the schema validation requirement
	// that all inputs be declared, while the actual input paths are in resolvedInputs.
	config := &RPackConfig{
		Config: &RPackConfigConfig{
			Values: configValues,
			Inputs: make(map[string]string),
		},
	}
	for _, name := range inputNames {
		config.Config.Inputs[name] = name // Synthetic: actual paths are in resolvedInputs
	}
	err = definst.ValidateConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate config values against definition schema: %w: %w", ErrSchemaValidation, err)
	}

	// Validate inputs
	err = ValidateRPackInputs(resolvedInputs, definst.Def.Inputs)
	if err != nil {
		return nil, nil, fmt.Errorf("validation of inputs failed: %w: %w", ErrInputValidation, err)
	}

	// Setup filesystem for file access.
	fs := NewRPackFS(true, defDir, runDir, tempDir, "", resolvedInputs)

	// Setup external data
	externalData := make(map[string]any)
	externalData["values"] = values
	externalData["inputs"] = inputNames

	// Read script file to string
	scriptBytes, err := os.ReadFile(definst.ScriptPath) //nolint:gosec // path comes from rpack definition
	if err != nil {
		return nil, nil, pkgerrors.Wrapf(err, "Failed to open script file: %s", definst.ScriptPath)
	}
	// Execute lua in context and capture changed files
	err = ExecuteLuaWithData(ctx, string(scriptBytes), fs, externalData)
	if err != nil {
		return fs, nil, fmt.Errorf("failed to execute script: %w: %w", ErrLuaExecution, err)
	}
	slog.Debug("Script execution successful")

	err = fs.Check()
	if err != nil {
		return fs, nil, fmt.Errorf("file access check failed: %w: %w", ErrPurityCheck, err)
	}

	// Drain recorder into result
	result := &execResult{}
	fsRecords := fs.Recorder().Records()

	// Log filesystem interactions
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

	seenReads := make(map[string]struct{})
	seenWrites := make(map[string]struct{})
	seenInputs := make(map[string]struct{})

	for _, record := range fsRecords {
		fp := record.Handle.FriendlyPath()
		resolver := record.Handle.Resolver()
		switch record.Typ {
		case FSAccessTypeRead:
			if _, ok := seenReads[fp]; !ok {
				result.FilesRead = append(result.FilesRead, fp)
				seenReads[fp] = struct{}{}
			}
			if resolver == MapResolver {
				// Extract input name from map:name or map:name/subpath
				name := fp
				if after, ok := strings.CutPrefix(name, "map:"); ok {
					name = after
					if idx := strings.Index(name, "/"); idx >= 0 {
						name = name[:idx]
					}
				}
				if _, ok := seenInputs[name]; !ok {
					result.InputsUsed = append(result.InputsUsed, name)
					seenInputs[name] = struct{}{}
				}
			}
		case FSAccessTypeWrite:
			if resolver == TargetResolver {
				relPath := record.Handle.IndirectTargetPath()
				if _, ok := seenWrites[relPath]; !ok {
					result.FilesWritten = append(result.FilesWritten, relPath)
					seenWrites[relPath] = struct{}{}
				}
			}
		}
	}

	return fs, result, nil
}

// printDryRunOutput prints all files in runDir to stdout in a
// deterministic format suitable for human inspection.
func printDryRunOutput(runDir string) error {
	var files []string
	err := filepath.Walk(runDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(runDir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to walk run directory")
	}

	sort.Strings(files)

	for _, relPath := range files {
		absPath := filepath.Join(runDir, relPath)
		content, rdErr := os.ReadFile(absPath) //nolint:gosec // path constructed from known run directory
		if rdErr != nil {
			return pkgerrors.Wrapf(rdErr, "Failed to read file: %s", relPath)
		}
		fmt.Printf("=== ./%s ===\n", relPath)
		_, _ = os.Stdout.Write(content)
		fmt.Println()
	}

	fmt.Fprintf(os.Stderr, "Wrote %d files to %s\n", len(files), runDir)
	return nil
}

// writeMetaJSON writes a meta.json file to the output directory.
func writeMetaJSON(outputDir string, result *execResult, execErr error) error {
	filesRead := []string{}
	filesWritten := []string{}
	inputsUsed := []string{}
	if result != nil {
		if result.FilesRead != nil {
			filesRead = result.FilesRead
		}
		if result.FilesWritten != nil {
			filesWritten = result.FilesWritten
		}
		if result.InputsUsed != nil {
			inputsUsed = result.InputsUsed
		}
	}
	meta := map[string]any{
		"success":       execErr == nil,
		"error":         nil,
		"error_phase":   nil,
		"files_read":    filesRead,
		"files_written": filesWritten,
		"inputs_used":   inputsUsed,
	}
	if execErr != nil {
		meta["error"] = execErr.Error()
		meta["error_phase"] = classifyError(execErr)
	}

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to marshal meta.json")
	}
	metaPath := filepath.Join(outputDir, "meta.json")
	if writeErr := os.WriteFile(metaPath, b, 0o644); writeErr != nil { //nolint:gosec // standard permissions for meta.json
		return pkgerrors.Wrap(writeErr, "Failed to write meta.json")
	}
	return nil
}

// copyDir copies all files from src to dst, creating directories as needed.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755) //nolint:gosec // standard permissions
		}

		content, rdErr := os.ReadFile(path) //nolint:gosec // path from Walk, trusted source
		if rdErr != nil {
			return pkgerrors.Wrapf(rdErr, "Failed to read: %s", path)
		}
		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // standard permissions
			return pkgerrors.Wrapf(mkErr, "Failed to create dir: %s", filepath.Dir(targetPath))
		}
		if wrErr := os.WriteFile(targetPath, content, 0o644); wrErr != nil { //nolint:gosec // standard permissions
			return pkgerrors.Wrapf(wrErr, "Failed to write: %s", targetPath)
		}
		return nil
	})
}

// ExecRPack loads and executes an rpack from the
// source file specified in `name`.
//
//nolint:gocognit,gocyclo // intentional: complex orchestration logic
func (e *Executor) ExecRPack(ctx context.Context, name string) error {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return pkgerrors.Wrapf(err, "Could not load rpack config: %s", name)
	}

	execPath := ci.ConfigPath
	if e.OverrideExecPath != "" {
		execPath = e.OverrideExecPath
	}
	pi, loadErr := LoadRPack(ci, execPath)
	if loadErr != nil {
		return pkgerrors.Wrapf(loadErr, "Could not load rpack: %s", name)
	}

	values := pi.ConfigInstance.Config.Config.Values
	inputNames := lo.Keys(pi.ConfigInstance.Config.Config.Inputs)
	configValues := pi.ConfigInstance.Config.Config.Values

	fs, result, execErr := e.execCore(ctx, pi.SourcePath, pi.RunPath, pi.TempPath, pi.ResolvedInputs, values, inputNames, configValues)

	if execErr != nil {
		if e.OutputDir != "" {
			if mkErr := os.MkdirAll(e.OutputDir, 0o755); mkErr != nil { //nolint:gosec // standard permissions
				slog.Warn("Failed to create output directory for meta.json", "dir", e.OutputDir, "error", mkErr)
			} else if metaErr := writeMetaJSON(e.OutputDir, result, execErr); metaErr != nil {
				slog.Warn("Failed to write meta.json", "dir", e.OutputDir, "error", metaErr)
			}
		}
		return execErr
	}

	if e.DryRun {
		if e.OutputDir != "" {
			if cpErr := copyDir(pi.RunPath, e.OutputDir); cpErr != nil {
				return pkgerrors.Wrap(cpErr, "Failed to copy files to output directory")
			}
			if metaErr := writeMetaJSON(e.OutputDir, result, nil); metaErr != nil {
				return metaErr
			}
		}
		return printDryRunOutput(pi.RunPath)
	}

	if e.OutputDir != "" {
		if !e.Force {
			entries, rdErr := os.ReadDir(e.OutputDir)
			if rdErr == nil && len(entries) > 0 {
				return pkgerrors.Errorf("output directory %s is not empty, use --force to overwrite", e.OutputDir)
			}
		}
		if mkErr := os.MkdirAll(e.OutputDir, 0o755); mkErr != nil { //nolint:gosec // standard permissions
			return pkgerrors.Wrapf(mkErr, "Could not create output directory: %s", e.OutputDir)
		}
		if cpErr := copyDir(pi.RunPath, e.OutputDir); cpErr != nil {
			return pkgerrors.Wrap(cpErr, "Failed to copy files to output directory")
		}
		return writeMetaJSON(e.OutputDir, result, nil)
	}

	// Copy/Rename files from run directory to execPath
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

		var chsum string
		chsum, err = util.Sha256File(absPath)
		if err != nil {
			return pkgerrors.Wrapf(err, "Failed to calculate checksum of: %s", absPath)
		}
		checksums[absPath] = chsum

		filesToMove = append(filesToMove, c)
		visitedPaths[absPath] = struct{}{}
	}

	oldLock := ci.LockFile
	oldLockIntegrity, err := oldLock.CheckIntegrity(execPath)
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to check lockfile integrity")
	}
	if len(oldLockIntegrity.Modified) > 0 {
		modFilesStr := strings.Join(oldLockIntegrity.Modified, ",")
		slog.Warn("Some files in lockfile were modified outside of rpack", "files", modFilesStr)
		if !e.Force {
			return pkgerrors.Errorf("Some locked files were modified outside of rpack, use force flag to ignore: %s", modFilesStr)
		}
	}

	if len(oldLockIntegrity.Removed) > 0 {
		slog.Warn("Some files in lockfile were removed outside of rpack", "files", strings.Join(oldLockIntegrity.Removed, ","))
	}

	newLockfile := NewRPackLockFile()
	for _, wFile := range filesToMove {
		chsum, ok := checksums[wFile.AbsPath]
		if !ok {
			panic("Can't find checksum for file")
		}
		newLockfile.AddFile(wFile.Path, chsum)
	}

	changes := newLockfile.Changes(oldLock)
	slog.Info("New files in lockfile", "files", changes.Added)
	slog.Info("Files no longer maintained by rpack, removing", "files", changes.Removed)

	for _, added := range changes.Added {
		targetFile := filepath.Clean(filepath.Join(execPath, added))
		var exists bool
		exists, err = util.FileExists(targetFile)
		if exists {
			slog.Warn("File is not managed by rdef but will be overwritten", "file", added)
			if !e.Force {
				return pkgerrors.Errorf("Existing file would need to be overwritten, use force flag to ignore: %s", added)
			}
		} else if err != nil {
			return pkgerrors.Wrapf(err, "Failed to check file exists: %s", added)
		}
	}

	for _, wFile := range filesToMove {
		targetFile := filepath.Clean(filepath.Join(execPath, wFile.Path))
		if err = os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil { //nolint:gosec // standard permissions
			return pkgerrors.Wrapf(err, "Failed to create dirs for: %s", targetFile)
		}
		err = os.Rename(wFile.AbsPath, targetFile)
		if err != nil {
			return pkgerrors.Wrapf(err, "Failed to move file %s to exec path %s", wFile.Path, execPath)
		}
	}

	for _, removedFile := range changes.Removed {
		p := filepath.Join(execPath, removedFile)
		var exists bool
		exists, err = util.FileExists(p)
		if err != nil {
			return pkgerrors.Wrapf(err, "Could not check deprecated file: %s", removedFile)
		}
		if exists {
			err = os.Remove(p)
			if err != nil {
				return pkgerrors.Wrapf(err, "Could not remove deprecated file: %s", removedFile)
			}
		} else {
			slog.Warn("File managed by rpack but marked for removal, does no longer exist, ignoring", "file", removedFile)
		}
	}

	err = newLockfile.WriteFile(ci.LockFilePath)
	if err != nil {
		return pkgerrors.Wrapf(err, "Could not write lockfile to %s", ci.LockFilePath)
	}

	return nil
}

// ExecRPackDirect runs an rpack from a local definition directory
// with programmatically supplied values and inputs.
//
//nolint:gocognit,gocyclo // intentional: orchestration logic
func (e *Executor) ExecRPackDirect(ctx context.Context, defDir string, values map[string]any, inputs map[string]string) error {
	absDefDir, err := filepath.Abs(defDir)
	if err != nil {
		return pkgerrors.Wrapf(err, "Could not resolve definition directory: %s", defDir)
	}

	runDir, err := os.MkdirTemp("", "rpack-run-*")
	if err != nil {
		return pkgerrors.Wrap(err, "Could not create run directory")
	}
	defer func() { _ = os.RemoveAll(runDir) }()

	tempDir, err := os.MkdirTemp("", "rpack-tmp-*")
	if err != nil {
		return pkgerrors.Wrap(err, "Could not create temp directory")
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Resolve inputs directly, supporting both relative and absolute paths.
	var resolvedInputs []*RPackResolvedInput
	for name, userPath := range inputs {
		cleanPath := filepath.Clean(userPath)
		absPath := cleanPath
		if !filepath.IsAbs(cleanPath) {
			cwd, wdErr := os.Getwd()
			if wdErr != nil {
				return pkgerrors.Wrap(wdErr, "Could not get working directory")
			}
			absPath = filepath.Join(cwd, cleanPath)
		}
		isDir, statErr := util.CheckFileOrDirExists(absPath)
		if statErr != nil {
			return pkgerrors.Wrapf(statErr, "User path %s=%s does not exist", name, userPath)
		}
		fileType := RPackInputTypeFile
		if isDir {
			fileType = RPackInputTypeDirectory
		}
		resolvedInputs = append(resolvedInputs, &RPackResolvedInput{
			Name:         name,
			UserPath:     cleanPath,
			ResolvedPath: absPath,
			Type:         fileType,
		})
	}

	inputNames := lo.Keys(inputs)
	configValues := values

	var result *execResult
	var execErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = pkgerrors.Errorf("Lua execution panicked: %v", r)
			}
		}()
		_, result, execErr = e.execCore(ctx, absDefDir, runDir, tempDir, resolvedInputs, values, inputNames, configValues)
	}()

	if execErr != nil {
		if e.OutputDir != "" {
			if mkErr := os.MkdirAll(e.OutputDir, 0o755); mkErr != nil { //nolint:gosec // standard permissions
				slog.Warn("Failed to create output directory for meta.json", "dir", e.OutputDir, "error", mkErr)
			} else if metaErr := writeMetaJSON(e.OutputDir, result, execErr); metaErr != nil {
				slog.Warn("Failed to write meta.json", "dir", e.OutputDir, "error", metaErr)
			}
		}
		return execErr
	}

	if e.DryRun {
		return printDryRunOutput(runDir)
	}

	if e.OutputDir != "" {
		if !e.Force {
			entries, rdErr := os.ReadDir(e.OutputDir)
			if rdErr == nil && len(entries) > 0 {
				return pkgerrors.Errorf("output directory %s is not empty, use --force to overwrite", e.OutputDir)
			}
		}
		if mkErr := os.MkdirAll(e.OutputDir, 0o755); mkErr != nil { //nolint:gosec // standard permissions for output directory
			return pkgerrors.Wrapf(mkErr, "Could not create output directory: %s", e.OutputDir)
		}
		if cpErr := copyDir(runDir, e.OutputDir); cpErr != nil {
			return pkgerrors.Wrap(cpErr, "Failed to copy files to output directory")
		}
		return writeMetaJSON(e.OutputDir, result, nil)
	}

	// No --output-dir and no --dry-run: write files to CWD.
	if cpErr := copyDir(runDir, "."); cpErr != nil {
		return pkgerrors.Wrap(cpErr, "Failed to copy files to working directory")
	}

	return nil
}
