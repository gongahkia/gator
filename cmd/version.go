package cmd

import (
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
	Run: func(cmd *cobra.Command, _ []string) {
		cmd.Printf("version: %s\n", version)
		cmd.Printf("git_commit: %s\n", gitCommit)
		cmd.Printf("go: %s\n", runtime.Version())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
