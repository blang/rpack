package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func Sha256String(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Sha256File calculates the SHA256 checksum of the file specified by name.
// It returns the checksum as a hex-encoded string. In case of any error
// (like file not found or read error), it returns an error.
func Sha256File(name string) (sha string, err error) {
	// Open the file for reading.
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer func() {
		err2 := file.Close()
		if err == nil && err2 != nil {
			err = err2
		}
	}()

	// Create a new SHA256 hash.
	hasher := sha256.New()

	// Copy the file contents into the hasher. This approach is efficient and
	// automatically uses an internal buffer.
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	// Return the hex-encoded checksum string.
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
