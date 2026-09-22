package rpack

import (
	"fmt"
	"os"
	"regexp"
)

// Mode handling for output file permissions (issue #15).
//
// The contract supports exactly two permission intents, mirroring Git's
// canonical file modes: non-executable (644) and executable (755). Modes
// cross the Lua boundary as octal strings ("755"), because Lua has no
// octal literals and decimal 493 is unreadable. "644"/"0644" and
// "755"/"0755" are compatibility aliases for the two intents; exact rwx
// modes are no longer part of the contract. Below the Lua layer only
// os.FileMode is used, and both spellings of an intent funnel through
// fs.Chmod with the canonical mode.

const (
	// NonExecutableMode is the canonical mode for non-executable output
	// (Git's 100644).
	NonExecutableMode os.FileMode = 0o644
	// ExecutableMode is the canonical mode for executable output
	// (Git's 100755).
	ExecutableMode os.FileMode = 0o755
)

// ownerExecuteBit is the single bit used to classify executability. Like
// Git, classification consults owner-execute alone; group/other execute
// bits are local materialization detail, not intent.
const ownerExecuteBit os.FileMode = 0o100

var (
	// octalModePattern matches the three-octal-digit shape with an optional
	// single leading zero ("755", "0755"). It separates malformed input
	// from well-formed but non-canonical modes so rejections are actionable.
	octalModePattern = regexp.MustCompile(`^0?[0-7]{3}$`)
	// specialBitsPattern matches the 4-digit form whose leading digit
	// carries setuid/setgid/sticky — rejected with a dedicated message. The
	// leading digit must be non-zero so "0644" stays a leading-zero alias.
	specialBitsPattern = regexp.MustCompile(`^[1-7][0-7]{3}$`)
)

// ParseOctalMode parses a declared mode string into an os.FileMode. Only
// the canonical compatibility aliases are accepted: "644"/"0644"
// (non-executable) and "755"/"0755" (executable). Exact rwx modes such as
// "600" were removed from the contract (issue #15); those rejections name
// the supported values so the fix is actionable. Special bits keep their
// dedicated message.
func ParseOctalMode(s string) (os.FileMode, error) {
	switch s {
	case "644", "0644":
		return NonExecutableMode, nil
	case "755", "0755":
		return ExecutableMode, nil
	}
	if specialBitsPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q: special mode bits (setuid/setgid/sticky) are not supported", s)
	}
	if octalModePattern.MatchString(s) {
		return 0, fmt.Errorf("unsupported exact mode %q: only \"644\" (non-executable) and \"755\" (executable), optionally with a leading zero, are supported; use the executable option for executability", s)
	}
	return 0, fmt.Errorf("invalid mode %q: must be an octal string like \"755\" (3 digits, 0-7)", s)
}

// CanonicalMode classifies a file mode as the canonical intent string:
// "755" when the owner-execute bit is set, "644" otherwise. Like Git,
// classification consults owner-execute alone, so 0775 and 0700 are both
// executable while 0664 and 0600 are both non-executable.
func CanonicalMode(mode os.FileMode) string {
	if mode.Perm()&ownerExecuteBit != 0 {
		return "755"
	}
	return "644"
}

// FormatMode formats a file mode's permission bits as a raw three-digit
// octal string for diagnostics and legacy-mode migration
// ("644", "600", "750"). It is deliberately not the canonical intent
// classifier — use CanonicalMode for that.
func FormatMode(mode os.FileMode) string {
	return fmt.Sprintf("%03o", mode.Perm())
}
