package rpack

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Legacy lockfile mode migration (issue #15).
//
// Lockfiles written before executable-intent semantics record full octal
// modes ("600", "750"). The current binary records and accepts only the
// canonical modes ("644"/"755"). Legacy records are neither silently skipped
// nor silently rewritten: every check and run hard-errors with a migration
// hint until the owner explicitly migrates via
// `rpack migrate-modes --acknowledge-permission-change <config>`.

// ModeMigrationEntry describes a single lockfile entry whose recorded mode
// was canonicalized by MigrateLockfileModes.
type ModeMigrationEntry struct {
	// Path of the lockfile entry, relative to the lockfile directory.
	Path string `json:"path"`
	// OldMode is the legacy recorded mode before migration (e.g. "600").
	OldMode string `json:"old_mode"`
	// NewMode is the canonical mode after migration ("644"/"755").
	NewMode string `json:"new_mode"`
}

var (
	// legacyOctalModePattern is the pre-canonicalization grammar accepted by
	// parseLegacyMode: three octal digits with an optional single leading
	// zero ("600", "0755"). It mirrors the grammar ParseOctalMode accepted
	// before it was restricted to canonical modes.
	legacyOctalModePattern = regexp.MustCompile(`^0?[0-7]{3}$`)
	// legacySpecialBitsPattern matches 4-digit octal forms whose leading
	// digit carries setuid/setgid/sticky — rejected with a dedicated
	// message, like the pre-restriction ParseOctalMode did. The leading
	// digit must be non-zero so "0644" stays an accepted leading-zero form.
	legacySpecialBitsPattern = regexp.MustCompile(`^[1-7][0-7]{3}$`)
)

// parseLegacyMode parses a recorded lockfile mode using the
// pre-canonicalization octal grammar. It exists only so legacy lockfile
// records can be validated and explicitly migrated; it must never be used
// for new mode declarations (those are restricted to the canonical modes)
// and deliberately never calls the now-restricted ParseOctalMode.
func parseLegacyMode(s string) (os.FileMode, error) {
	if legacySpecialBitsPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q: special mode bits (setuid/setgid/sticky) are not supported", s)
	}
	if !legacyOctalModePattern.MatchString(s) {
		return 0, fmt.Errorf("invalid mode %q: must be an octal string like \"600\" (3 digits, 0-7)", s)
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q: %w", s, err)
	}
	return os.FileMode(v), nil
}

// isCanonicalRecordedMode reports whether a recorded lockfile mode is one of
// the canonical executable-intent modes the current binary writes ("644"/
// "755", via CanonicalMode). Empty (unknown, pre-feature entry) is handled
// by callers separately.
func isCanonicalRecordedMode(mode string) bool {
	return mode == CanonicalMode(ExecutableMode) || mode == CanonicalMode(NonExecutableMode)
}

// checkRecordedMode validates a lockfile-recorded mode:
//   - empty means unknown (pre-feature entry): valid, check is skipped;
//   - canonical modes are the only records the current binary writes;
//   - any other value parsing under the legacy octal grammar is a legacy
//     record that requires explicit migration;
//   - anything else is invalid.
func checkRecordedMode(mode string) error {
	if mode == "" || isCanonicalRecordedMode(mode) {
		return nil
	}
	if _, err := parseLegacyMode(mode); err != nil {
		return err
	}
	return fmt.Errorf("legacy mode %q requires migration: run 'rpack migrate-modes --acknowledge-permission-change <config>'", mode)
}

// modeIntent describes a canonical mode as executable intent. Drift
// diagnostics use intent words, never literal rwx bits (issue #15).
func modeIntent(mode string) string {
	if mode == CanonicalMode(ExecutableMode) {
		return "executable"
	}
	return "non-executable"
}

// modeDriftMessage formats an executable-intent drift diagnostic for a
// managed file. Both modes are canonical, so the message shows the intent
// plus the canonical octal record, e.g.
// "deploy.sh (expected executable (755), found non-executable (644))".
func modeDriftMessage(path, expected, found string) string {
	return fmt.Sprintf("%s (expected %s (%s), found %s (%s))",
		path, modeIntent(expected), expected, modeIntent(found), found)
}

