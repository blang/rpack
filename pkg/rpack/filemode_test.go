package rpack

import (
	"os"
	"strings"
	"testing"
)

// TestParseOctalMode pins the declared-mode grammar (issue #15): only the
// canonical compatibility aliases "644"/"0644" (non-executable) and
// "755"/"0755" (executable) are accepted. Exact rwx modes, special bits,
// and non-octal shapes are all rejected.
func TestParseOctalMode(t *testing.T) {
	accept := []struct {
		input string
		want  os.FileMode
	}{
		{"644", NonExecutableMode},
		{"0644", NonExecutableMode},
		{"755", ExecutableMode},
		{"0755", ExecutableMode},
	}
	for _, tc := range accept {
		t.Run("accept/"+tc.input, func(t *testing.T) {
			got, err := ParseOctalMode(tc.input)
			if err != nil {
				t.Fatalf("ParseOctalMode(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseOctalMode(%q) = %o, want %o", tc.input, got, tc.want)
			}
		})
	}

	reject := []string{
		// Exact rwx modes removed from the contract (issue #15)
		"600", "0600", "700", "750", "777", "0777", "400", "444", "664", "000",
		// Special bits
		"1755", "4755", "7777",
		// Malformed
		"75", "7555x", "88x", "888", "0o755", "", " 755", "755 ",
		// Decimal 493 == 0755, still rejected: octal string only
		"493",
	}
	for _, input := range reject {
		t.Run("reject/"+input, func(t *testing.T) {
			if got, err := ParseOctalMode(input); err == nil {
				t.Fatalf("ParseOctalMode(%q) = %v, want error", input, got)
			}
		})
	}
}

func TestParseOctalModeErrorMessages(t *testing.T) {
	// Unsupported exact modes are actionable: they name the offending value
	// and the supported canonical modes.
	if _, err := ParseOctalMode("600"); err == nil ||
		!strings.Contains(err.Error(), `"600"`) ||
		!strings.Contains(err.Error(), `"644"`) ||
		!strings.Contains(err.Error(), `"755"`) {
		t.Fatalf("600: want actionable unsupported-mode message, got %v", err)
	}
	// The special-bits shape gets its dedicated message, not the generic one.
	if _, err := ParseOctalMode("1755"); err == nil ||
		!strings.Contains(err.Error(), "setuid") {
		t.Fatalf("1755: want setuid message, got %v", err)
	}
	// Garbage shows the offending value and the grammar.
	if _, err := ParseOctalMode("88x"); err == nil ||
		!strings.Contains(err.Error(), `"88x"`) || !strings.Contains(err.Error(), `"755"`) {
		t.Fatalf("88x: want value+grammar message, got %v", err)
	}
}

// TestCanonicalMode pins the Git-style classification: owner-execute alone
// decides, so 0775 and 0700 are executable while 0664 and 0600 are not.
func TestCanonicalMode(t *testing.T) {
	tests := []struct {
		want string
		mode os.FileMode
	}{
		{"644", NonExecutableMode},
		{"755", ExecutableMode},
		// Owner-execute set: executable regardless of other bits.
		{"755", 0o777},
		{"755", 0o700},
		{"755", 0o775},
		{"755", 0o751},
		{"755", 0o100},
		// Owner-execute clear: non-executable regardless of other bits.
		{"644", 0o644},
		{"644", 0o664},
		{"644", 0o666},
		{"644", 0o600},
		{"644", 0o044},
		{"644", 0o000},
		// Non-perm bits are ignored.
		{"755", os.ModeSetuid | 0o755},
		{"644", os.ModeDir | 0o644},
	}
	for _, tc := range tests {
		if got := CanonicalMode(tc.mode); got != tc.want {
			t.Errorf("CanonicalMode(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestCanonicalModeConstants pins the exported canonical modes against their
// intent strings.
func TestCanonicalModeConstants(t *testing.T) {
	if got := CanonicalMode(NonExecutableMode); got != "644" {
		t.Errorf("CanonicalMode(NonExecutableMode) = %q, want %q", got, "644")
	}
	if got := CanonicalMode(ExecutableMode); got != "755" {
		t.Errorf("CanonicalMode(ExecutableMode) = %q, want %q", got, "755")
	}
	if NonExecutableMode != 0o644 || ExecutableMode != 0o755 {
		t.Fatalf("canonical mode constants drifted: nonexec=%o exec=%o", NonExecutableMode, ExecutableMode)
	}
}

// TestFormatMode pins FormatMode as the raw rwx diagnostic formatter: it
// prints whatever permission bits are present (including non-canonical
// ones) and strips non-perm bits.
func TestFormatMode(t *testing.T) {
	tests := []struct {
		want string
		mode os.FileMode
	}{
		{"644", 0o644},
		{"755", 0o755},
		{"600", 0o600},
		{"000", 0o000},
		// Non-perm bits are stripped.
		{"755", os.ModeSetuid | 0o755},
	}
	for _, tc := range tests {
		if got := FormatMode(tc.mode); got != tc.want {
			t.Errorf("FormatMode(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
}
