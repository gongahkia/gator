package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath string
	traceFile  string
	verbose    bool
)

var rootCmd = &cobra.Command{
	Use:          "paw",
	Short:        "Pipelined Agent Workbench",
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "config file")
	rootCmd.PersistentFlags().StringVar(&traceFile, "trace-file", "", "trace file")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose output")
}
