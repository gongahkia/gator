package cmd

import (
	"fmt"
	"io"
	"strconv"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/pricing"
	"github.com/gongahkia/paw/internal/stage"
	"github.com/spf13/cobra"
)

var statsRate string

var statsCmd = &cobra.Command{
	Use:   "stats [task-id]",
	Short: "Summarize a run trace",
	Example: `  paw stats task-abc123
  paw stats --trace-file .paw/trace-task-abc123.ndjson
  paw stats task-abc123 --rate brain:in=3.0,brain:out=15.0,drone=0.25`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return usageErrorf("expected at most one task id")
		}
		if len(args) == 0 && traceFile == "" {
			return usageErrorf("expected task id or --trace-file")
		}
		taskID := ""
		if len(args) == 1 {
			taskID = args[0]
		}
		path := traceFile
		if path == "" {
			path = runTracePath(taskID)
		}
		events, err := stage.ReadTraceEvents(path)
		if err != nil {
			return err
		}
		summary, err := summarizeTrace(path, taskID, events)
		if err != nil {
			return err
		}
		rates, err := pricing.ParseRates(statsRate)
		if err != nil {
			return usageError(err)
		}
		writeStats(cmd.OutOrStdout(), summary, rates)
		return nil
	},
}

type traceStats struct {
	TaskID             string
	Turns              int
	BrainInput         int
	BrainOutput        int
	BrainCacheCreation int
	BrainCacheRead     int
	BrainTokenSource   string
	Drone              int
	DroneTokenSource   string
	WallMS             int64
	Verify             string
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().StringVar(&statsRate, "rate", "", "token rates per 1M tokens, comma-separated")
}

func summarizeTrace(path, fallbackTaskID string, events []stage.TraceEvent) (traceStats, error) {
	var last *envelope.Envelope
	var wallMS int64
	for i := range events {
		wallMS += events[i].DurationMS
		if events[i].Envelope != nil {
			last = events[i].Envelope
		}
	}
	if last == nil {
		return traceStats{}, fmt.Errorf("trace %s contains no envelope snapshots", path)
	}
	taskID := last.TaskID
	if taskID == "" {
		taskID = fallbackTaskID
	}
	turns := last.Budget.Turn
	if last.Turn > turns {
		turns = last.Turn
	}
	return traceStats{
		TaskID:             taskID,
		Turns:              turns,
		BrainInput:         last.Budget.BrainInputTokens,
		BrainOutput:        last.Budget.BrainOutputTokens,
		BrainCacheCreation: last.Budget.BrainCacheCreationTokens,
		BrainCacheRead:     last.Budget.BrainCacheReadTokens,
		BrainTokenSource: budget.SourceForTokens(
			last.Budget.BrainInputTokens+last.Budget.BrainOutputTokens+last.Budget.BrainCacheCreationTokens+last.Budget.BrainCacheReadTokens,
			last.Budget.BrainTokenSource,
		),
		Drone:            last.Budget.DroneTokens,
		DroneTokenSource: budget.SourceForTokens(last.Budget.DroneTokens, last.Budget.DroneTokenSource),
		WallMS:           wallMS,
		Verify:           verifyStatus(last),
	}, nil
}

func verifyStatus(env *envelope.Envelope) string {
	if env.Verify == nil {
		return "unknown"
	}
	if env.Verify.Passed {
		return "passed"
	}
	return "failed"
}

func writeStats(w io.Writer, s traceStats, rates pricing.Rates) {
	brainCost, hasBrainCost := rates.BrainCost(s.BrainInput, s.BrainOutput, s.BrainCacheCreation, s.BrainCacheRead)
	droneCost, hasDroneCost := rates.Cost(pricing.DroneTotal, s.Drone)
	cached := s.BrainCacheCreation + s.BrainCacheRead
	_, _ = fmt.Fprintf(w, "Task:      %s\n", s.TaskID)
	_, _ = fmt.Fprintf(w, "Turns:     %d\n", s.Turns)
	_, _ = fmt.Fprintf(w, "Brain:     %s in / %s out", formatCount(s.BrainInput), formatCount(s.BrainOutput))
	if cached > 0 {
		_, _ = fmt.Fprintf(w, " (cached: %s, %.1f%%)", formatCount(cached), cachedPercent(s.BrainInput, cached))
	}
	_, _ = fmt.Fprintf(w, " -> %s\n", formatCost(brainCost, hasBrainCost))
	_, _ = fmt.Fprintf(w, "Drone:     %s total -> %s\n", formatCount(s.Drone), formatCost(droneCost, hasDroneCost))
	_, _ = fmt.Fprintf(w, "Token src: brain=%s drone=%s\n", s.BrainTokenSource, s.DroneTokenSource)
	_, _ = fmt.Fprintf(w, "Wall time: %.1fs\n", float64(s.WallMS)/1000)
	_, _ = fmt.Fprintf(w, "Verify:    %s\n", s.Verify)
}

func formatCount(n int) string {
	if n < 0 {
		return "-" + formatCount(-n)
	}
	s := strconv.Itoa(n)
	out := make([]byte, 0, len(s)+(len(s)-1)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func cachedPercent(input, cached int) float64 {
	total := input + cached
	if total == 0 {
		return 0
	}
	return float64(cached) * 100 / float64(total)
}

func formatCost(cost float64, ok bool) string {
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("$%.4f", cost)
}
