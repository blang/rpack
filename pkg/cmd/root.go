package cmd

import (
	"io"
	"log/slog"
	"os"

	"github.com/golang-cz/devslog"
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "rpack",
	Version: BuildVersion,
	Short:   "RPack file packaging",
	Long:    ``,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		flagDebug, _ := cmd.Flags().GetBool("debug")
		flagNoColor, _ := cmd.Flags().GetBool("no-color")
		logLevel := slog.LevelInfo
		if flagDebug {
			logLevel = slog.LevelDebug
		}
		setupLogger(logLevel, flagNoColor)
	},
}

// Execute runs the root command.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// setupLogger installs the default logger on os.Stderr. Note that devslog
// additionally auto-disables color when NO_COLOR is non-empty or TERM=dumb.
func setupLogger(lvl slog.Level, noColor bool) {
	slog.SetDefault(newLogger(lvl, noColor, os.Stderr))
}

// newLogger builds a devslog-based logger writing to w. Split out from
// setupLogger so tests can capture output and exercise color behavior.
func newLogger(lvl slog.Level, noColor bool, w io.Writer) *slog.Logger {
	slogOpts := &slog.HandlerOptions{
		AddSource: false,
		Level:     lvl,
	}

	opts := &devslog.Options{
		HandlerOptions:    slogOpts,
		MaxSlicePrintSize: 100,
		SortKeys:          true,
		TimeFormat:        "[15:04:05]",
		NewLineAfterLog:   false,
		DebugColor:        devslog.Magenta,
		StringerFormatter: true,
		NoColor:           noColor,
	}

	return slog.New(devslog.NewHandler(w, opts))
}

func init() {
	rootCmd.PersistentFlags().BoolP("debug", "", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolP("no-color", "", false, "Disable colored log output")
}
