// Package getsource provides a curated wrapper around the go-getter library
// for downloading RPack source packages, with explicit control over which
// detectors, getters, and decompressors are supported.
package getsource

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	getter "github.com/hashicorp/go-getter"
)

// Detectors is the curated list of source address detectors.
// We explicitly list only the detectors RPack officially supports,
// insulating us from detectors that go-getter might add in future.
var Detectors = []getter.Detector{
	&withoutQueryParams{d: new(getter.GitHubDetector)},
	new(getter.GitDetector),
	new(getter.BitBucketDetector),
	new(getter.GCSDetector),
	new(getter.S3Detector),
	new(fileDetector),
}

// fileDetector is a replacement for go-getter's FileDetector that rejects
// relative filesystem paths without ./ or ../ prefix with a clear error
// type that callers can inspect.
type fileDetector struct{}

func (d *fileDetector) Detect(src, pwd string) (result string, ok bool, err error) {
	if src == "" {
		return "", false, nil
	}

	if !filepath.IsAbs(src) {
		// Allow ./ and ../ prefixes by resolving against pwd.
		if strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") {
			resolved, absErr := filepath.Abs(filepath.Join(pwd, src))
			if absErr != nil {
				return "", true, &MaybeRelativePathError{Addr: src}
			}
			return fmtFileURL(resolved), true, nil
		}
		// Reject bare relative paths with a clear error.
		return "", true, &MaybeRelativePathError{Addr: src}
	}

	return fmtFileURL(src), true, nil
}

func fmtFileURL(path string) string {
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
		return fmt.Sprintf("file://%s", path)
	}
	if path[0] == '/' {
		path = path[1:]
	}
	return fmt.Sprintf("file:///%s", path)
}

// MaybeRelativePathError is returned when a source address looks like a relative
// filesystem path without the required "./" or "../" prefix.
type MaybeRelativePathError struct {
	Addr string
}

// Error implements the error interface.
func (e *MaybeRelativePathError) Error() string {
	return fmt.Sprintf("RPack cannot detect a supported source type for %q — if this is a local path, use a ./ or ../ prefix", e.Addr)
}

// withoutQueryParams wraps a detector to strip query parameters before
// detection and reattach them afterward. This allows GitHub URLs with
// query params like ?ref=main to be detected correctly.
type withoutQueryParams struct {
	d getter.Detector
}

func (w *withoutQueryParams) Detect(src, pwd string) (result string, ok bool, err error) {
	var qp string
	if idx := strings.Index(src, "?"); idx > -1 {
		qp = src[idx+1:]
		src = src[:idx]
	}

	result, ok, err = w.d.Detect(src, pwd)
	if result != "" && qp != "" {
		result += "?" + qp
	}
	return result, ok, err
}
