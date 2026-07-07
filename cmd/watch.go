package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
	pawlog "github.com/gongahkia/paw/internal/log"
	"github.com/gongahkia/paw/internal/ui"
	watchpkg "github.com/gongahkia/paw/internal/watch"
	"github.com/spf13/cobra"
)

var (
	watchMarkers         string
	watchDebounce        time.Duration
	watchMaxTurns        int
	watchRawContext      bool
	watchDisableCompress bool
	watchDroneModel      string
	watchQuiet           bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch files for AI markers",
	Example: `  paw watch
  paw watch --markers "ai:,TODO(ai):"
  paw watch --raw-context`,
	Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected args")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		if watchMaxTurns > 0 {
			cfg.MaxTurns = watchMaxTurns
		}
		if watchDroneModel != "" {
			cfg.Drone.Model = watchDroneModel
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		ctx, stop := signalContext(cmd.Context())
		defer stop()
		markers := watchpkg.ParseMarkers(watchMarkers)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "watching %s (markers: %s)\n", cwd, strings.Join(markers, ","))
		watcher := &watchpkg.Watcher{
			Root:     cwd,
			Markers:  markers,
			Debounce: watchDebounce,
			Runner: func(ctx context.Context, marker watchpkg.Marker) error {
				return runWatchMarker(ctx, cmd, cfg, cwd, marker)
			},
			Status: func(status watchpkg.Status) {
				printWatchStatus(cmd.OutOrStdout(), status)
			},
		}
		return watcher.Run(ctx)
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().StringVar(&watchMarkers, "markers", "ai:", "comma-separated marker prefixes")
	watchCmd.Flags().DurationVar(&watchDebounce, "debounce", watchpkg.DefaultDebounce, "save debounce duration")
	watchCmd.Flags().IntVar(&watchMaxTurns, "max-turns", 0, "maximum agent turns")
	watchCmd.Flags().BoolVar(&watchRawContext, "raw-context", false, "bypass model compression")
	watchCmd.Flags().BoolVar(&watchDisableCompress, "disable-compress", false, "use deterministic compression fallback")
	watchCmd.Flags().StringVar(&watchDroneModel, "drone-model", "", "drone model override")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress progress output")
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signalNotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

var signalNotifyContext = signal.NotifyContext

func runWatchMarker(ctx context.Context, cmd *cobra.Command, cfg config.Config, cwd string, marker watchpkg.Marker) error {
	pipeline, err := newAgentPipeline(cfg, watchRawContext, watchDisableCompress)
	if err != nil {
		return err
	}
	env := envelope.NewEnvelope(taskID(marker.Instruction, cwd), marker.Instruction, cwd)
	env.Budget.MaxTurns = cfg.MaxTurns
	env.Budget.MaxBrainTokens = cfg.MaxBrainTokens
	traceHandle, tracer, err := setupRunTracer(env.TaskID, runTraceOptions(cmd, watchQuiet)...)
	if err != nil {
		return err
	}
	defer func() { _ = traceHandle.Close() }()
	pipeline.SetTracer(tracer)
	pipeline.SetProgress(ui.NewProgress(cmd.ErrOrStderr(), ui.WithQuiet(watchQuiet), ui.WithColorizer(ui.NewColorizer(cmd.ErrOrStderr(), ui.WithNoColor(noColor))), ui.WithLogger(pawlog.From(ctx)), ui.WithStructured(pawlog.IsJSON(logFormat))))
	out, err := pipeline.RunLoop(ctx, env)
	if err != nil {
		return err
	}
	if runSucceeded(out) {
		return nil
	}
	if out.Verify != nil && !out.Verify.Passed {
		return verifyFailedErrorf("verification failed")
	}
	return verifyFailedErrorf("run stopped without done or verify pass")
}

func printWatchStatus(w io.Writer, status watchpkg.Status) {
	loc := fmt.Sprintf("%s:%d", status.Marker.DisplayPath, status.Marker.Line)
	switch status.State {
	case "found":
		_, _ = fmt.Fprintf(w, "found %s %q\n", loc, status.Marker.Text)
	case "running":
		_, _ = fmt.Fprintf(w, "running %s\n", loc)
	case "passed":
		_, _ = fmt.Fprintf(w, "passed %s\n", loc)
	case "removed":
		_, _ = fmt.Fprintf(w, "removed %s\n", loc)
	case "failed":
		_, _ = fmt.Fprintf(w, "failed %s: %v\n", loc, status.Err)
	}
}
