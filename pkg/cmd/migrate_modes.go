// Package cmd implements the migrate-modes command.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack"
)

// migrateModesCmd migrates legacy recorded lockfile modes to canonical
// executable-intent modes (issue #15).
var migrateModesCmd = &cobra.Command{
	Use:          "migrate-modes --acknowledge-permission-change [-w <dir>] <config-file>",
	Short:        "Migrate legacy lockfile modes to canonical executable intent",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	Long: `Migrate legacy recorded permission modes in a lockfile to the canonical
executable-intent modes ("644"/"755").

Only the RECORDED modes in the lockfile are rewritten; files on disk are
never modified. Legacy read/write guarantees (e.g. owner-only "600") are
relinquished: from now on the lockfile only guarantees executable intent.
Because this weakens recorded guarantees, the command refuses to run unless
the change is explicitly acknowledged with --acknowledge-permission-change.

Any content, executable-intent, or removed-file drift refuses the migration:
there is no --force on this command, and acknowledgment never blesses drift.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ack, err := cmd.Flags().GetBool("acknowledge-permission-change")
		if err != nil {
			return err
		}
		// The flag must actually be true: mere presence (e.g.
		// --acknowledge-permission-change=false) is not acknowledgment.
		if !ack {
			return fmt.Errorf("mode migration relinquishes the legacy read/write permission guarantees recorded in the lockfile; re-run with --acknowledge-permission-change to acknowledge this change")
		}
		workingDir, err := cmd.Flags().GetString("working-dir")
		if err != nil {
			return err
		}

		entries, err := rpack.MigrateLockfileModes(args[0], workingDir)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			cmd.Println("Nothing to migrate: no legacy modes recorded in lockfile")
			return nil
		}
		cmd.Println("WARNING: migrated entries no longer record read/write permission guarantees; only executable intent is guaranteed from now on")
		cmd.Println("WARNING: files on disk were not modified")
		for _, entry := range entries {
			cmd.Printf("migrated %s: recorded mode %s -> %s\n", entry.Path, entry.OldMode, entry.NewMode)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateModesCmd)

	migrateModesCmd.Flags().Bool("acknowledge-permission-change", false, "Acknowledge that migration relinquishes legacy read/write permission guarantees (recorded modes become canonical executable intent)")
	migrateModesCmd.Flags().StringP("working-dir", "w", "", "Override working dir, defaults to location of rpack file")
}
