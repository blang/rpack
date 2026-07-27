package rpack

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// Mode handling for output file permissions (ADR 0001).
//
// Modes cross the Lua boundary as octal strings ("755"), because Lua has no
// octal literals and decimal 493 is unreadable. Below the Lua layer only
// os.FileMode is used. The lockfile records the same octal string vocabulary.

var (
	// octalModePattern is the accepted grammar: three octal digits with an
	// optional single leading zero ("755", "0755").
	octalModePattern = regexp.MustCompile(`^0?[0-7]{3}$`)
	// specialBitsPattern matches the 4-digit form whose leading digit carries
	// setuid/setgid/sticky — rejected with a dedicated message. The leading
	// digit must be non-zero so "0644" stays an accepted leading-zero form.
	specialBitsPattern = regexp.MustCompile(`^[1-7][0-7]{3}$`)
)

// ParseOctalMode parses a declared mode string into an os.FileMode.
// It rejects special bits (setuid/setgid/sticky) and any mode without
// owner-read: rpack hashes its own outputs (computeFilesToMove), so an
// owner-unreadable staged file would fail the run after successful execution
// and, once recorded, would hard-error integrity checks before --force could
// heal.
func ParseOctalMode(s string) (os.FileMode, error) {
	if specialBitsPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q: special mode bits (setuid/setgid/sticky) are not supported", s)
	}
	if !octalModePattern.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q: must be an octal string like \"755\" (3 digits, 0-7)", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q: %w", s, err)
	}
	mode := os.FileMode(v)
	if mode&0o400 == 0 {
		return 0, fmt.Errorf("invalid mode %q: mode must keep the file owner-readable; rpack hashes its own outputs", s)
	}
	return mode, nil
}

// FormatMode formats a file mode's permission bits as the canonical octal
// string recorded in the lockfile and shown in messages ("644", "755").
func FormatMode(mode os.FileMode) string {
	return fmt.Sprintf("%03o", mode.Perm())
}
