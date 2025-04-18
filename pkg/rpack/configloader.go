package rpack

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

const (
	RPackFileSuffix     = ".rpack.yaml"
	RPackLockFileSuffix = ".rpack.lock.yaml"
)

// LoadRPackConfig creates a RPackConfigInstance by loading the RPackConfig and RPackLockFile from a file.
// It does not perform validation of user supplied config, but validate the whole file against a schema.
func LoadRPackConfig(name string) (*RPackConfigInstance, error) {
	absPath, err := filepath.Abs(name)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not construct absolute path for file %s", name)
	}

	configFileName := filepath.Base(absPath)

	// Check format of filename
	if !strings.HasSuffix(configFileName, RPackFileSuffix) {
		return nil, errors.Errorf("RPack filename does not ends in %s: %s", RPackFileSuffix, configFileName)
	}

	configPath := filepath.Dir(absPath)

	// Load RPackConfig from file
	config, err := loadRPackFile(absPath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not load rpack file: %s", absPath)
	}

	if err := config.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Validating rpack file against schema: %s", absPath)
	}

	// Load LockFile from file
	lockFileName, trimmed := strings.CutSuffix(configFileName, RPackFileSuffix)
	if !trimmed {
		return nil, errors.Errorf("RPack filename does not ends in %s: %s", RPackFileSuffix, configFileName)
	}
	lockFileName = lockFileName + RPackLockFileSuffix
	lockFilePath := filepath.Join(configPath, lockFileName)

	var lockFile *RPackLockFile
	if _, err := os.Stat(lockFilePath); errors.Is(err, os.ErrNotExist) {
		slog.Info("Lockfile does not exist", "path", lockFilePath)
		lockFile = NewRPackLockFile()
	} else {
		lockFile, err = loadRPackLockFile(lockFilePath)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not load lockfile %s", lockFilePath)
		}
	}
	if err := lockFile.Validate(); err != nil {
		return nil, errors.Wrapf(err, "Lockfile validation failed: %s", lockFilePath)
	}

	return &RPackConfigInstance{
		ConfigPath:   configPath,
		Config:       config,
		LockFile:     lockFile,
		LockFilePath: lockFilePath,
	}, nil
}

func loadRPackFile(name string) (*RPackConfig, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open file: %s", name)
	}
	var c RPackConfig
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to unmarshal yaml in file: %s", name)
	}
	return &c, nil
}

func loadRPackLockFile(name string) (*RPackLockFile, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open file: %s", name)
	}
	var c RPackLockFile
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to unmarshal yaml in file: %s", name)
	}
	return &c, nil
}

func (l *RPackLockFile) WriteFile(name string) error {
	b, err := yaml.Marshal(l)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal lockfile")
	}
	err = os.WriteFile(name, b, 0666)
	if err != nil {
		return errors.Wrap(err, "Failed to write lockfile")
	}
	return nil
}
