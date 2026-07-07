package cmd

import (
	"github.com/gongahkia/paw/internal/config"
	pawlog "github.com/gongahkia/paw/internal/log"
	"github.com/gongahkia/paw/internal/stage"
	"github.com/gongahkia/paw/internal/ui"
	"github.com/spf13/cobra"
)

var (
	resumeMaxTurns        int
	resumeRawContext      bool
	resumeDisableCompress bool
	resumeDroneModel      string
	resumeQuiet           bool
)

var resumeCmd = &cobra.Command{
	Use:   "resume <task-id>",
	Short: "Resume a run from its trace",
	Example: `  paw resume task-abc123
  paw resume task-abc123 --trace-file .paw/trace-task-abc123.ndjson`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return usageErrorf("expected task id")
		}
		path := runTracePath(args[0])
		env, err := stage.ReadTrace(path)
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		if resumeMaxTurns > 0 {
			env.Budget.MaxTurns = resumeMaxTurns
		}
		if resumeDroneModel != "" {
			cfg.Drone.Model = resumeDroneModel
		}
		pipeline, err := newAgentPipeline(cfg, resumeRawContext, resumeDisableCompress)
		if err != nil {
			return err
		}
		traceHandle, tracer, err := setupAppendTracer(path, runTraceOptions(cmd, resumeQuiet)...)
		if err != nil {
			return err
		}
		defer func() { _ = traceHandle.Close() }()
		pipeline.SetTracer(tracer)
		pipeline.SetProgress(ui.NewProgress(cmd.ErrOrStderr(), ui.WithQuiet(resumeQuiet), ui.WithColorizer(ui.NewColorizer(cmd.ErrOrStderr(), ui.WithNoColor(noColor))), ui.WithLogger(pawlog.From(cmd.Context())), ui.WithStructured(pawlog.IsJSON(logFormat))))
		out, err := pipeline.RunFrom(cmd.Context(), env)
		if err != nil {
			return err
		}
		return writeRunResult(cmd, out)
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().IntVar(&resumeMaxTurns, "max-turns", 0, "maximum agent turns")
	resumeCmd.Flags().BoolVar(&resumeRawContext, "raw-context", false, "bypass model compression")
	resumeCmd.Flags().BoolVar(&resumeDisableCompress, "disable-compress", false, "use deterministic compression fallback")
	resumeCmd.Flags().StringVar(&resumeDroneModel, "drone-model", "", "drone model override")
	resumeCmd.Flags().BoolVar(&resumeQuiet, "quiet", false, "suppress progress output")
}
