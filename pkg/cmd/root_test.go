package cmd

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// logToBuffer runs a logger built with the given options and env, returning
// everything it writes for one Info call. t.Setenv isolates NO_COLOR/TERM per
// test (devslog reads them when the handler is constructed).
func logToBuffer(t *testing.T, noColor bool) string {
	t.Helper()

	var buf bytes.Buffer
	logger := newLogger(slog.LevelInfo, noColor, &buf)
	logger.Info("color-probe-message")
	return buf.String()
}

// TestNoColorFlagPersistent asserts --no-color is registered as a global
// persistent boolean flag, so it is inherited by every subcommand.
func TestNoColorFlagPersistent(t *testing.T) {
	f := rootCmd.PersistentFlags().Lookup("no-color")
	if f == nil {
		t.Fatalf("expected persistent --no-color flag on root command")
	}
	if f.Value.Type() != "bool" {
		t.Fatalf("expected --no-color to be a bool flag, got %q", f.Value.Type())
	}
}

// TestLoggerDefaultHasANSI asserts that, with color-friendly env (NO_COLOR
// empty, TERM not dumb), the default logger emits ANSI escape sequences —
// proving color output is on unless explicitly or environmentally disabled.
func TestLoggerDefaultHasANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	out := logToBuffer(t, false)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escapes in default output, got: %q", out)
	}
	if !strings.Contains(out, "color-probe-message") {
		t.Fatalf("expected log message in output, got: %q", out)
	}
}

// TestLoggerNoColor asserts color is suppressed — but the message kept — for
// each disable path: the explicit flag, non-empty NO_COLOR, and TERM=dumb.
func TestLoggerNoColor(t *testing.T) {
	cases := []struct {
		name       string
		noColorEnv string
		term       string
		noColor    bool
	}{
		{"explicit flag", "", "xterm-256color", true},
		{"NO_COLOR env", "1", "xterm-256color", false},
		{"TERM=dumb", "", "dumb", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.noColorEnv)
			t.Setenv("TERM", tc.term)

			out := logToBuffer(t, tc.noColor)
			if strings.Contains(out, "\x1b[") {
				t.Fatalf("expected no ANSI escapes, got: %q", out)
			}
			if !strings.Contains(out, "color-probe-message") {
				t.Fatalf("expected log message in output, got: %q", out)
			}
		})
	}
}

// TestDebugFlagStillPersistent guards the pre-existing --debug contract while
// the flag set is being touched for --no-color.
func TestDebugFlagStillPersistent(t *testing.T) {
	f := rootCmd.PersistentFlags().Lookup("debug")
	if f == nil {
		t.Fatalf("expected persistent --debug flag on root command")
	}
	if f.Value.Type() != "bool" {
		t.Fatalf("expected --debug to be a bool flag, got %q", f.Value.Type())
	}
}
