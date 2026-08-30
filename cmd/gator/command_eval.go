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
	"github.com/gongahkia/gator/internal/extension"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

const evalUsage = "usage: gator eval DIR [--run-id ID] [--report PATH] [--script PATH] [--live] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--require-resolved]"

type evalFlags struct {
	runID           string
	reportPath      string
	reportDirectory string
	scriptPath      string
	provider        string
	model           string
	baseURL         string
	live            bool
	requireResolved bool
}

type evalRuntime struct {
	live     bool
	provider string
	model    string
	executor gatorrun.Executor
}

func evalCommand(arguments []string, out io.Writer) error {
	if len(arguments) > 0 && arguments[0] == "suite" {
		return evalSuiteCommand(arguments[1:], out)
	}
	return evalFixtureCommand(arguments, out)
}

func evalFixtureCommand(arguments []string, out io.Writer) error {
	directory, options, err := parseEvalArguments("eval", arguments, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.reportDirectory) != "" {
		return errors.New("--report-dir is available only with 'gator eval suite'")
	}
	if options.live && strings.TrimSpace(options.scriptPath) != "" {
		return errors.New("--script and --live cannot be used together")
	}
	runtime, err := newEvalRuntime(options)
	if err != nil {
		return err
	}
	stateDir, err := evaluationStateDir()
	if err != nil {
		return err
	}
	report, err := runEvalFixture(context.Background(), directory, options, runtime, stateDir, "")
	if err != nil {
		return err
	}
	reportPath := options.reportPath
	if strings.TrimSpace(reportPath) == "" {
		reportPath = filepath.Join(directory, "reports", report.RunID+".json")
	}
	if err := eval.WriteReport(reportPath, report); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "eval %s status=%s run_id=%s report=%s\n", report.ID, report.Status, report.RunID, reportPath); err != nil {
		return err
	}
	if options.requireResolved && report.Status != "resolved" {
		return fmt.Errorf("eval %s status=%s; report was written to %s", report.ID, report.Status, reportPath)
	}
	return nil
}

func evalSuiteCommand(arguments []string, out io.Writer) error {
	directory, options, err := parseEvalArguments("eval suite", arguments, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.reportPath) != "" {
		return errors.New("--report is available only with 'gator eval DIR'; use --report-dir for a suite")
	}
	if strings.TrimSpace(options.scriptPath) != "" {
		return errors.New("--script is available only with 'gator eval DIR'; suite fixtures use their own script.json")
	}
	if !options.live {
		return errors.New("suite evaluation requires --live; offline scripts exercise harness mechanics, not model quality")
	}
	suite, err := eval.LoadSuite(directory)
	if err != nil {
		return err
	}
	runtime, err := newEvalRuntime(options)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.runID) == "" {
		options.runID = suite.ID + "-" + time.Now().UTC().Format("20060102T150405Z")
	}
	if len(options.runID) > 92 {
		return errors.New("suite --run-id must contain at most 92 characters so case run IDs remain portable")
	}
	if err := eval.ValidateRunID(options.runID); err != nil {
		return err
	}
	if strings.TrimSpace(options.reportDirectory) == "" {
		options.reportDirectory = filepath.Join(directory, "reports", options.runID)
	}
	stateDir, err := evaluationStateDir()
	if err != nil {
		return err
	}
	startedAt := time.Now()
	reports := make([]eval.Report, 0, len(suite.Cases))
	seenIDs := make(map[string]struct{}, len(suite.Cases))
	for index, fixture := range suite.Cases {
		caseDirectory := filepath.Join(directory, fixture)
		caseOptions := options
		caseOptions.runID = fmt.Sprintf("%s-%02d", options.runID, index+1)
		report, err := runEvalFixture(context.Background(), caseDirectory, caseOptions, runtime, stateDir, "")
		if err != nil {
			return err
		}
		if _, exists := seenIDs[report.ID]; exists {
			return fmt.Errorf("eval suite repeats fixture id %q", report.ID)
		}
		seenIDs[report.ID] = struct{}{}
		caseReportPath := filepath.Join(options.reportDirectory, report.ID+".json")
		if err := eval.WriteReport(caseReportPath, report); err != nil {
			return err
		}
		reports = append(reports, report)
	}
	summary := eval.SummarizeSuite(suite, options.runID, runtime.provider, runtime.model, startedAt, time.Now(), reports)
	summaryPath := filepath.Join(options.reportDirectory, "suite.json")
	if err := eval.WriteSuiteReport(summaryPath, summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "eval suite %s status=%s resolved=%d unresolved=%d errors=%d report=%s\n", summary.ID, summary.Status, summary.Resolved, summary.Unresolved, summary.Errors, summaryPath); err != nil {
		return err
	}
	if options.requireResolved && summary.Status != "resolved" {
		return fmt.Errorf("eval suite %s status=%s; report was written to %s", summary.ID, summary.Status, summaryPath)
	}
	return nil
}

