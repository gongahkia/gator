package cmd

import "github.com/spf13/cobra"

var (
	benchConfigName      string
	benchDataset         string
	benchModel           string
	benchJobsDir         string
	benchJobName         string
	benchResultsPath     string
	benchAgentImportPath string
	benchHarborBin       string
	benchNConcurrent     int
	benchNTasks          int
	benchNAttempts       int
	benchDryRun          bool
)

var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Run paw Harbor benchmark sweeps",
	Example: `  paw bench --config raw --n-tasks 1 --dry-run
  paw bench --config full --n-concurrent 4 --n-tasks 89 --job-name paw-full-full
  paw bench --config full --dataset "$BENCH_SWEBENCH_DATASET" --job-name paw-swebench-full`,
	RunE: runBenchCommand,
}

func init() {
	rootCmd.AddCommand(benchCmd)
	benchCmd.Flags().StringVar(&benchConfigName, "config", "full", "benchmark config: full, no-compress, raw")
	benchCmd.Flags().StringVar(&benchDataset, "dataset", "terminal-bench@2.0", "Harbor dataset")
	benchCmd.Flags().StringVar(&benchModel, "model", "openai/glm-4.6", "Harbor model")
	benchCmd.Flags().StringVar(&benchJobsDir, "jobs-dir", ".paw/bench-jobs", "Harbor jobs directory")
	benchCmd.Flags().StringVar(&benchJobName, "job-name", "", "Harbor job name")
	benchCmd.Flags().StringVar(&benchResultsPath, "results-file", "docs/RESULTS.md", "results markdown file to update")
	benchCmd.Flags().StringVar(&benchAgentImportPath, "agent-import-path", "paw_harbor:PawAgent", "Harbor agent import path")
	benchCmd.Flags().StringVar(&benchHarborBin, "harbor-bin", "harbor", "Harbor executable")
	benchCmd.Flags().IntVar(&benchNConcurrent, "n-concurrent", 4, "Harbor concurrency")
	benchCmd.Flags().IntVar(&benchNTasks, "n-tasks", 0, "optional Harbor task subset size")
	benchCmd.Flags().IntVar(&benchNAttempts, "n-attempts", 0, "optional Harbor attempts per task")
	benchCmd.Flags().BoolVar(&benchDryRun, "dry-run", false, "print Harbor command without running it")
}
