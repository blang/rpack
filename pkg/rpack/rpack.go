package rpack

import (
	_ "embed"
	"path/filepath"

	"github.com/blang/rpack/pkg/rpack/util"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// RPackConfig is the configuration to use a rpack file
type RPackConfig struct {
	SchemaVersion string `json:"@schema_version"`

	// Follows https://github.com/hashicorp/go-getter syntax.
	Source string `json:"source"`

	// Bundles values and inputs
	Config *RPackConfigConfig `json:"config"`
}

// RPackConfigConfig bundles Values and Input declaration
type RPackConfigConfig struct {
	// Inputs defines dirs and files the rpack is allowed to read
	// This should match the definitions inputs.
	Inputs map[string]string `json:"inputs"`

	// Values represents the values for the config defined
	Values map[string]interface{} `json:"values"`
}

func (c *RPackConfig) Validate() error {
	err := RPackSchemaValidator.Validate(c)
	if err != nil {
		return errors.Wrap(err, "Validating rpack against schema failed")
	}
	return nil
}

//go:embed schema.cue
var RPackSchema string

const (
	RPackInternalSchemaName = "#Schema"
)

var RPackSchemaValidator = lo.Must(NewCueValidator([]byte(RPackSchema), RPackInternalSchemaName))

// RPack is the internal representation of a RPackConfig
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

const (
	RPackConfigCurrentSchemaVersion   = "v1"
	RPackLockFileCurrentSchemaVersion = "v1"
)

// RPackLockFile keeps track of the files written by a RPackInstance to remove files not written between executions
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

func (f *RPackLockFile) Validate() error {
	if f.SchemaVersion != RPackLockFileCurrentSchemaVersion {
		return errors.Errorf("Unsupported lockfile schema version %q, supported %q", f.SchemaVersion, RPackLockFileCurrentSchemaVersion)
	}
	return nil
}

// RPackLockFileFile is a single lock file state
type RPackLockFileFile struct {
	// Path relative to lockfile directory marking the filename
	Path string `json:"path"`
	// Sha of the path, so we can check if we will remove a modified file
	Sha string `json:"sha"`
}

func (f *RPackLockFile) AddFile(path string, sha string) {
	f.Files = append(f.Files, &RPackLockFileFile{
		Path: path,
		Sha:  sha,
	})
}

type RPackLockFileIntegrity struct {
	Modified []string
	Removed  []string
}

// CheckIntegrity checks if managed files are still valid
func (f *RPackLockFile) CheckIntegrity(path string) (*RPackLockFileIntegrity, error) {
	res := &RPackLockFileIntegrity{}
	cleanBase := filepath.Clean(path)
	for _, file := range f.Files {
		filePath := filepath.Join(cleanBase, file.Path)
		if err := util.CheckFileExists(filePath); err != nil {
			res.Removed = append(res.Removed, file.Path)
			continue
		}
		chsum, err := util.Sha256File(filePath)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not calculate checksum for %s: %s", file.Path, filePath)
		}
		if file.Sha != chsum {
			res.Modified = append(res.Modified, file.Path)
		}
	}
	return res, nil
}

type RPackLockFileChanges struct {
	// File added in comparison
	Added []string

	// File removed in comparision
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
