// Package cmd implements the validate command.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack"
)

// validateCmd represents the validate command.
var validateCmd = &cobra.Command{
	Use:   "validate --def <dir>",
	Short: "Validate an rpack definition",
	Long: `Validate checks that an rpack definition directory contains:

- rpack.yaml with valid schema (name, inputs)
- script.lua (present and readable)
- schema.cue (if present, valid CUE syntax)

Exits 0 if the definition is valid, non-zero with an error message otherwise.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defDir, err := cmd.Flags().GetString("def")
		if err != nil {
			return err
		}
		if defDir == "" {
			return cmd.Usage()
		}
		_, err = rpack.ValidateRPackDef(defDir)
		if err != nil {
			return fmt.Errorf("invalid definition: %w", err)
		}
		fmt.Println("Definition is valid.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().StringP("def", "d", "", "Path to rpack definition directory")
}