// MigrateLockfileModes migrates legacy recorded modes in the lockfile
// belonging to the rpack config `name` to canonical executable-intent modes.
//
// It loads only the config and lockfile (no source fetch, no script
// execution) and refuses when no lockfile exists yet. Legacy recorded modes
// are canonicalized from the RECORDED value by owner-execute intent, never
// from the observed disk state, so on-disk executable drift cannot be
// blessed by migration. Entries with unknown (empty) modes stay unknown, and
// already-canonical entries are skipped.
//
// Before writing, full content, executable-intent, and removed-file
// integrity is checked against the canonicalized lockfile; any conflict
// refuses the migration regardless of acknowledgment. No managed file is
// touched (no chmod, no content changes); the lockfile is only rewritten
// when at least one entry actually changed.
func MigrateLockfileModes(name, overrideExecPath string) ([]ModeMigrationEntry, error) {
	ci, err := LoadRPackConfig(name)
	if err != nil {
		return nil, fmt.Errorf("could not load rpack config: %s: %w", name, err)
	}

	// A missing lockfile has nothing to migrate; require a regular run
	// first so migration is never confused with installation.
	if _, statErr := os.Stat(ci.LockFilePath); statErr != nil {
		return nil, fmt.Errorf("no lockfile found at %s: run 'rpack run %s' first", ci.LockFilePath, name)
	}

	execPath := ci.ConfigPath
	if overrideExecPath != "" {
		execPath = overrideExecPath
	}

	// Clone every entry before modifying anything, so the loaded original
	// stays untouched and only verified canonicalized state is ever written.
	migrated := &RPackLockFile{
		SchemaVersion: ci.LockFile.SchemaVersion,
		Files:         make([]*RPackLockFileFile, 0, len(ci.LockFile.Files)),
	}
	var entries []ModeMigrationEntry
	for _, file := range ci.LockFile.Files {
		clone := &RPackLockFileFile{Path: file.Path, Sha: file.Sha, Mode: file.Mode}
		switch {
		case clone.Mode == "":
			// Unknown (pre-feature) entry: keep unknown, never invent intent.
		case isCanonicalRecordedMode(clone.Mode):
			// Already canonical: nothing to migrate.
		default:
			mode, parseErr := parseLegacyMode(clone.Mode)
			if parseErr != nil {
				return nil, fmt.Errorf("lockfile entry %s has invalid recorded mode: %w", clone.Path, parseErr)
			}
			// Canonicalize the RECORDED mode, not the on-disk mode: the
			// record is the author's declaration, the disk state is not.
			clone.Mode = CanonicalMode(mode)
			entries = append(entries, ModeMigrationEntry{
				Path:    clone.Path,
				OldMode: file.Mode,
				NewMode: clone.Mode,
			})
		}
		migrated.Files = append(migrated.Files, clone)
	}

	// Full integrity guard on the canonicalized state: content, executable
	// intent, and existence must all be clean before anything is written.
	// Conflicts refuse the migration regardless of acknowledgment — the
	// acknowledgment only relinquishes legacy read/write guarantees, it
	// never blesses drift.
	integrity, err := migrated.CheckIntegrity(execPath)
	if err != nil {
		return nil, fmt.Errorf("cannot migrate lockfile %s: %w", ci.LockFilePath, err)
	}
	if conflict := integrityConflict(integrity); conflict != "" {
		return nil, fmt.Errorf("cannot migrate lockfile %s:\n%s\nRestore or resolve the reported files before migrating", ci.LockFilePath, conflict)
	}

	if len(entries) == 0 {
		slog.Info("No legacy modes recorded in lockfile, nothing to migrate", "lockfile", ci.LockFilePath)
		return entries, nil
	}

	if err := migrated.WriteFile(ci.LockFilePath); err != nil {
		return nil, fmt.Errorf("could not write migrated lockfile %s: %w", ci.LockFilePath, err)
	}
	return entries, nil
}

// integrityConflict formats a lockfile integrity result as human-readable
// conflict lines, or returns "" when the result is clean.
func integrityConflict(i *RPackLockFileIntegrity) string {
	var conflicts []string
	if len(i.Modified) > 0 {
		conflicts = append(conflicts, fmt.Sprintf("modified files: %s", strings.Join(i.Modified, ", ")))
	}
	if len(i.ModeModified) > 0 {
		conflicts = append(conflicts, fmt.Sprintf("executable intent changed: %s", strings.Join(i.ModeModified, ", ")))
	}
	if len(i.Removed) > 0 {
		conflicts = append(conflicts, fmt.Sprintf("removed files: %s", strings.Join(i.Removed, ", ")))
	}
	return strings.Join(conflicts, "\n")
}
