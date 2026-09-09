package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/eval"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/workrun"
)

func workEvalCommand(args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: gator eval work validate|run|show|compare DATASET_OR_REPORT [options]")
	}
	action, path := args[0], args[1]
	if action == "export-langsmith" {
		report, err := eval.LoadWorkExperiment(path)
		if err != nil {
			return err
		}
		endpoint := os.Getenv("LANGSMITH_ENDPOINT")
		if endpoint == "" {
			endpoint = "https://api.smith.langchain.com/api/v1"
		}
		return eval.ExportLangSmith(context.Background(), http.DefaultClient, endpoint, os.Getenv("LANGSMITH_API_KEY"), report)
	}
	if action == "show" {
		report, err := eval.LoadWorkExperiment(path)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s (%s): %d/%d trials pass; at least one succeeds on %d cases; all trials succeed on %d cases.\nCategories: %v\n", report.ID, report.Target, report.Passed, report.Total, report.CasesAtLeastOne, report.CasesAll, report.Categories)
		return err
	}
	if action == "compare" {
		if len(args) != 3 {
			return errors.New("compare requires two experiment.json paths")
		}
		a, err := eval.LoadWorkExperiment(path)
		if err != nil {
			return err
		}
		b, err := eval.LoadWorkExperiment(args[2])
		if err != nil {
			return err
		}
		comparison, err := eval.CompareWork(a, b)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, comparison)
		return err
	}
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		return err
	}
	if action == "validate" {
		_, err = fmt.Fprintf(out, "%s: %d valid %s cases\n", dataset.ID, len(dataset.Cases), dataset.Target)
		return err
	}
	if action != "run" {
		return errors.New("unknown Work evaluation operation")
	}
	flags := flag.NewFlagSet("eval work run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	reportDir := flags.String("report-dir", "", "new output directory")
	id := flags.String("id", "work-eval", "experiment ID")
	attempts := flags.Int("attempts", 1, "independent trials per case (1-10)")
	delegation := flags.Bool("delegation", true, "enable specialists")
	live := flags.Bool("live", false, "use real provider models")
	provider := flags.String("provider", "", "live provider")
	model := flags.String("model", "", "live model")
	requests := flags.Int("max-model-requests", 0, "total live model request budget")
	seconds := flags.Int("timeout-seconds", 0, "total live wall budget")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if *reportDir == "" {
		return errors.New("--report-dir is required")
	}
	if *live && (*requests < 1 || *seconds < 1 || *provider == "" || *model == "") {
		return errors.New("live Work eval requires explicit provider, model, max-model-requests and timeout-seconds")
	}
	absolute, err := filepath.Abs(*reportDir)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if *seconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*seconds)*time.Second)
		defer cancel()
	}
	options := eval.WorkEvalOptions{ID: *id, Harness: commit, Provider: *provider, Model: *model, ReportDir: absolute, Attempts: *attempts, Delegation: *delegation, Live: *live, MaxRequests: *requests}
	report, err := eval.RunWorkExperiment(ctx, path, dataset, options, func(c eval.WorkCase, state string) (workrun.Service, error) {
		if *live {
			request := workrun.Request{}
			return configuredWorkService(*provider, *model, state, &request)
		}
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s: %d/%d trials pass; categories %v. Report: %s\n", report.ID, report.Passed, report.Total, report.Categories, filepath.Join(absolute, "experiment.json"))
	if err != nil {
		return err
	}
	if report.Passed != report.Total {
		return errors.New("Work evaluation has failing trials")
	}
	return nil
}
func scriptedWorkService(c eval.WorkCase, state string) workrun.Service {
	replace := func(turns []agent.Turn) []agent.Turn {
		data, _ := json.Marshal(turns)
		data = []byte(strings.ReplaceAll(string(data), "{{run}}", c.ID))
		var copy []agent.Turn
		_ = json.Unmarshal(data, &copy)
		return copy
	}
	executor := workrun.Executor{Model: &eval.ScriptedModel{Turns: replace(c.Turns)}, StateDir: state, RoleModels: map[string]agent.Model{}}
	for role, turns := range c.RoleTurns {
		executor.RoleModels[role] = &eval.ScriptedModel{Turns: replace(turns)}
	}
	executor.Code = func(ctx context.Context, request workrun.CodeRequest) (workrun.CodeResult, error) {
		turns := c.CodeTurns
		if specific, ok := c.RoleTurns["code."+request.Task]; ok {
			turns = specific
		}
		backend := &nativeWorkBackend{code: gatorrun.Executor{Model: &eval.ScriptedModel{Turns: replace(turns)}}, provider: "scripted", model: "fixture"}
		return backend.codeDelegate(state)(ctx, request)
	}
	return workrun.Service{Executor: executor}
}
