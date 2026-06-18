package rpack

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"errors"
	"fmt"

	"sigs.k8s.io/yaml"
)

// RPack file extensions and suffixes.
const (
	RPackFileSuffix     = ".rpack.yaml"
	RPackLockFileSuffix = ".rpack.lock.yaml"
)

// LoadRPackConfig creates a RPackConfigInstance by loading the RPackConfig and RPackLockFile from a file.
// It does not perform validation of user supplied config, but validate the whole file against a schema.
func LoadRPackConfig(name string) (*RPackConfigInstance, error) {
	absPath, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("could not construct absolute path for file %s: %w", name, err)
	}

	configFileName := filepath.Base(absPath)

	// Check format of filename
	if !strings.HasSuffix(configFileName, RPackFileSuffix) {
		return nil, fmt.Errorf("rPack filename does not ends in %s: %s", RPackFileSuffix, configFileName)
	}

	configPath := filepath.Dir(absPath)

	// Load RPackConfig from file
	config, err := loadRPackFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("could not load rpack file: %s: %w", absPath, err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validating rpack file against schema: %s: %w", absPath, err)
	}

	// Load LockFile from file
	lockFileName, trimmed := strings.CutSuffix(configFileName, RPackFileSuffix)
	if !trimmed {
		return nil, fmt.Errorf("rPack filename does not ends in %s: %s", RPackFileSuffix, configFileName)
	}
	lockFileName += RPackLockFileSuffix
	lockFilePath := filepath.Join(configPath, lockFileName)

	var lockFile *RPackLockFile
	if _, err := os.Stat(lockFilePath); errors.Is(err, os.ErrNotExist) {
		slog.Info("Lockfile does not exist", "path", lockFilePath)
		lockFile = NewRPackLockFile()
	} else {
		lockFile, err = loadRPackLockFile(lockFilePath)
		if err != nil {
			return nil, fmt.Errorf("could not load lockfile %s: %w", lockFilePath, err)
		}
	}
	if err := lockFile.Validate(); err != nil {
		return nil, fmt.Errorf("lockfile validation failed: %s: %w", lockFilePath, err)
	}

	return &RPackConfigInstance{
		ConfigPath:   configPath,
		Config:       config,
		LockFile:     lockFile,
		LockFilePath: lockFilePath,
	}, nil
}

func loadRPackFile(name string) (*RPackConfig, error) {
	b, err := os.ReadFile(name) //nolint:gosec // intentional: path comes from user config
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %s: %w", name, err)
	}
	var c RPackConfig
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml in file: %s: %w", name, err)
	}
	return &c, nil
}

func loadRPackLockFile(name string) (*RPackLockFile, error) {
	b, err := os.ReadFile(name) //nolint:gosec // intentional: path comes from user config
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %s: %w", name, err)
	}
	var c RPackLockFile
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml in file: %s: %w", name, err)
	}
	return &c, nil
}

// WriteFile writes the lock file content to the given path.
func (l *RPackLockFile) WriteFile(name string) error {
	b, err := yaml.Marshal(l)
	if err != nil {
		return fmt.Errorf("failed to marshal lockfile: %w", err)
	}
	err = os.WriteFile(name, b, 0o666) //nolint:gosec // intentional: standard file permissions for package manager output
	if err != nil {
		return fmt.Errorf("failed to write lockfile: %w", err)
	}
	return nil
}
