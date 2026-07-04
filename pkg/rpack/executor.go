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
		return nil, nil, fmt.Errorf("could not setup RPackDef: %w", err)
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
	if err = definst.ValidateConfig(config); err != nil {
		return nil, nil, fmt.Errorf("failed to validate config values against definition schema: %w: %w", ErrSchemaValidation, err)
	}

	// Validate inputs
	if err = ValidateRPackInputs(resolvedInputs, definst.Def.Inputs); err != nil {
		return nil, nil, fmt.Errorf("validation of inputs failed: %w: %w", ErrInputValidation, err)
	}

	// Setup filesystem for file access.
	fs := NewRPackFS(true, defDir, runDir, tempDir, "", resolvedInputs)

	// Setup external data
	externalData := map[string]any{
		"values": values,
		"inputs": inputNames,
	}

	// Read script file to string
	scriptBytes, err := os.ReadFile(definst.ScriptPath) //nolint:gosec // path comes from rpack definition
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open script file: %s: %w", definst.ScriptPath, err)
	}

	// Execute lua in context and capture changed files
	if err = ExecuteLuaWithData(ctx, string(scriptBytes), fs, externalData); err != nil {
		return fs, nil, fmt.Errorf("failed to execute script: %w: %w", ErrLuaExecution, err)
	}
	slog.Debug("Script execution successful")

	if err = fs.Check(); err != nil {
		return fs, nil, fmt.Errorf("file access check failed: %w: %w", ErrPurityCheck, err)
	}

	return fs, drainRecorder(ctx, fs), nil
}

// drainRecorder builds an execResult from the FSRecorder's records: distinct
// files read, distinct target files written, and distinct named inputs used
// (resolved from map: accesses). It also logs the interactions at Info level.
func drainRecorder(ctx context.Context, fs *RPackFS) *execResult {
	records := fs.Recorder().Records()
	logFSInteractions(ctx, records)
	return buildExecResult(records)
}

// logFSInteractions emits a structured slog.Info summary of every recorded
// filesystem interaction. No-op below Info level.
func logFSInteractions(ctx context.Context, records []FSRecorderRecord) {
	if !slog.Default().Enabled(ctx, slog.LevelInfo) {
		return
	}
	type userRecord struct {
		Typ          string
		Resolver     string
		FriendlyPath string
	}
	userRecords := make([]userRecord, 0, len(records))
	for _, record := range records {
		userRecords = append(userRecords, userRecord{
			Typ:          record.Typ.String(),
			Resolver:     record.Handle.Resolver(),
			FriendlyPath: record.Handle.FriendlyPath(),
		})
	}
	slog.Info("Filesystem interactions:", "count", len(records), "records", userRecords)
}

// buildExecResult aggregates recorder records into distinct files-read,
// target-files-written, and named-inputs-used lists.
func buildExecResult(records []FSRecorderRecord) *execResult {
	result := &execResult{}
	seenReads := make(map[string]struct{})
	seenWrites := make(map[string]struct{})
	seenInputs := make(map[string]struct{})

	for _, record := range records {
		switch record.Typ {
		case FSAccessTypeRead:
			recordRead(record, result, seenReads, seenInputs)
		case FSAccessTypeWrite:
			recordWrite(record, result, seenWrites)
		}
	}
	return result
}

// recordRead records a distinct file read and, for map: reads, a distinct
// named input usage.
func recordRead(record FSRecorderRecord, result *execResult, seenReads, seenInputs map[string]struct{}) {
	fp := record.Handle.FriendlyPath()
	if _, ok := seenReads[fp]; !ok {
		result.FilesRead = append(result.FilesRead, fp)
		seenReads[fp] = struct{}{}
	}
	if record.Handle.Resolver() == MapResolver {
		if name := mapInputName(fp); name != "" {
			if _, ok := seenInputs[name]; !ok {
				result.InputsUsed = append(result.InputsUsed, name)
				seenInputs[name] = struct{}{}
			}
		}
	}
}

// recordWrite records a distinct target file written.
func recordWrite(record FSRecorderRecord, result *execResult, seenWrites map[string]struct{}) {
	if record.Handle.Resolver() != TargetResolver {
		return
	}
	relPath := record.Handle.IndirectTargetPath()
	if _, ok := seenWrites[relPath]; !ok {
		result.FilesWritten = append(result.FilesWritten, relPath)
		seenWrites[relPath] = struct{}{}
	}
}

// mapInputName extracts the input name from a map:-prefixed friendly path
// ("map:users.yaml" → "users.yaml", "map:inputdir1/sub/x" → "inputdir1").
// Returns "" when the path is not map:-prefixed.
func mapInputName(fp string) string {
	name, ok := strings.CutPrefix(fp, "map:")
	if !ok {
		return ""
	}
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
	}
	return name
}

