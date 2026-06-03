// Package cmd implements the publish command.
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack"
	"github.com/blang/rpack/pkg/rpack/getsource"
)

var publishCmd = &cobra.Command{
	Use:   "publish --def <dir> --type <oci|archive> --target <target>",
	Short: "Publish an rpack definition",
	Long: `Publish packages an rpack definition directory and pushes it to a target.

Supported types:
  oci     - Push as OCI artifact to a container registry
  archive - Create a tar.xz archive on disk

OCI example:
  rpack publish -d ./myrpack -T oci -t oci://docker.io/user/pack?tag=v1

Archive example:
  rpack publish -d ./myrpack -T archive -t ./dist/mypack.tar.xz

Authentication for OCI is resolved automatically from Podman/Docker config,
credential helpers, or OCI_USERNAME/OCI_PASSWORD environment variables.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defDir, _ := cmd.Flags().GetString("def")
		pubType, _ := cmd.Flags().GetString("type")
		target, _ := cmd.Flags().GetString("target")

		if defDir == "" || pubType == "" || target == "" {
			return cmd.Usage()
		}

		// Full definition validation (CUE schema, script.lua, schema.cue)
		if _, err := rpack.ValidateRPackDef(defDir); err != nil {
			return fmt.Errorf("definition validation failed: %w", err)
		}

		switch pubType {
		case "oci":
			return getsource.PublishRPack(context.Background(), defDir,
				func(registry, repo string) (getsource.OCIPublisher, error) {
					return getsource.NewORASStore(registry, repo)
				}, target)
		case "archive":
			return getsource.PublishArchive(defDir, target)
		default:
			return fmt.Errorf("unknown publish type %q, valid types: oci, archive", pubType)
		}
	},
}

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringP("def", "d", "", "Path to the rpack definition directory")
	publishCmd.Flags().StringP("type", "T", "", "Publish type: oci or archive")
	publishCmd.Flags().StringP("target", "t", "", "Target URL (oci://) or path (.tar.xz)")
}
