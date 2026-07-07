package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

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
	RunE: func(cmd *cobra.Command, _ []string) error {
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
		row := summary.row(spec.Name)
		if err := printBenchTable(cmd.OutOrStdout(), []benchRow{row}); err != nil {
			return err
		}
		if benchResultsPath != "" {
			return updateResultsFile(benchResultsPath, row)
		}
		return nil
	},
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

type benchConfig struct {
	Name string
}

func benchConfigSpec(name string) (benchConfig, error) {
	switch name {
	case "full", "no-compress", "raw":
		return benchConfig{Name: name}, nil
	default:
		return benchConfig{}, usageErrorf("unsupported bench config %q", name)
	}
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

func buildHarborArgs(opts benchOptions) []string {
	args := []string{
		"run",
		"--dataset", opts.Dataset,
		"--agent-import-path", opts.AgentImportPath,
		"--model", opts.Model,
		"--n-concurrent", strconv.Itoa(opts.NConcurrent),
		"--jobs-dir", opts.JobsDir,
		"--job-name", opts.JobName,
		"--agent-env", "PAW_BENCH_CONFIG=" + opts.Config,
	}
	if opts.NTasks > 0 {
		args = append(args, "--n-tasks", strconv.Itoa(opts.NTasks))
	}
	if opts.NAttempts > 0 {
		args = append(args, "--n-attempts", strconv.Itoa(opts.NAttempts))
	}
	return args
}

func runHarbor(ctx context.Context, bin string, args []string, log io.Writer) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	return cmd.Run()
}

type benchSummary struct {
	Tasks       int
	Passed      int
	BrainTokens []float64
	DroneTokens []float64
	WallSeconds []float64
}

type benchRow struct {
	Config      string
	Tasks       int
	PassRate    string
	BrainTokens string
	DroneTokens string
	WallSeconds string
}

func (s benchSummary) row(config string) benchRow {
	return benchRow{
		Config:      config,
		Tasks:       s.Tasks,
		PassRate:    ratio(s.Passed, s.Tasks),
		BrainTokens: formatMedian(s.BrainTokens, 0),
		DroneTokens: formatMedian(s.DroneTokens, 0),
		WallSeconds: formatMedian(s.WallSeconds, 1),
	}
}

func loadBenchSummary(root string) (benchSummary, error) {
	var summary benchSummary
	seenTrials := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		switch {
		case entry.Name() == "result.json":
			return readResultJSON(path, &summary, seenTrials)
		case entry.Name() == "reward.txt" && !hasSiblingTrialResult(path):
			reward, ok := readRewardText(path)
			if ok {
				summary.Tasks++
				if reward >= 1 {
					summary.Passed++
				}
			}
		case strings.HasSuffix(entry.Name(), ".ndjson") && isTracePath(path):
			brain, drone, ok := readTrace(path)
			if ok {
				summary.BrainTokens = append(summary.BrainTokens, float64(brain))
				summary.DroneTokens = append(summary.DroneTokens, float64(drone))
			}
		}
		return nil
	})
	if err != nil {
		return benchSummary{}, err
	}
	if summary.Tasks == 0 && len(summary.BrainTokens) == 0 && len(summary.DroneTokens) == 0 {
		return benchSummary{}, fmt.Errorf("no Harbor results found under %s", root)
	}
	return summary, nil
}

func readResultJSON(path string, summary *benchSummary, seen map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	if trials, ok := doc["trial_results"].([]any); ok {
		for _, trial := range trials {
			if obj, ok := trial.(map[string]any); ok {
				addTrialResult(obj, summary, seen)
			}
		}
		return nil
	}
	if _, ok := doc["trial_name"]; ok {
		addTrialResult(doc, summary, seen)
	}
	return nil
}

func addTrialResult(trial map[string]any, summary *benchSummary, seen map[string]bool) {
	id := stringField(trial, "id")
	if id == "" {
		id = stringField(trial, "task_name") + "/" + stringField(trial, "trial_name")
	}
	if id == "/" || seen[id] {
		return
	}
	seen[id] = true
	passed, ok := trialPassed(trial)
	if ok {
		summary.Tasks++
		if passed {
			summary.Passed++
		}
	}
	if seconds, ok := trialWallSeconds(trial); ok {
		summary.WallSeconds = append(summary.WallSeconds, seconds)
	}
}

