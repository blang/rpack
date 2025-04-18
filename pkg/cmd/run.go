/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/blang/rpack/pkg/rpack"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an rpack file",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		e := &rpack.Executor{}
		flagWD, err := cmd.Flags().GetString("working-dir")
		if err != nil {
			return err
		}
		if flagWD != "" {
			e.OverrideExecPath = flagWD
		}
		flagForce, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		e.Force = flagForce

		flagDryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		e.DryRun = flagDryRun

		err = e.ExecRPack(context.TODO(), args[0])
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.PersistentFlags().StringP("working-dir", "w", "", "Override working dir, defaults to location of rpack file")
	runCmd.PersistentFlags().BoolP("force", "f", false, "Force execution: Overwrite files, ignore warnings")
	runCmd.PersistentFlags().BoolP("dry-run", "", false, "Dry run execution")
}
