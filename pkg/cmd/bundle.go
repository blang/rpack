// Package cmd implements the CLI commands for rpack.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/blang/rpack/pkg/rpack"
	"github.com/blang/rpack/pkg/rpack/getsource"
)

var bundleCmd = &cobra.Command{
	Use:   "bundle --def <dir> --format <zip|tar.xz|tar.bz2> --output <path>",
	Short: "Bundle an rpack definition into an archive",
	Long: `Bundle packages an rpack definition directory into a single archive file.

Supported formats:
  zip      - ZIP archive (.zip)
  tar.xz   - Tar archive with XZ compression (.tar.xz)
  tar.bz2  - Tar archive with Bzip2 compression (.tar.bz2)

Examples:
  rpack bundle -d ./myrpack -f zip -o ./dist/mypack.zip
  rpack bundle -d ./myrpack -f tar.xz -o ./dist/mypack.tar.xz
  rpack bundle -d ./myrpack -f tar.bz2 -o ./dist/mypack.tar.bz2

The resulting archive can be distributed via OCI registries, HTTP servers,
or any other file distribution mechanism. For OCI distribution, use the
oras CLI tool to push the archive:

  oras push --artifact-type=application/vnd.rpack.modulepkg \
    registry.example.com/repo:v1 ./dist/mypack.zip:archive/zip`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defDir, _ := cmd.Flags().GetString("def")
		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")

		if defDir == "" || format == "" || output == "" {
			return cmd.Usage()
		}

		// Full definition validation (CUE schema, script.lua, schema.cue)
		if _, err := rpack.ValidateRPackDef(defDir); err != nil {
			return fmt.Errorf("definition validation failed: %w", err)
		}

		switch format {
		case "zip":
			return getsource.BundleZip(defDir, output)
		case "tar.xz":
			return getsource.BundleTarXZ(defDir, output)
		case "tar.bz2":
			return getsource.BundleTarBZ2(defDir, output)
		default:
			return fmt.Errorf("unknown format %q, valid formats: zip, tar.xz, tar.bz2", format)
		}
	},
}

func init() {
	rootCmd.AddCommand(bundleCmd)
	bundleCmd.Flags().StringP("def", "d", "", "Path to the rpack definition directory")
	bundleCmd.Flags().StringP("format", "f", "", "Archive format: zip, tar.xz, or tar.bz2")
	bundleCmd.Flags().StringP("output", "o", "", "Output archive path")
}
