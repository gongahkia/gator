package cmd

import (
	"os"

	"github.com/gongahkia/paw/internal/egress"
	pawlog "github.com/gongahkia/paw/internal/log"
	"github.com/spf13/cobra"
)

var (
	configPath string
	traceFile  string
	verbose    bool
	noColor    bool
	logFormat  string
	logLevel   string
)

var rootCmd = &cobra.Command{
	Use:               "paw",
	Short:             "Pipelined Agent Workbench",
	SilenceUsage:      true,
	PersistentPreRunE: installCommandLogger,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		code := codeForError(err)
		logger, logErr := pawlog.Install(pawlog.Config{Format: logFormat, Level: logLevel, Writer: os.Stderr})
		if logErr != nil {
			logger, _ = pawlog.Install(pawlog.Config{Writer: os.Stderr})
		}
		logger.Error("command failed", "error", egress.ScrubText(err.Error()), "exit_code", int(code))
		os.Exit(int(code))
	}
}

func init() {
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file")
	rootCmd.PersistentFlags().StringVar(&traceFile, "trace-file", "", "trace file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored stderr output")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format: text, json")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
}

func installCommandLogger(cmd *cobra.Command, _ []string) error {
	logger, err := pawlog.Install(pawlog.Config{Format: logFormat, Level: logLevel, Writer: cmd.ErrOrStderr()})
	if err != nil {
		return usageError(err)
	}
	cmd.SetContext(pawlog.With(cmd.Context(), logger))
	return nil
}
