package util

import (
	"io"
	"os"

	"github.com/pkg/errors"
)

func CopyFile(dst, src string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer srcF.Close()

	info, err := srcF.Stat()
	if err != nil {
		return err
	}

	dstF, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer dstF.Close()

	if _, err := io.Copy(dstF, srcF); err != nil {
		return err
	}
	return nil
}

func CheckFileExists(name string) error {

	exists, err := FileExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return errors.Errorf("File does not exist: %s", name)
	}
	return nil
}

// FileExists checks if a file exists and is not a directory.
func FileExists(name string) (bool, error) {
	fileInfo, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return true, errors.Wrapf(err, "Error accessing file: %s", name)
	}

	// Check if the path is actually a directory.
	if fileInfo.IsDir() {
		return true, errors.Errorf("Path is a directory, not a file: %s", name)
	}
	return true, nil
}

func CheckFileOrDirExists(name string) (dir bool, err error) {
	// Try to obtain the file information.
	fileInfo, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, errors.Wrapf(err, "File or Dir does not exist: %s", name)
	} else if err != nil {
		return false, errors.Wrapf(err, "Error accessing file or dir: %s", name)
	}

	// Check if the path is actually a directory.
	if fileInfo.IsDir() {
		return true, nil
	}

	return false, nil
}
