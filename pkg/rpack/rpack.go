package rpack

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"

	"fmt"

	"github.com/samber/lo"

	"github.com/blang/rpack/pkg/rpack/util"
)

// RPackConfig is the configuration to use a rpack file
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackConfig struct {
	Config        *RPackConfigConfig `json:"config"`
	SchemaVersion string             `json:"@schema_version"`
	Source        string             `json:"source"`
}

// RPackConfigConfig bundles Values and Input declaration
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackConfigConfig struct {
	// Inputs defines dirs and files the rpack is allowed to read
	// This should match the definitions inputs.
	Inputs map[string]string `json:"inputs"`

	// Values represents the values for the config defined
	Values map[string]any `json:"values"`
}

// Validate checks the configuration for errors.
func (c *RPackConfig) Validate() error {
	err := RPackSchemaValidator.Validate(c)
	if err != nil {
		return fmt.Errorf("validating rpack against schema failed: %w", err)
	}
	return nil
}

// RPackSchema holds the CUE schema for rpack configuration validation.
//
//go:embed schema.cue
var RPackSchema string

// CUE schema internal names.
const (
	RPackInternalSchemaName = "#Schema"
)

// RPackSchemaValidator is the precompiled CUE schema validator.
var RPackSchemaValidator = lo.Must(NewCueValidator([]byte(RPackSchema), RPackInternalSchemaName))

// RPackConfigInstance is the internal representation of a RPackConfig.
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackConfigInstance struct {

	// Path of the config
	ConfigPath string

	// The RPackConfig
	Config *RPackConfig

	// Lockfile loaded if exists
	LockFile *RPackLockFile

	// Lockfile Path
	LockFilePath string
}

// Current schema versions for config and lockfile.
const (
	RPackConfigCurrentSchemaVersion   = "v1"
	RPackLockFileCurrentSchemaVersion = "v1"
)

// RPackLockFile keeps track of the files written by a RPackInstance to remove files not written between executions
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackLockFile struct {
	SchemaVersion string               `json:"@schema_version"`
	Files         []*RPackLockFileFile `json:"files"`
}

// NewRPackLockFile creates a new empty RPackLockFile with the latest schema version set.
func NewRPackLockFile() *RPackLockFile {
	return &RPackLockFile{
		SchemaVersion: RPackLockFileCurrentSchemaVersion,
		Files:         []*RPackLockFileFile{},
	}
}

// Validate checks the lock file for errors.
func (f *RPackLockFile) Validate() error {
	if f.SchemaVersion != RPackLockFileCurrentSchemaVersion {
		return fmt.Errorf("unsupported lockfile schema version %q, supported %q", f.SchemaVersion, RPackLockFileCurrentSchemaVersion)
	}
	return nil
}

// RPackLockFileFile is a single lock file state
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackLockFileFile struct {
	// Path relative to lockfile directory marking the filename
	Path string `json:"path"`
	// Sha of the path, so we can check if we will remove a modified file
	Sha string `json:"sha"`
	// Mode records canonical executable intent: "644" for non-executable,
	// "755" for executable (ADR 0002), not the file's full rwx permissions.
	// A string type keeps the YAML output quoted, dodging the YAML
	// octal-integer trap. Empty means unknown: the entry was written by a
	// pre-feature rpack and the mode check is skipped for it.
	Mode string `json:"mode"`
}

// AddFile adds a file entry to the lock file.
func (f *RPackLockFile) AddFile(path, sha, mode string) {
	f.Files = append(f.Files, &RPackLockFileFile{
		Path: path,
		Sha:  sha,
		Mode: mode,
	})
}

// RPackLockFileIntegrity represents integrity check results for a lock file.
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackLockFileIntegrity struct {
	Modified []string
	// ModeModified holds entries formatted with expected/found executable
	// intent ("deploy.sh (expected executable (755), found non-executable
	// (644))") so callers can surface directly why a content-identical file
	// was flagged. Diagnostics describe executable intent, never literal
	// rwx bits (ADR 0002).
	ModeModified []string
	Removed      []string
}

// CheckIntegrity checks if managed files are still valid
func (f *RPackLockFile) CheckIntegrity(path string) (*RPackLockFileIntegrity, error) {
	res := &RPackLockFileIntegrity{}
	cleanBase := filepath.Clean(path)
	for _, file := range f.Files {
		// Recorded modes must be canonical executable-intent records
		// ("644"/"755") or empty (unknown, pre-feature entry). Legacy
		// non-canonical records are pre-migration lockfile state and
		// hard-error with a migration hint: this is deliberately not
		// drift-classified, so --force cannot silently rewrite a recorded
		// permission guarantee (issue #15). Invalid values are rejected
		// with a clear message; YAML decoding itself never fails on them.
		if err := checkRecordedMode(file.Mode); err != nil {
			return nil, fmt.Errorf("lockfile entry %s: %w", file.Path, err)
		}
		filePath := filepath.Join(cleanBase, file.Path)
		if err := util.CheckFileExists(filePath); err != nil {
			res.Removed = append(res.Removed, file.Path)
			continue
		}
		chsum, err := util.Sha256File(filePath)
		if err != nil {
			// An unreadable managed file (e.g. chmod 000 out-of-band) is
			// classified as drift rather than a hard error, so a --force run
			// can always heal it (ADR 0001).
			slog.Warn("Could not checksum managed file, treating as modified", "file", file.Path, "error", err)
			res.Modified = append(res.Modified, file.Path)
		} else if file.Sha != chsum {
			res.Modified = append(res.Modified, file.Path)
		}
		if file.Mode == "" {
			// Pre-feature lockfile entry: the recorded mode is unknown, so
			// there is nothing honest to check against. The next successful
			// run rewrites the lockfile with modes recorded (ADR 0001).
			slog.Warn("Lockfile entry has no recorded mode, skipping mode check", "file", file.Path)
			continue
		}
		info, statErr := os.Stat(filePath)
		if statErr != nil {
			return nil, fmt.Errorf("could not stat %s: %s: %w", file.Path, filePath, statErr)
		}
		// Compare executable intent, not literal rwx bits: read/write
		// differences from local policy (umask, ACLs) are not drift, a
		// changed owner-execute bit is (issue #15).
		if onDisk := CanonicalMode(info.Mode()); onDisk != file.Mode {
			res.ModeModified = append(res.ModeModified, modeDriftMessage(file.Path, file.Mode, onDisk))
		}
	}
	return res, nil
}

// RPackLockFileChanges represents changes detected in a lock file.
//
//nolint:revive // intentional: RPack prefix is the domain convention
type RPackLockFileChanges struct {
	// File added in comparison
	Added []string

	// File removed in comparison
	Removed []string
}

// Changes records the changes from the existing (new) lockfile to the old lockfile
func (f *RPackLockFile) Changes(old *RPackLockFile) *RPackLockFileChanges {
	changes := &RPackLockFileChanges{}
	newFiles := make(map[string]struct{})
	oldFiles := make(map[string]struct{})
	for _, newFile := range f.Files {
		newFiles[newFile.Path] = struct{}{}
	}
	for _, oldFile := range old.Files {
		oldFiles[oldFile.Path] = struct{}{}
		if _, ok := newFiles[oldFile.Path]; !ok {
			changes.Removed = append(changes.Removed, oldFile.Path)
		}
	}
	for _, newFile := range f.Files {
		if _, ok := oldFiles[newFile.Path]; !ok {
			changes.Added = append(changes.Added, newFile.Path)
		}
	}
	return changes
}
