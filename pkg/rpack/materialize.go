package rpack

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

func validateOutputMode(mode os.FileMode) error {
	if mode != NonExecutableMode && mode != ExecutableMode {
		return fmt.Errorf("unsupported output mode %o: only 644 (non-executable) and 755 (executable) are supported", mode)
	}
	return nil
}

// writeOutputFile creates a fresh inode in the destination directory. Like Git,
// it requests 0666/0777 and lets the OS apply umask and inherited default ACLs.
// Neither an existing destination's mode nor private staging permissions leak
// into the output. Rename publishes the fully written, validated file atomically.
func writeOutputFile(path string, content []byte, mode os.FileMode) error {
	if err := validateOutputMode(mode); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil { //nolint:gosec // creation permissions are restricted by the caller's umask
		return fmt.Errorf("could not create output directory: %w", err)
	}
	perm := os.FileMode(0o666)
	if mode == ExecutableMode {
		perm = 0o777
	}
	return writeCreatedFile(path, content, perm, mode)
}

// writeCreatedFile is also used for private (0600) staging. CreateTemp cannot
// create with a caller-selected mode, and chmod afterwards would bypass umask,
// so use an unpredictable, exclusively created sibling instead. No process-wide
// umask probe or chmod is needed, including when replacing an existing file.
func writeCreatedFile(path string, content []byte, perm, intent os.FileMode) error {
	tmp := filepath.Join(filepath.Dir(path), ".rpack-"+rand.Text())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) //nolint:gosec // random exclusive temporary sibling; OS applies creation policy
	if err != nil {
		return fmt.Errorf("could not create output for %s: %w", path, err)
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmp) //nolint:gosec // tmp is an internal random sibling name; the destination dir is a validated output path
	}()
	if _, wErr := f.Write(content); wErr != nil {
		return fmt.Errorf("could not write %s: %w", path, wErr)
	}
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("could not stat output for %s: %w", path, err)
	}
	if info.Mode().Perm()&0o400 == 0 {
		return fmt.Errorf("creation policy makes %s owner-unreadable (mode %s); rpack must be able to verify its outputs", path, FormatMode(info.Mode()))
	}
	if CanonicalMode(info.Mode()) != CanonicalMode(intent) {
		return fmt.Errorf("creation policy prevents %s output %s (mode %s); check umask and filesystem permissions", modeIntent(CanonicalMode(intent)), path, FormatMode(info.Mode()))
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("could not close output for %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // intended user destination write; path is a resolver-validated relative output
		return fmt.Errorf("could not publish %s: %w", path, err)
	}
	return nil
}