func trialPassed(trial map[string]any) (bool, bool) {
	verifier, ok := trial["verifier_result"].(map[string]any)
	if !ok {
		return false, false
	}
	rewards, ok := verifier["rewards"].(map[string]any)
	if !ok || len(rewards) == 0 {
		return false, false
	}
	for _, value := range rewards {
		if numberValue(value) >= 1 {
			return true, true
		}
	}
	return false, true
}

func trialWallSeconds(trial map[string]any) (float64, bool) {
	if seconds, ok := durationFields(trial); ok {
		return seconds, true
	}
	if phase, ok := trial["agent_execution"].(map[string]any); ok {
		return durationFields(phase)
	}
	return 0, false
}

func durationFields(obj map[string]any) (float64, bool) {
	started, ok := parseJSONTime(stringField(obj, "started_at"))
	if !ok {
		return 0, false
	}
	finished, ok := parseJSONTime(stringField(obj, "finished_at"))
	if !ok || finished.Before(started) {
		return 0, false
	}
	return finished.Sub(started).Seconds(), true
}

func parseJSONTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func readRewardText(path string) (float64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	reward, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	return reward, err == nil
}

func hasSiblingTrialResult(rewardPath string) bool {
	trialDir := filepath.Dir(filepath.Dir(rewardPath))
	_, err := os.Stat(filepath.Join(trialDir, "result.json"))
	return err == nil
}

type benchTraceEvent struct {
	Stage  string `json:"stage"`
	Tokens int    `json:"tokens"`
}

func readTrace(path string) (int, int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer func() { _ = file.Close() }()
	var brain, drone int
	seen := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event benchTraceEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		seen = true
		switch event.Stage {
		case "plan", "edit":
			brain += event.Tokens
		case "compress":
			drone += event.Tokens
		}
	}
	return brain, drone, seen
}

func isTracePath(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.paw/") || strings.Contains(filepath.Base(clean), "trace")
}

func printBenchTable(w io.Writer, rows []benchRow) error {
	if _, err := fmt.Fprintln(w, "| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| --- | ---: | ---: | ---: | ---: | ---: |"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, row.markdown()); err != nil {
			return err
		}
	}
	return nil
}

const (
	resultsStartMarker = "<!-- paw-results:start -->"
	resultsEndMarker   = "<!-- paw-results:end -->"
)

func updateResultsFile(path string, row benchRow) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	start, end := markerBounds(lines)
	if start == -1 || end == -1 || start >= end {
		return fmt.Errorf("results table markers not found in %s", path)
	}
	next := row.markdown()
	replaced := false
	for i := start + 1; i < end; i++ {
		if strings.HasPrefix(lines[i], "| "+row.Config+" |") {
			lines[i] = next
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines[:end], append([]string{next}, lines[end:]...)...)
	}
	out := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(out), 0o644)
}

func markerBounds(lines []string) (int, int) {
	start, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case resultsStartMarker:
			start = i
		case resultsEndMarker:
			end = i
			return start, end
		}
	}
	return start, end
}

func (r benchRow) markdown() string {
	return fmt.Sprintf("| %s | %d | %s | %s | %s | %s |", r.Config, r.Tasks, r.PassRate, r.BrainTokens, r.DroneTokens, r.WallSeconds)
}

func ratio(numerator, denominator int) string {
	if denominator == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", float64(numerator)/float64(denominator))
}

func formatMedian(values []float64, decimals int) string {
	if len(values) == 0 {
		return "n/a"
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	value := sorted[mid]
	if len(sorted)%2 == 0 {
		value = (sorted[mid-1] + sorted[mid]) / 2
	}
	return fmt.Sprintf("%."+strconv.Itoa(decimals)+"f", value)
}

func stringField(obj map[string]any, key string) string {
	value, _ := obj[key].(string)
	return value
}

func numberValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	default:
		return 0
	}
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
