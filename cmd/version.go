package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	gitCommit = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "version: %s\n", version)
		fmt.Fprintf(cmd.OutOrStdout(), "git_commit: %s\n", gitCommit)
		fmt.Fprintf(cmd.OutOrStdout(), "go: %s\n", runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
