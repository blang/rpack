/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"

	"github.com/blang/rpack/pkg/rpack"
	"github.com/spf13/cobra"
)

// checkCmd represents the run command
var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Check integrity of a rpack",
	Long:         ``,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := &rpack.Checker{}
		flagWD, err := cmd.Flags().GetString("working-dir")
		if err != nil {
			return err
		}
		if flagWD != "" {
			c.OverrideExecPath = flagWD
		}

		err = c.CheckIntegrity(context.TODO(), args[0])
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)

	checkCmd.PersistentFlags().StringP("working-dir", "w", "", "Override working dir, defaults to location of rpack file")
}
