package rpack

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestParseOctalMode pins backward-compatible parsing: every value accepted by
// the former exact-mode contract is reduced to canonical executable intent.
func TestParseOctalMode(t *testing.T) {
	accept := []struct {
		input string
		want  os.FileMode
	}{
		{"400", NonExecutableMode},
		{"0444", NonExecutableMode},
		{"600", NonExecutableMode},
		{"0600", NonExecutableMode},
		{"640", NonExecutableMode},
		{"644", NonExecutableMode},
		{"0644", NonExecutableMode},
		{"664", NonExecutableMode},
		{"500", ExecutableMode},
		{"0700", ExecutableMode},
		{"750", ExecutableMode},
		{"755", ExecutableMode},
		{"0755", ExecutableMode},
		{"777", ExecutableMode},
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
		// Owner-unreadable modes were already rejected by the old contract.
		"000", "044", "100", "0377",
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

// TestParseOctalMode_PreservesFormerGrammar exhaustively covers all 000-777
// values, with and without the old optional leading zero. Every owner-readable
// value accepted by the exact-mode implementation must remain accepted.
func TestParseOctalMode_PreservesFormerGrammar(t *testing.T) {
	for raw := 0; raw <= 0o777; raw++ {
		for _, input := range []string{fmt.Sprintf("%03o", raw), fmt.Sprintf("0%03o", raw)} {
			got, err := ParseOctalMode(input)
			if os.FileMode(raw)&0o400 == 0 {
				if err == nil {
					t.Fatalf("ParseOctalMode(%q) = %o, want historical owner-read rejection", input, got)
				}
				continue
			}
			if err != nil {
				t.Fatalf("ParseOctalMode(%q) broke backward compatibility: %v", input, err)
			}
			want := canonicalOutputMode(os.FileMode(raw))
			if got != want {
				t.Fatalf("ParseOctalMode(%q) = %o, want canonical intent %o", input, got, want)
			}
		}
	}
}

func TestParseOctalModeErrorMessages(t *testing.T) {
	// The historical owner-read constraint remains actionable.
	if _, err := ParseOctalMode("000"); err == nil ||
		!strings.Contains(err.Error(), `"000"`) ||
		!strings.Contains(err.Error(), "owner-readable") {
		t.Fatalf("000: want actionable owner-readable message, got %v", err)
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
