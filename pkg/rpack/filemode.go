package rpack

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// Mode handling for output file permissions (issue #15).
//
// The contract supports exactly two permission intents, mirroring Git's
// canonical file modes: non-executable (644) and executable (755). Modes
// cross the Lua boundary as octal strings ("755"), because Lua has no
// octal literals and decimal 493 is unreadable. For backward compatibility,
// every mode accepted by the former exact-mode contract remains valid, but
// it is reduced to executable intent at the boundary. Below the Lua layer
// only canonical os.FileMode values are used.

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

// ParseOctalMode parses a mode string using the former exact-mode grammar and
// reduces it to canonical executable intent. This preserves existing rpack
// definitions and lockfiles while dropping read/write bits from the contract:
// for example, 0600 becomes NonExecutableMode and 0750 becomes ExecutableMode.
// The historical owner-read requirement and special-bit rejection remain.
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
	return canonicalOutputMode(mode), nil
}

// canonicalOutputMode reduces any file mode supplied through the Go filesystem
// API to the two output intents. Lua mode strings are validated before reaching
// this layer, while direct callers retain the old API's broad os.FileMode input.
func canonicalOutputMode(mode os.FileMode) os.FileMode {
	if mode.Perm()&ownerExecuteBit != 0 {
		return ExecutableMode
	}
	return NonExecutableMode
}

// CanonicalMode classifies a file mode as the canonical intent string:
// "755" when the owner-execute bit is set, "644" otherwise. Like Git,
// classification consults owner-execute alone, so 0775 and 0700 are both
// executable while 0664 and 0600 are both non-executable.
func CanonicalMode(mode os.FileMode) string {
	if canonicalOutputMode(mode) == ExecutableMode {
		return "755"
	}
	return "644"
}

// modeIntent describes a canonical mode as executable intent. Diagnostics use
// intent words, never literal local read/write bits.
func modeIntent(mode string) string {
	if mode == CanonicalMode(ExecutableMode) {
		return "executable"
	}
	return "non-executable"
}

// modeDriftMessage formats an executable-intent drift diagnostic. Both mode
// arguments are canonical lockfile values.
func modeDriftMessage(path, expected, found string) string {
	return fmt.Sprintf("%s (expected %s (%s), found %s (%s))",
		path, modeIntent(expected), expected, modeIntent(found), found)
}

// FormatMode formats a file mode's permission bits as a raw three-digit
// octal string for diagnostics
// ("644", "600", "750"). It is deliberately not the canonical intent
// classifier — use CanonicalMode for that.
func FormatMode(mode os.FileMode) string {
	return fmt.Sprintf("%03o", mode.Perm())
}
