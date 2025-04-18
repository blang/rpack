package cmd

import (
	"os"

	"log/slog"

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
		logLevel := slog.LevelInfo
		if flagDebug {
			logLevel = slog.LevelDebug
		}
		setupLogger(logLevel)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func setupLogger(lvl slog.Level) {
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
	}

	logger := slog.New(devslog.NewHandler(os.Stderr, opts))
	slog.SetDefault(logger)
}

func init() {
	rootCmd.PersistentFlags().BoolP("debug", "", false, "Enable verbose logging")
}