// finalizeOutput performs the shared output finalization for ExecRPack and
// ExecRPackDirect after execCore has run. It handles:
//   - the error path: best-effort meta.json into OutputDir, then returns the error;
//   - the dry-run path: optionally copies files into OutputDir and writes meta.json,
//     then prints the run directory to stdout;
//   - the --output-dir path: refuses a non-empty directory unless Force, copies the
//     run directory there, and writes meta.json.
//
// It returns handled=true when the terminal output path (dry-run or --output-dir)
// was taken, signaling the caller that the lockfile-protected file-move or CWD
// copy must NOT run. On the error path it returns (false, execErr) so the caller
// can return the error unchanged.
func (e *Executor) finalizeOutput(result *execResult, runDir string, execErr error) (bool, error) {
	if execErr != nil {
		writeErrorMeta(e.OutputDir, result, execErr)
		return false, execErr
	}
	if e.DryRun {
		if e.OutputDir != "" {
			if err := copyDir(runDir, e.OutputDir); err != nil {
				return false, fmt.Errorf("failed to copy files to output directory: %w", err)
			}
			if err := writeMetaJSON(e.OutputDir, result, nil); err != nil {
				return false, err
			}
		}
		return true, printDryRunOutput(runDir)
	}
	if e.OutputDir != "" {
		if err := assertOutputDirEmpty(e.OutputDir, e.Force); err != nil {
			return false, err
		}
		if err := os.MkdirAll(e.OutputDir, 0o755); err != nil { //nolint:gosec // standard permissions
			return false, fmt.Errorf("could not create output directory: %s: %w", e.OutputDir, err)
		}
		if err := copyDir(runDir, e.OutputDir); err != nil {
			return false, fmt.Errorf("failed to copy files to output directory: %w", err)
		}
		return true, writeMetaJSON(e.OutputDir, result, nil)
	}
	return false, nil
}

// writeErrorMeta best-effort writes meta.json capturing a failed execution into
// outputDir. Failures are logged, never returned, so the original execution
// error remains the surfaced one.
func writeErrorMeta(outputDir string, result *execResult, execErr error) {
	if outputDir == "" {
		return
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil { //nolint:gosec // standard permissions
		slog.Warn("Failed to create output directory for meta.json", "dir", outputDir, "error", err)
		return
	}
	if err := writeMetaJSON(outputDir, result, execErr); err != nil {
		slog.Warn("Failed to write meta.json", "dir", outputDir, "error", err)
	}
}

// assertOutputDirEmpty returns an error unless outputDir is empty or missing.
// A non-empty directory is refused so the caller must opt in via Force. A
// ReadDir error (e.g. not-yet-existing dir) is treated as empty so MkdirAll
// can create it next.
func assertOutputDirEmpty(outputDir string, force bool) error {
	if force {
		return nil
	}
	entries, err := os.ReadDir(outputDir)
	if err == nil && len(entries) > 0 {
		return fmt.Errorf("output directory %s is not empty, use --force to overwrite", outputDir)
	}
	return nil
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
		return fmt.Errorf("failed to walk run directory: %w", err)
	}

	sort.Strings(files)

	for _, relPath := range files {
		absPath := filepath.Join(runDir, relPath)
		content, rdErr := os.ReadFile(absPath) //nolint:gosec // path constructed from known run directory
		if rdErr != nil {
			return fmt.Errorf("failed to read file: %s: %w", relPath, rdErr)
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
		return fmt.Errorf("failed to marshal meta.json: %w", err)
	}
	metaPath := filepath.Join(outputDir, "meta.json")
	if writeErr := os.WriteFile(metaPath, b, 0o644); writeErr != nil { //nolint:gosec // standard permissions for meta.json
		return fmt.Errorf("failed to write meta.json: %w", writeErr)
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
			return fmt.Errorf("failed to read: %s: %w", path, rdErr)
		}
		if mkErr := os.MkdirAll(filepath.Dir(targetPath), 0o755); mkErr != nil { //nolint:gosec // standard permissions
			return fmt.Errorf("failed to create dir: %s: %w", filepath.Dir(targetPath), mkErr)
		}
		if wrErr := os.WriteFile(targetPath, content, 0o644); wrErr != nil { //nolint:gosec // standard permissions
			return fmt.Errorf("failed to write: %s: %w", targetPath, wrErr)
		}
		return nil
	})
}

