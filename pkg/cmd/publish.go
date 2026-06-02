// Package cmd implements the publish command.
package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack/getsource"
)

// publishCmd represents the publish command.
var publishCmd = &cobra.Command{
	Use:   "publish --def <dir> --target <oci-url>",
	Short: "Publish an rpack definition to an OCI registry",
	Long: `Publish packages an rpack definition directory and pushes it as an OCI artifact.

The definition directory must contain at least rpack.yaml and script.lua.
The target must be an OCI registry URL: oci://registry.example.com/repo/path?tag=v1

Authentication is read from the OCI_USERNAME and OCI_PASSWORD environment variables.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defDir, err := cmd.Flags().GetString("def")
		if err != nil {
			return err
		}
		if defDir == "" {
			return cmd.Usage()
		}

		target, err := cmd.Flags().GetString("target")
		if err != nil {
			return err
		}
		if target == "" {
			return cmd.Usage()
		}

		return getsource.PublishRPack(
			context.Background(),
			defDir,
			func(registry, repo string) (getsource.OCIPublisher, error) {
				return getsource.NewORASStore(registry, repo)
			},
			target,
		)
	},
}

func init() {
	rootCmd.AddCommand(publishCmd)

	publishCmd.PersistentFlags().StringP("def", "d", "", "Path to the rpack definition directory")
	publishCmd.PersistentFlags().StringP("target", "t", "", "OCI target URL (oci://registry/repo?tag=v1)")
}
