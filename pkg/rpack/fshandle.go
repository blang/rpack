package rpack

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// FSHandle is returned by resolver and represents a file handle with a friendly name such as
// prefix:path.
// All file operations are abstracted in this interface to hide any real filesystem operations.
type FSHandle interface {
	// Resolver returns the resolver responsible for the Handle.
	Resolver() string
	// FriendlyPath returns the path specified by the user
	// such as map:my-list
	FriendlyPath() string // Path including prefix
	// IndirectTargetPath returns the indirect path to the target if it exists, otherwise ""
	IndirectTargetPath() string
	Read() ([]byte, error)
	Write([]byte) error
	Stat() (exists bool, dir bool, err error)
	ReadDir() (files []FSHandle, dirs []FSHandle, err error)
	Transfer(absPath string) error // Transfers a file to a target file location - used for later on relocating
}

// Ensure FileBackedFSHandle implements FSHandle
var _ = FSHandle(&FileBackedFSHandle{})

// FileBackedFSHandle represents a file handle backed by a real filesystem.
type FileBackedFSHandle struct {
	absPath      string
	friendlyPath string
	resolver     string
	// Contains the indirect path to the target (repo) if exists
	indirectTargetPath string
}

// NewFileBackedFSHandle creates a new file-backed filesystem handle.
func NewFileBackedFSHandle(absPath, friendlyPath, resolver, indirectTargetPath string) *FileBackedFSHandle {
	slog.Debug("New FileBackedFSHandle", "absPath", absPath, "friendlyPath", friendlyPath, "resolver", resolver, "indirectTargetPath", indirectTargetPath)
	return &FileBackedFSHandle{
		absPath:            absPath,
		friendlyPath:       friendlyPath,
		resolver:           resolver,
		indirectTargetPath: indirectTargetPath,
	}
}

// Resolver returns the resolver name.
func (f *FileBackedFSHandle) Resolver() string {
	return f.resolver
}

// FriendlyPath returns the human-readable path.
func (f *FileBackedFSHandle) FriendlyPath() string {
	return f.friendlyPath
}

func (f *FileBackedFSHandle) Read() ([]byte, error) {
	content, err := os.ReadFile(f.absPath)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not read %s", f.friendlyPath)
	}
	return content, nil
}

func (f *FileBackedFSHandle) Write(b []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.absPath), 0o755); err != nil { //nolint:gosec // intentional: standard directory permissions
		return errors.Wrapf(err, "Could not write %s", f.friendlyPath)
	}
	if err := os.WriteFile(f.absPath, b, 0o644); err != nil { //nolint:gosec // intentional: standard file permissions for package manager output
		return errors.Wrapf(err, "Could not write %s", f.friendlyPath)
	}
	return nil
}

// Stat returns file existence and directory status.
func (f *FileBackedFSHandle) Stat() (_dir, _exists bool, _err error) {
	fileInfo, err := os.Stat(f.absPath)
	if os.IsNotExist(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Wrapf(err, "Error accessing file: %s", f.friendlyPath)
	}

	return fileInfo.IsDir(), true, nil
}

// ReadDir returns directory entries.
func (f *FileBackedFSHandle) ReadDir() (_files, _dirs []FSHandle, _err error) {
	entries, err := os.ReadDir(f.absPath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "Error readdir: %s", f.friendlyPath)
	}
	var files []FSHandle
	var dirs []FSHandle
	for _, e := range entries {
		absPath := filepath.Join(f.absPath, e.Name())
		slog.Debug("Friendly path of parent for readdir", "friendlyPath", f.friendlyPath)
		friendlyPath := filepath.Join(f.friendlyPath, e.Name())
		indirectTargetPath := filepath.Join(f.indirectTargetPath, e.Name())
		newHandle := NewFileBackedFSHandle(absPath, friendlyPath, f.resolver, indirectTargetPath)
		if e.IsDir() {
			dirs = append(dirs, newHandle)
		} else {
			files = append(files, newHandle)
		}
	}
	return files, dirs, nil
}

// IndirectTargetPath returns the indirect target path for renaming.
func (f *FileBackedFSHandle) IndirectTargetPath() string {
	return f.indirectTargetPath
}

// Transfer copies the file to the target path.
// TODO: Might not be used since we implement renaming through IndirectTargetPath
func (f *FileBackedFSHandle) Transfer(dest string) error {
	err := os.Rename(f.absPath, dest)
	if err != nil {
		return errors.Wrapf(err, "Failed to transfer %s to %s", f.friendlyPath, dest)
	}
	return nil
}