func parseEvalArguments(name string, arguments []string, suite bool) (string, evalFlags, error) {
	var options evalFlags
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.runID, "run-id", "", "unique evaluation attempt id")
	flags.StringVar(&options.reportPath, "report", "", "write the JSON report to this path")
	flags.StringVar(&options.reportDirectory, "report-dir", "", "write suite reports to this directory")
	flags.StringVar(&options.scriptPath, "script", "", "offline scripted turns JSON")
	flags.StringVar(&options.provider, "provider", "", "provider for a live evaluation")
	flags.StringVar(&options.model, "model", "", "model for a live evaluation")
	flags.StringVar(&options.baseURL, "base-url", "", "provider base URL for a live evaluation")
	flags.BoolVar(&options.live, "live", false, "use a real provider")
	flags.BoolVar(&options.requireResolved, "require-resolved", false, "exit nonzero after writing reports unless every run resolved")
	target, flagArguments, err := splitEvalArguments(arguments)
	if err != nil {
		return "", evalFlags{}, err
	}
	if err := flags.Parse(flagArguments); err != nil {
		return "", evalFlags{}, err
	}
	if strings.TrimSpace(target) == "" {
		if suite {
			return "", evalFlags{}, errors.New("usage: gator eval suite DIR [--run-id ID] [--report-dir DIR] --live [--provider PROVIDER] [--model MODEL] [--base-url URL] [--require-resolved]")
		}
		return "", evalFlags{}, errors.New(evalUsage)
	}
	directory, err := filepath.Abs(target)
	if err != nil {
		return "", evalFlags{}, err
	}
	return directory, options, nil
}

// splitEvalArguments intentionally accepts flags on either side of DIR. The
// standard flag package stops parsing at the first positional argument, while
// documented headless invocations conventionally put the fixture first.
func splitEvalArguments(arguments []string) (string, []string, error) {
	valueFlags := map[string]bool{
		"--run-id": true, "--report": true, "--report-dir": true, "--script": true,
		"--provider": true, "--model": true, "--base-url": true,
	}
	booleanFlags := map[string]bool{"--live": true, "--require-resolved": true}
	var target string
	var flagArguments []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if strings.HasPrefix(argument, "-") {
			name, _, hasValue := strings.Cut(argument, "=")
			if valueFlags[name] {
				if hasValue {
					flagArguments = append(flagArguments, argument)
					continue
				}
				if index+1 == len(arguments) {
					return "", nil, fmt.Errorf("eval option %s requires a value", name)
				}
				flagArguments = append(flagArguments, argument, arguments[index+1])
				index++
				continue
			}
			if booleanFlags[name] {
				flagArguments = append(flagArguments, argument)
				continue
			}
			return "", nil, fmt.Errorf("unknown eval option %q", name)
		}
		if target != "" {
			return "", nil, errors.New("eval accepts exactly one fixture directory")
		}
		target = argument
	}
	return target, flagArguments, nil
}