// ExecRPack loads and executes an rpack from the
// source file specified in `name`.
func (e *Executor) ExecRPack(ctx context.Context, name string) error {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return fmt.Errorf("could not load rpack config: %s: %w", name, err)
	}

	execPath := ci.ConfigPath
	if e.OverrideExecPath != "" {
		execPath = e.OverrideExecPath
	}
	pi, loadErr := LoadRPack(ci, execPath)
	if loadErr != nil {
		return fmt.Errorf("could not load rpack: %s: %w", name, loadErr)
	}

	values := pi.ConfigInstance.Config.Config.Values
	inputNames := lo.Keys(pi.ConfigInstance.Config.Config.Inputs)
	configValues := pi.ConfigInstance.Config.Config.Values

	fs, result, execErr := e.execCore(ctx, pi.SourcePath, pi.RunPath, pi.TempPath, pi.ResolvedInputs, values, inputNames, configValues)

	handled, err := e.finalizeOutput(result, pi.RunPath, execErr)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	// Normal mode: relocate produced files into execPath under lockfile control.
	filesToMove, checksums, err := computeFilesToMove(fs, pi.RunPath)
	if err != nil {
		return err
	}
	return e.applyLockfileChanges(ci.LockFile, execPath, ci.LockFilePath, filesToMove, checksums)
}

