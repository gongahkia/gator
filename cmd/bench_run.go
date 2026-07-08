package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type benchConfig struct {
	Name string
}

type benchOptions struct {
	Config          string
	Dataset         string
	Model           string
	JobsDir         string
	JobName         string
	AgentImportPath string
	NConcurrent     int
	NTasks          int
	NAttempts       int
}

func runBenchCommand(cmd *cobra.Command, _ []string) error {
	spec, err := benchConfigSpec(benchConfigName)
	if err != nil {
		return err
	}
	opts := benchOptions{
		Config:          spec.Name,
		Dataset:         benchDataset,
		Model:           benchModel,
		JobsDir:         benchJobsDir,
		JobName:         benchJobName,
		AgentImportPath: benchAgentImportPath,
		NConcurrent:     benchNConcurrent,
		NTasks:          benchNTasks,
		NAttempts:       benchNAttempts,
	}
	if opts.JobName == "" {
		opts.JobName = "paw-" + spec.Name + "-" + time.Now().UTC().Format("20060102T150405Z")
	}
	args := buildHarborArgs(opts)
	if benchDryRun {
		cmd.Println(shellCommand(benchHarborBin, args))
		return nil
	}
	if err := runHarbor(cmd.Context(), benchHarborBin, args, cmd.ErrOrStderr()); err != nil {
		return err
	}
	summary, err := loadBenchSummary(filepath.Join(opts.JobsDir, opts.JobName))
	if err != nil {
		summary, err = loadBenchSummary(opts.JobsDir)
		if err != nil {
			return err
		}
	}
	if err := validateBenchSummary(summary, opts); err != nil {
		return err
	}
	row := summary.row(spec.Name, opts)
	if err := printBenchTable(cmd.OutOrStdout(), []benchRow{row}); err != nil {
		return err
	}
	if benchResultsPath != "" {
		if err := writeBenchResultConfig(benchResultsPath, row, opts); err != nil {
			return err
		}
		return updateResultsFile(benchResultsPath, row)
	}
	return nil
}

func validateBenchSummary(summary benchSummary, opts benchOptions) error {
	if summary.JobFinishedKnown && !summary.JobFinished {
		return usageErrorf("Harbor job %q did not finish; refusing to record partial results", opts.JobName)
	}
	expected := opts.expectedTrials()
	if expected == 0 {
		return nil
	}
	if summary.Tasks < expected {
		return usageErrorf("incomplete Harbor results for %q: got %d task results, expected %d", opts.JobName, summary.Tasks, expected)
	}
	return nil
}

func (opts benchOptions) expectedTrials() int {
	if opts.NTasks <= 0 {
		return 0
	}
	attempts := opts.NAttempts
	if attempts <= 0 {
		attempts = 1
	}
	return opts.NTasks * attempts
}

func benchConfigSpec(name string) (benchConfig, error) {
	switch name {
	case "full", "no-compress", "raw":
		return benchConfig{Name: name}, nil
	default:
		return benchConfig{}, usageErrorf("unsupported bench config %q", name)
	}
}

func buildHarborArgs(opts benchOptions) []string {
	args := []string{
		"run",
		"--dataset", opts.Dataset,
		"--agent-import-path", opts.AgentImportPath,
		"--model", opts.Model,
		"--n-concurrent", strconv.Itoa(opts.NConcurrent),
		"--jobs-dir", opts.JobsDir,
		"--job-name", opts.JobName,
		"--artifact", "/workspace/.paw/trace.ndjson",
		"--allow-agent-host", "host.docker.internal",
		"--yes",
		"--agent-env", "PAW_BENCH_CONFIG=" + opts.Config,
		"--agent-env", "PAW_TRACE_MODE=" + benchTraceMode(),
	}
	if opts.NTasks > 0 {
		args = append(args, "--n-tasks", strconv.Itoa(opts.NTasks))
	}
	if opts.NAttempts > 0 {
		args = append(args, "--n-attempts", strconv.Itoa(opts.NAttempts))
	}
	return args
}

func benchTraceMode() string {
	if mode := os.Getenv("PAW_BENCH_TRACE_MODE"); mode != "" {
		return mode
	}
	return "compact"
}

func runHarbor(ctx context.Context, bin string, args []string, log io.Writer) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}

func shellCommand(bin string, args []string) string {
	parts := append([]string{bin}, args...)
	for i, part := range parts {
		parts[i] = shellQuote(part)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("._/:=-", r)
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
