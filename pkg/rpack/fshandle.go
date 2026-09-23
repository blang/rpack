package rpack

import (
	"log/slog"
	"os"
	"path/filepath"

	"fmt"
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
	// Chmod sets executable intent, not the physical staging permissions.
	// The handle must point to an existing regular file; BaseFS.Chmod enforces
	// the access and existence restrictions before calling.
	Chmod(mode os.FileMode) error
	// OutputMode is canonical executable intent, independent of staging stat.
	OutputMode() os.FileMode
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
	outputMode         os.FileMode
}

// NewFileBackedFSHandle creates a new file-backed filesystem handle.
func NewFileBackedFSHandle(absPath, friendlyPath, resolver, indirectTargetPath string) *FileBackedFSHandle {
	slog.Debug("New FileBackedFSHandle", "absPath", absPath, "friendlyPath", friendlyPath, "resolver", resolver, "indirectTargetPath", indirectTargetPath)
	return &FileBackedFSHandle{
		absPath:            absPath,
		friendlyPath:       friendlyPath,
		resolver:           resolver,
		indirectTargetPath: indirectTargetPath,
		outputMode:         NonExecutableMode,
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
		return nil, fmt.Errorf("could not read %s: %w", f.friendlyPath, err)
	}
	return content, nil
}

func (f *FileBackedFSHandle) Write(b []byte) error {
	if err := os.MkdirAll(filepath.Dir(f.absPath), 0o700); err != nil {
		return fmt.Errorf("could not write %s: %w", f.friendlyPath, err)
	}
	// Staging is private even when the final output is executable or public.
	// Recreate rather than truncating, so an old inode's permissions never leak.
	if err := writeCreatedFile(f.absPath, b, 0o600, NonExecutableMode); err != nil {
		return fmt.Errorf("could not write %s: %w", f.friendlyPath, err)
	}
	f.outputMode = NonExecutableMode
	return nil
}

// Chmod amends metadata only. Final creation applies the caller's umask to the
// appropriate Git-style base mode; staging never needs to become executable.
func (f *FileBackedFSHandle) Chmod(mode os.FileMode) error {
	info, err := os.Stat(f.absPath)
	if err != nil {
		return fmt.Errorf("could not chmod %s: %w", f.friendlyPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cannot chmod %s: not a regular file", f.friendlyPath)
	}
	f.outputMode = canonicalOutputMode(mode)
	return nil
}

// OutputMode returns the last successful write/chmod's canonical intent.
func (f *FileBackedFSHandle) OutputMode() os.FileMode {
	return f.outputMode
}

// Stat returns file existence and directory status.
func (f *FileBackedFSHandle) Stat() (_exists, _dir bool, _err error) {
	fileInfo, err := os.Stat(f.absPath)
	if os.IsNotExist(err) {
		return false, false, nil
	} else if err != nil {
		return false, false, fmt.Errorf("error accessing file: %s: %w", f.friendlyPath, err)
	}

	return true, fileInfo.IsDir(), nil
}

// ReadDir returns directory entries.
func (f *FileBackedFSHandle) ReadDir() (_files, _dirs []FSHandle, _err error) {
	entries, err := os.ReadDir(f.absPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error readdir: %s: %w", f.friendlyPath, err)
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

// Transfer materializes the declared output and removes its staged copy.
func (f *FileBackedFSHandle) Transfer(dest string) error {
	content, err := f.Read()
	if err != nil {
		return err
	}
	err = writeOutputFile(dest, content, f.OutputMode())
	if err != nil {
		return fmt.Errorf("failed to transfer %s to %s: %w", f.friendlyPath, dest, err)
	}
	return os.Remove(f.absPath)
}
