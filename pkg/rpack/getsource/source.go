package getsource

import (
	"path/filepath"

	getter "github.com/hashicorp/go-getter"
)

// NormalizeSource detects and normalizes a source address using the
// curated detector set defined in Detectors.
func NormalizeSource(src string) (string, error) {
	result, err := getter.Detect(src, "", Detectors)
	if err != nil {
		return "", err
	}
	return result, nil
}

// SplitSourceSubdir splits a normalized source address into the
// package address and an optional subdirectory portion.
// Handles go-getter's //subdir syntax.
func SplitSourceSubdir(result string) (packageAddr, subDir string) {
	packageAddr, subDir = getter.SourceDirSubdir(result)
	if subDir != "" {
		subDir = filepath.Clean(subDir)
	}
	return packageAddr, subDir
}