func newEvalRuntime(options evalFlags) (evalRuntime, error) {
	if !options.live {
		if strings.TrimSpace(options.provider) != "" || strings.TrimSpace(options.model) != "" || strings.TrimSpace(options.baseURL) != "" {
			return evalRuntime{}, errors.New("--provider, --model, and --base-url require --live")
		}
		return evalRuntime{provider: "scripted", model: "scripted"}, nil
	}
	providerName := strings.TrimSpace(options.provider)
	if providerName == "" {
		var err error
		providerName, err = providerNameFromEnvironment()
		if err != nil {
			return evalRuntime{}, err
		}
	}
	providerName, defaultModel, err := resolveConfiguredProvider(providerName, "")
	if err != nil {
		return evalRuntime{}, fmt.Errorf("live eval provider: %w", err)
	}
	modelName := strings.TrimSpace(options.model)
	if modelName == "" {
		modelName = strings.TrimSpace(os.Getenv("GATOR_MODEL"))
	}
	if modelName == "" {
		modelName = defaultModel
	}
	if modelName == "" {
		return evalRuntime{}, errors.New("live evaluation requires --model or a configured provider default")
	}
	baseURL := strings.TrimSpace(options.baseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("GATOR_BASE_URL"))
	}
	executor, err := newExecutor(providerName, modelName, baseURL)
	if err != nil {
		return evalRuntime{}, err
	}
	// A fresh fixture has no repository trust identity, but global extensions
	// still would. Evaluation deliberately excludes every optional integration.
	executor.Extensions = extension.Resolver{}
	executor.HookTrusts = nil
	executor.LSPTrusts = nil
	executor.MCPTrusts = nil
	return evalRuntime{live: true, provider: providerName, model: modelName, executor: executor}, nil
}

func runEvalFixture(ctx context.Context, directory string, options evalFlags, runtime evalRuntime, stateDir, scriptOverride string) (eval.Report, error) {
	spec, err := eval.LoadSpec(directory)
	if err != nil {
		return eval.Report{}, err
	}
	runID := options.runID
	if strings.TrimSpace(runID) == "" {
		runID = spec.ID + "-" + time.Now().UTC().Format("20060102T150405Z")
	}
	if err := eval.ValidateRunID(runID); err != nil {
		return eval.Report{}, err
	}
	source, err := evalRepositoryPath(directory, spec)
	if err != nil {
		return eval.Report{}, err
	}
	repository, err := eval.PrepareRepository(ctx, source)
	if err != nil {
		return eval.Report{}, err
	}
	executor := runtime.executor
	executor.Sandbox = eval.PolicyFromSpec(spec)
	if !runtime.live {
		scriptPath := scriptOverride
		if strings.TrimSpace(scriptPath) == "" {
			scriptPath = options.scriptPath
		}
		if strings.TrimSpace(scriptPath) == "" {
			scriptPath = filepath.Join(directory, "script.json")
		}
		turns, err := eval.LoadTurns(scriptPath)
		if err != nil {
			return eval.Report{}, err
		}
		if len(turns) == 0 {
			return eval.Report{}, errors.New("offline eval requires DIR/script.json or --script; use --live for a configured model")
		}
		executor.Model = &eval.ScriptedModel{Turns: turns}
	}
	return eval.Run(ctx, eval.Options{
		Spec:       spec,
		RunID:      runID,
		Provider:   runtime.provider,
		ModelName:  runtime.model,
		StateDir:   stateDir,
		Repository: repository,
		Executor:   executor,
	})
}

func evalRepositoryPath(directory string, spec eval.Spec) (string, error) {
	repository := "repository"
	if strings.TrimSpace(spec.Repository) != "" {
		repository = spec.Repository
	}
	clean := filepath.Clean(repository)
	if filepath.IsAbs(repository) || (clean != "." && (clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)))) {
		return "", errors.New("eval repository must be inside the fixture directory")
	}
	return filepath.Join(directory, clean), nil
}

func evaluationStateDir() (string, error) {
	if directory := strings.TrimSpace(os.Getenv("GATOR_STATE_DIR")); directory != "" {
		return directory, nil
	}
	directory, err := os.MkdirTemp("", "gator-eval-state-")
	if err != nil {
		return "", fmt.Errorf("create eval state directory: %w", err)
	}
	return directory, nil
}
