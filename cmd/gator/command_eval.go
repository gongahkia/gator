package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/eval"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func evalCommand(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("eval", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	runID := flags.String("run-id", "", "unique evaluation attempt id")
	reportPath := flags.String("report", "", "write the JSON report to this path")
	scriptPath := flags.String("script", "", "offline scripted turns JSON (default DIR/script.json)")
	live := flags.Bool("live", false, "use the configured cloud or local model instead of a script")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: gator eval DIR [--run-id ID] [--report PATH] [--script PATH] [--live]")
	}
	directory, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	spec, err := eval.LoadSpec(directory)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*runID) == "" {
		*runID = spec.ID + "-" + time.Now().UTC().Format("20060102T150405Z")
	}
	repository := filepath.Join(directory, "repository")
	if spec.Repository != "" {
		if filepath.IsAbs(spec.Repository) {
			repository = spec.Repository
		} else {
			repository = filepath.Join(directory, spec.Repository)
		}
	}
	workspace := filepath.Join(os.TempDir(), "gator-eval-"+*runID)
	if err := os.RemoveAll(workspace); err != nil {
		return err
	}
	if err := eval.CopyRepository(repository, workspace); err != nil {
		return fmt.Errorf("copy eval repository: %w", err)
	}
	executor := gatorrun.Executor{Sandbox: eval.PolicyFromSpec(spec)}
	providerName := "scripted"
	modelName := "scripted"
	if *live {
		defaults, err := configuredDefaults()
		if err != nil {
			return err
		}
		providerName = defaults.Provider
		modelName = defaults.Model
		liveExecutor, err := newExecutor(providerName, modelName, os.Getenv("GATOR_BASE_URL"))
		if err != nil {
			return err
		}
		liveExecutor.Sandbox = eval.PolicyFromSpec(spec)
		executor = liveExecutor
	} else {
		scriptFile := strings.TrimSpace(*scriptPath)
		if scriptFile == "" {
			scriptFile = filepath.Join(directory, "script.json")
		}
		turns, err := eval.LoadTurns(scriptFile)
		if err != nil {
			return err
		}
		if len(turns) == 0 {
			return errors.New("offline eval requires DIR/script.json or --script; use --live for a configured model")
		}
		executor.Model = &eval.ScriptedModel{Turns: turns}
	}
	stateDir := os.Getenv("GATOR_STATE_DIR")
	if stateDir == "" {
		stateDir = filepath.Join(os.TempDir(), "gator-eval-state-"+*runID)
	}
	report, err := eval.Run(context.Background(), eval.Options{
		Spec:       spec,
		RunID:      *runID,
		Provider:   providerName,
		ModelName:  modelName,
		StateDir:   stateDir,
		Repository: workspace,
		Executor:   executor,
	})
	if err != nil {
		return err
	}
	if *reportPath == "" {
		*reportPath = filepath.Join(directory, "reports", *runID+".json")
	}
	if err := eval.WriteReport(*reportPath, report); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "eval %s status=%s run_id=%s report=%s\n", report.ID, report.Status, report.RunID, *reportPath)
	return err
}
