package cmd

import (
	"fmt"
	"os"

	"github.com/gongahkia/paw/internal/ui"
	"github.com/spf13/cobra"
)

var (
	configPath string
	traceFile  string
	verbose    bool
	noColor    bool
)

var rootCmd = &cobra.Command{
	Use:          "paw",
	Short:        "Pipelined Agent Workbench",
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		color := ui.NewColorizer(os.Stderr, ui.WithNoColor(noColor))
		fmt.Fprintln(os.Stderr, color.Red(err.Error()))
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file")
	rootCmd.PersistentFlags().StringVar(&traceFile, "trace-file", "", "trace file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored stderr output")
}
