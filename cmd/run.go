package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/paw/internal/compress"
	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/edit"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/gather"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/plan"
	"github.com/gongahkia/paw/internal/stage"
	"github.com/gongahkia/paw/internal/verify"
	"github.com/spf13/cobra"
)

var (
	runInstruction     string
	runInstructionFile string
	runMaxTurns        int
	runRawContext      bool
	runDisableCompress bool
	runDroneModel      string
	runNoninteractive  bool
	runExplain         bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full agent pipeline",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if runExplain {
			cmd.Println("gather | compress | plan | edit | verify")
			return nil
		}
		if runNoninteractive || os.Getenv("PAW_NONINTERACTIVE") != "" {
			cmd.SilenceUsage = true
		}
		instruction, err := readInstruction()
		if err != nil {
			return err
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		if runMaxTurns > 0 {
			cfg.MaxTurns = runMaxTurns
		}
		if runDroneModel != "" {
			cfg.Drone.Model = runDroneModel
		}
		brain, err := llm.NewBrainClient(llmConfig(cfg))
		if err != nil {
			return err
		}
		var drone llm.Client
		if !runDisableCompress && !runRawContext {
			drone, err = llm.NewDroneClient(llmConfig(cfg))
			if err != nil {
				return err
			}
		}
		compressStage := compress.New(drone)
		compressStage.DisableCompress = runDisableCompress
		compressStage.RawContext = runRawContext
		planStage := plan.New(brain)
		planStage.UseRawContext = runRawContext
		editStage := edit.New(brain)
		editStage.UseRawContext = runRawContext
		pipeline, err := stage.NewPipeline(
			gather.New(cfg.Gather),
			compressStage,
			planStage,
			editStage,
			verify.New(""),
		)
		if err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		id := taskID(instruction, cwd)
		env := envelope.NewEnvelope(id, instruction, cwd)
		env.Budget.MaxTurns = cfg.MaxTurns
		env.Budget.MaxBrainTokens = cfg.MaxBrainTokens
		traceHandle, tracer, err := setupRunTracer(id)
		if err != nil {
			return err
		}
		defer func() { _ = traceHandle.Close() }()
		pipeline.SetTracer(tracer)
		out, err := pipeline.RunLoop(cmd.Context(), env)
		if err != nil {
			return err
		}
		if err := out.Marshal(cmd.OutOrStdout()); err != nil {
			return err
		}
		if runSucceeded(out) {
			return nil
		}
		return fmt.Errorf("run stopped without done or verify pass")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&runInstruction, "instruction", "", "task instruction")
	runCmd.Flags().StringVar(&runInstructionFile, "instruction-file", "", "task instruction file")
	runCmd.Flags().IntVar(&runMaxTurns, "max-turns", 0, "maximum agent turns")
	runCmd.Flags().BoolVar(&runRawContext, "raw-context", false, "bypass model compression")
	runCmd.Flags().BoolVar(&runDisableCompress, "disable-compress", false, "use deterministic compression fallback")
	runCmd.Flags().StringVar(&runDroneModel, "drone-model", "", "drone model override")
	runCmd.Flags().BoolVar(&runNoninteractive, "noninteractive", false, "disable interactive prompts")
	runCmd.Flags().BoolVar(&runExplain, "explain", false, "print composed pipeline")
}

func readInstruction() (string, error) {
	if runInstruction != "" && runInstructionFile != "" {
		return "", fmt.Errorf("use --instruction or --instruction-file, not both")
	}
	if runInstruction != "" {
		return runInstruction, nil
	}
	if runInstructionFile == "" {
		return "", fmt.Errorf("missing --instruction or --instruction-file")
	}
	data, err := os.ReadFile(runInstructionFile)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func llmConfig(cfg config.Config) llm.FactoryConfig {
	return llm.FactoryConfig{
		Brain: llm.EndpointConfig{
			Transport: cfg.Brain.Transport,
			BaseURL:   cfg.Brain.BaseURL,
			APIKey:    cfg.Brain.APIKey,
			Provider:  cfg.Brain.Provider,
			Model:     cfg.Brain.Model,
		},
		Drone: llm.EndpointConfig{
			Transport: cfg.Drone.Transport,
			BaseURL:   cfg.Drone.BaseURL,
			APIKey:    cfg.Drone.APIKey,
			Provider:  cfg.Drone.Provider,
			Model:     cfg.Drone.Model,
		},
		CallTimeout: cfg.CallTimeout,
	}
}

func taskID(instruction, cwd string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s", instruction, cwd)))
	return "task-" + hex.EncodeToString(sum[:])[:12]
}

func setupRunTracer(taskID string) (*os.File, *stage.Tracer, error) {
	path := traceFile
	if path == "" {
		path = filepath.Join(".paw", "trace-"+taskID+".ndjson")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, stage.NewTracer(file), nil
}

func runSucceeded(env *envelope.Envelope) bool {
	return env.Done || env.Verify != nil && env.Verify.Passed
}
