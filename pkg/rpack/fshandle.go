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

type FileBackedFSHandle struct {
	absPath      string
	friendlyPath string
	resolver     string
	// Contains the indirect path to the target (repo) if exists
	indirectTargetPath string
}

func NewFileBackedFSHandle(absPath string, friendlyPath string, resolver string, indirectTargetPath string) *FileBackedFSHandle {
	slog.Debug("New FileBackedFSHandle", "absPath", absPath, "friendlyPath", friendlyPath, "resolver", resolver, "indirectTargetPath", indirectTargetPath)
	return &FileBackedFSHandle{
		absPath:            absPath,
		friendlyPath:       friendlyPath,
		resolver:           resolver,
		indirectTargetPath: indirectTargetPath,
	}
}

func (f *FileBackedFSHandle) Resolver() string {
	return f.resolver
}

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
	if err := os.MkdirAll(filepath.Dir(f.absPath), 0755); err != nil {
		return errors.Wrapf(err, "Could not write %s", f.friendlyPath)
	}
	if err := os.WriteFile(f.absPath, b, 0644); err != nil {
		return errors.Wrapf(err, "Could not write %s", f.friendlyPath)
	}
	return nil
}

func (f *FileBackedFSHandle) Stat() (_dir bool, _exists bool, _err error) {
	fileInfo, err := os.Stat(f.absPath)
	if os.IsNotExist(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, errors.Wrapf(err, "Error accessing file: %s", f.friendlyPath)
	}

	return fileInfo.IsDir(), true, nil
}

func (f *FileBackedFSHandle) ReadDir() (_files []FSHandle, _dirs []FSHandle, _err error) {
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

func (f *FileBackedFSHandle) IndirectTargetPath() string {
	return f.indirectTargetPath
}

// TODO: Might not be used since we implement renaming through IndirectTargetPath
func (f *FileBackedFSHandle) Transfer(dest string) error {
	err := os.Rename(f.absPath, dest)
	if err != nil {
		return errors.Wrapf(err, "Failed to transfer %s to %s", f.friendlyPath, dest)
	}
	return nil
}