// ExecRPackDirect runs an rpack from a local definition directory
// with programmatically supplied values and inputs.
func (e *Executor) ExecRPackDirect(ctx context.Context, defDir string, values map[string]any, inputs map[string]string) error {
	absDefDir, err := filepath.Abs(defDir)
	if err != nil {
		return fmt.Errorf("could not resolve definition directory: %s: %w", defDir, err)
	}

	runDir, err := os.MkdirTemp("", "rpack-run-*")
	if err != nil {
		return fmt.Errorf("could not create run directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(runDir) }()

	tempDir, err := os.MkdirTemp("", "rpack-tmp-*")
	if err != nil {
		return fmt.Errorf("could not create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	resolvedInputs, err := resolveDirectInputs(inputs)
	if err != nil {
		return err
	}

	inputNames := lo.Keys(inputs)
	configValues := values

	var result *execResult
	var execErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("lua execution panicked: %v", r)
			}
		}()
		_, result, execErr = e.execCore(ctx, absDefDir, runDir, tempDir, resolvedInputs, values, inputNames, configValues)
	}()

	handled, err := e.finalizeOutput(result, runDir, execErr)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	// No --output-dir and no --dry-run: write files to CWD.
	if cpErr := copyDir(runDir, "."); cpErr != nil {
		return fmt.Errorf("failed to copy files to working directory: %w", cpErr)
	}

	return nil
}

// resolveDirectInputs resolves programmatic --def-mode inputs (name→path) to
// RPackResolvedInputs, accepting both relative (to CWD) and absolute paths and
// deriving each input's type from the on-disk stat.
func resolveDirectInputs(inputs map[string]string) ([]*RPackResolvedInput, error) {
	var resolvedInputs []*RPackResolvedInput
	for name, userPath := range inputs {
		cleanPath := filepath.Clean(userPath)
		absPath := cleanPath
		if !filepath.IsAbs(cleanPath) {
			cwd, wdErr := os.Getwd()
			if wdErr != nil {
				return nil, fmt.Errorf("could not get working directory: %w", wdErr)
			}
			absPath = filepath.Join(cwd, cleanPath)
		}
		isDir, statErr := util.CheckFileOrDirExists(absPath)
		if statErr != nil {
			return nil, fmt.Errorf("user path %s=%s does not exist: %w", name, userPath, statErr)
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
	return resolvedInputs, nil
}

// computeFilesToMove enumerates the target write handles of fs, mapping each
// unique produced file to a ControlledFile paired with its on-disk sha256.
// A handle written multiple times by the script is counted once; this is the
// input to the lockfile-based relocation in ExecRPack's terminal path.
func computeFilesToMove(fs *RPackFS, runDir string) ([]*ControlledFile, map[string]string, error) {
	visited := make(map[string]struct{})
	var filesToMove []*ControlledFile
	checksums := make(map[string]string)
	for _, handle := range fs.TargetWriteHandles() {
		relPath := handle.IndirectTargetPath()
		absPath := filepath.Clean(filepath.Join(runDir, relPath))
		if _, ok := visited[absPath]; ok {
			slog.Debug("File was already moved, but written multiple times, skipping", "path", handle.FriendlyPath())
			continue
		}
		chsum, err := util.Sha256File(absPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to calculate checksum of: %s: %w", absPath, err)
		}
		checksums[absPath] = chsum
		filesToMove = append(filesToMove, &ControlledFile{
			Path:    relPath,
			AbsPath: absPath,
		})
		visited[absPath] = struct{}{}
	}
	return filesToMove, checksums, nil
}

// applyLockfileChanges reconciles the lockfile-protected target directory with
// the files produced by this run: it refuses (unless Force) when a previously
// managed file was modified out-of-band, overwrites newly-added files (refusing
// unless Force), moves produced files into place, removes files no longer
// managed, and writes the new lockfile. It is the lockfile half of ExecRPack's
// terminal path.
func (e *Executor) applyLockfileChanges(oldLock *RPackLockFile, execPath, lockFilePath string, filesToMove []*ControlledFile, checksums map[string]string) error {
	if err := e.guardLockfileIntegrity(oldLock, execPath); err != nil {
		return err
	}

	newLockfile := buildNewLockfile(filesToMove, checksums)
	changes := newLockfile.Changes(oldLock)
	slog.Info("New files in lockfile", "files", changes.Added)
	slog.Info("Files no longer maintained by rpack, removing", "files", changes.Removed)

	if err := e.guardAddedFiles(execPath, changes.Added); err != nil {
		return err
	}
	if err := moveFiles(filesToMove, execPath); err != nil {
		return err
	}
	if err := cleanupRemovedFiles(execPath, changes.Removed); err != nil {
		return err
	}
	if err := newLockfile.WriteFile(lockFilePath); err != nil {
		return fmt.Errorf("could not write lockfile to %s: %w", lockFilePath, err)
	}
	return nil
}

// guardLockfileIntegrity checks the existing lockfile against files on disk
// and refuses (unless Force) when managed files were modified out-of-band.
// Manually-removed managed files are only logged; their cleanup is driven by
// the lockfile diff later in applyLockfileChanges.
func (e *Executor) guardLockfileIntegrity(oldLock *RPackLockFile, execPath string) error {
	integrity, err := oldLock.CheckIntegrity(execPath)
	if err != nil {
		return fmt.Errorf("failed to check lockfile integrity: %w", err)
	}
	if len(integrity.Modified) > 0 {
		modFilesStr := strings.Join(integrity.Modified, ",")
		slog.Warn("Some files in lockfile were modified outside of rpack", "files", modFilesStr)
		if !e.Force {
			return fmt.Errorf("some locked files were modified outside of rpack, use force flag to ignore: %s", modFilesStr)
		}
	}
	if len(integrity.Removed) > 0 {
		slog.Warn("Some files in lockfile were removed outside of rpack", "files", strings.Join(integrity.Removed, ","))
	}
	return nil
}

// guardAddedFiles refuses to overwrite files that exist on disk but were not
// managed by the previous lockfile, unless Force is set. Such files would be
// clobbered by the relocation of a newly-produced rpack output.
func (e *Executor) guardAddedFiles(execPath string, added []string) error {
	for _, path := range added {
		targetFile := filepath.Clean(filepath.Join(execPath, path))
		exists, err := util.FileExists(targetFile)
		if exists {
			slog.Warn("File is not managed by rpack but will be overwritten", "file", path)
			if !e.Force {
				return fmt.Errorf("existing file would need to be overwritten, use force flag to ignore: %s", path)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check file exists: %s: %w", path, err)
		}
	}
	return nil
}

// buildNewLockfile constructs the lockfile recording the files produced by this
// run. Panics if a produced file has no checksum: computeFilesToMove always
// pairs them, so this is an invariant guard against a future caller bug.
func buildNewLockfile(filesToMove []*ControlledFile, checksums map[string]string) *RPackLockFile {
	lockfile := NewRPackLockFile()
	for _, wFile := range filesToMove {
		chsum, ok := checksums[wFile.AbsPath]
		if !ok {
			panic("Can't find checksum for file")
		}
		lockfile.AddFile(wFile.Path, chsum)
	}
	return lockfile
}

// moveFiles relocates each produced file from its run-directory absolute path
// into execPath at its relative target path, creating parent directories.
func moveFiles(filesToMove []*ControlledFile, execPath string) error {
	for _, wFile := range filesToMove {
		targetFile := filepath.Clean(filepath.Join(execPath, wFile.Path))
		if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil { //nolint:gosec // standard permissions
			return fmt.Errorf("failed to create dirs for: %s: %w", targetFile, err)
		}
		if err := os.Rename(wFile.AbsPath, targetFile); err != nil {
			return fmt.Errorf("failed to move file %s to exec path %s: %w", wFile.Path, execPath, err)
		}
	}
	return nil
}

// cleanupRemovedFiles deletes files that the previous lockfile managed but
// this run no longer produces. Already-missing files are logged and skipped.
func cleanupRemovedFiles(execPath string, removed []string) error {
	for _, removedFile := range removed {
		p := filepath.Join(execPath, removedFile)
		exists, err := util.FileExists(p)
		if err != nil {
			return fmt.Errorf("could not check deprecated file: %s: %w", removedFile, err)
		}
		if exists {
			if err = os.Remove(p); err != nil {
				return fmt.Errorf("could not remove deprecated file: %s: %w", removedFile, err)
			}
		} else {
			slog.Warn("File managed by rpack but marked for removal, does no longer exist, ignoring", "file", removedFile)
		}
	}
	return nil
}
