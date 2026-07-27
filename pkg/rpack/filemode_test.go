package rpack

import (
	"os"
	"strings"
	"testing"
)

// TestParseOctalMode pins the declared-mode grammar (ADR 0001): three octal
// digits with optional leading zero, no special bits, no owner-unreadable
// modes (rpack hashes its own outputs), no non-octal shapes.
func TestParseOctalMode(t *testing.T) {
	accept := []struct {
		input string
		want  os.FileMode
	}{
		{"755", 0o755},
		{"0755", 0o755},
		{"644", 0o644},
		{"0644", 0o644},
		{"600", 0o600},
		{"700", 0o700},
		{"400", 0o400},
		{"777", 0o777},
		{"444", 0o444},
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
		// Owner-read missing (self-DoS on hashing)
		"000", "200", "077",
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
	// The special-bits shape gets its dedicated message, not the generic one.
	if _, err := ParseOctalMode("1755"); err == nil ||
		!strings.Contains(err.Error(), "setuid") {
		t.Fatalf("1755: want setuid message, got %v", err)
	}
	// Owner-unreadable modes explain why.
	if _, err := ParseOctalMode("000"); err == nil ||
		!strings.Contains(err.Error(), "owner-readable") {
		t.Fatalf("000: want owner-readable message, got %v", err)
	}
	// Garbage shows the offending value and the grammar.
	if _, err := ParseOctalMode("88x"); err == nil ||
		!strings.Contains(err.Error(), `"88x"`) || !strings.Contains(err.Error(), `"755"`) {
		t.Fatalf("88x: want value+grammar message, got %v", err)
	}
}

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
