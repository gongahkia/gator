package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
)

type watchReplayOptions struct {
	ReportDir, Provider, Model string
	Live                       bool
	MaxRequests                int
}

// The factory permits the same replay to use deterministic test models or a
// deliberately selected provider. There is no live-provider fallback.
type watchServiceFactory func(eval.WatchEpisode, string, *workrun.Request) (workrun.Service, error)

func workWatchEvalCommand(args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: work eval watch validate|run|score DATASET_OR_REPORT [--report-dir NEW_DIR] [--reviews REVIEWS_JSON]")
	}
	operation, path := args[0], args[1]
	if operation == "score" {
		flags := flag.NewFlagSet("watch score", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		reviewsPath := flags.String("reviews", "", "explicit human or fixture review JSON")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if len(flags.Args()) != 0 {
			return errors.New("unexpected watch score arguments")
		}
		report, err := eval.LoadWatchReport(path)
		if err != nil {
			return err
		}
		var reviews eval.WatchReviews
		if *reviewsPath != "" {
			reviews, err = eval.LoadWatchReviews(*reviewsPath)
			if err != nil {
				return err
			}
		}
		score, err := eval.ScoreWatch(report, reviews)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(score)
	}
	dataset, err := eval.LoadWatchDataset(path)
	if err != nil {
		return err
	}
	if operation == "validate" {
		if len(args) != 2 {
			return errors.New("watch validate takes only a dataset")
		}
		for _, episode := range dataset.Episodes {
			if _, err := eval.ReadWatchSource(path, episode); err != nil {
				return fmt.Errorf("episode %s: %w", episode.ID, err)
			}
		}
		_, err = fmt.Fprintf(out, "%s: %d sequential episodes, %d material events\n", dataset.ID, len(dataset.Episodes), len(dataset.Events))
		return err
	}
	if operation != "run" {
		return errors.New("unknown watch evaluation operation")
	}
	flags := flag.NewFlagSet("watch run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	directory := flags.String("report-dir", "", "new isolated report directory")
	live := flags.Bool("live", false, "use the explicitly selected provider for ordinary episodes")
	provider := flags.String("provider", "", "live provider")
	model := flags.String("model", "", "live model")
	requests := flags.Int("max-model-requests", 0, "total request budget across all episodes and retries")
	seconds := flags.Int("timeout-seconds", 0, "total wall budget")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 || *directory == "" {
		return errors.New("watch run requires --report-dir and no extra arguments")
	}
	if *live && (*provider == "" || *model == "" || *requests < 1 || *seconds < 1) {
		return errors.New("live watch replay requires provider, model, max-model-requests and timeout-seconds")
	}
	if !*live && (*provider != "" || *model != "") {
		return errors.New("provider/model require explicit --live")
	}
	ctx := context.Background()
	if *seconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*seconds)*time.Second)
		defer cancel()
	}
	var factory watchServiceFactory
	if *live {
		factory = func(_ eval.WatchEpisode, state string, request *workrun.Request) (workrun.Service, error) {
			return configuredWorkService(*provider, *model, state, request)
		}
	}
	report, err := runWatchReplay(ctx, path, dataset, watchReplayOptions{ReportDir: *directory, Provider: *provider, Model: *model, Live: *live, MaxRequests: *requests}, factory)
	if err != nil {
		return err
	}
	score, err := eval.ScoreWatch(report, eval.WatchReviews{})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s: %d episodes, %d change alerts, %d health notices; %d alerts await review. Mode: %s. Report: %s\n", dataset.ID, len(report.Observations), score.Alerts, score.HealthNotices, score.UnreviewedAlerts, report.Mode, filepath.Join(*directory, "watch-report.json"))
	return err
}

func runWatchReplay(ctx context.Context, datasetPath string, dataset eval.WatchDataset, options watchReplayOptions, factory watchServiceFactory) (eval.WatchReport, error) {
	var report eval.WatchReport
	if err := dataset.Validate(); err != nil {
		return report, err
	}
	if options.Live && factory == nil {
		return report, errors.New("live replay requires an explicit model factory")
	}
	if options.ReportDir == "" {
		return report, errors.New("watch report directory is required")
	}
	dir, err := filepath.Abs(options.ReportDir)
	if err != nil {
		return report, err
	}
	// A pre-existing directory is never reused, even when it looks empty.
	if err := os.Mkdir(dir, 0700); err != nil {
		return report, fmt.Errorf("create fresh watch report: %w", err)
	}
	state, source := filepath.Join(dir, "state"), filepath.Join(dir, "workspace")
	if err := os.Mkdir(source, 0700); err != nil {
		return report, err
	}
	var identity [12]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return report, err
	}
	report = eval.WatchReport{Version: 1, ID: "watch-" + hex.EncodeToString(identity[:]), Dataset: dataset, Mode: "scripted", Provider: options.Provider, Model: options.Model, Harness: commit, StartedAt: time.Now().UTC()}
	if options.Live {
		report.Mode = "live"
	}
	goals, err := goal.Open(state)
	if err != nil {
		return report, err
	}
	g, err := goals.Add(dataset.ID, dataset.Outcome, source)
	if err != nil {
		return report, err
	}
	draft, err := followup.New(source, g.ID)
	if err != nil {
		return report, err
	}
	draft.Title, draft.Outcome, draft.AlertWhen, draft.Mode = dataset.ID, dataset.Outcome, dataset.AlertWhen, action.Inspect
	draft.MaxChecksPerDay = 288
	draft.OriginConversationID = report.ID
	definition, err := draft.Definition(g.ID, nil)
	if err != nil {
		return report, err
	}
	store, err := jobs.Open(state)
	if err != nil {
		return report, err
	}
	definition, err = store.Save(definition)
	if err != nil {
		return report, err
	}
	var budget *agent.Budget
	if options.MaxRequests > 0 {
		budget = &agent.Budget{Limits: agent.Limits{ModelRequests: options.MaxRequests}}
	}
	if err := eval.SaveWatchReport(filepath.Join(dir, "watch-report.json"), report); err != nil {
		return report, err
	}
	for _, episode := range dataset.Episodes {
		if err := ctx.Err(); err != nil {
			report.StopReason = err.Error()
			_ = eval.SaveWatchReport(filepath.Join(dir, "watch-report.json"), report)
			return report, err
		}
		data, err := eval.ReadWatchSource(datasetPath, episode)
		if err != nil {
			return report, fmt.Errorf("episode %s: %w", episode.ID, err)
		}
		if err := os.WriteFile(filepath.Join(source, "project.md"), data, 0600); err != nil {
			return report, err
		}
		sourceHash := sha256.Sum256(data)
		attempt, err := store.Begin(definition, report.StartedAt.Add(time.Duration(episode.AtSecond)*time.Second))
		if err != nil {
			return report, err
		}
		observation := eval.WatchObservation{EpisodeID: episode.ID, AtSecond: episode.AtSecond, SourceSHA256: hex.EncodeToString(sourceHash[:]), FaultInjected: episode.Fault}
		var modelCalls, providerErrors, toolErrors atomic.Int32
		var executionErr error
		execute := func(checkCtx context.Context, d jobs.Definition, root string, runIDs ...string) (jobResult, error) {
			runCtx, cancel := context.WithCancel(checkCtx)
			defer cancel()
			result, err := runJobProcessWith(runCtx, d, root, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
				request.Budget = budget
				request.DisableDelegation = true
				service := workrun.Service{Executor: workrun.Executor{StateDir: root, Model: &eval.ScriptedModel{Turns: append([]agent.Turn(nil), episode.Turns...)}}}
				if factory != nil && episode.Fault == "" {
					var err error
					service, err = factory(episode, root, &request)
					if err != nil {
						return workrun.Outcome{}, err
					}
				}
				if episode.Fault == "provider" {
					service.Executor.Model = watchFailureModel{}
				}
				if episode.Fault == "tool" {
					service.Executor.Model = &eval.ScriptedModel{Turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "injected-read", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/missing-fixture-file.md"}`)}}}}}
				}
				if service.Executor.Model == nil {
					return workrun.Outcome{}, errors.New("watch model factory returned no model")
				}
				service.Executor.Model = watchObservedModel{Model: service.Executor.Model, Calls: &modelCalls, Errors: &providerErrors}
				request.OnEvent = func(event agent.Event) {
					if event.Kind == agent.EventToolFinished && event.ToolError != "" {
						toolErrors.Add(1)
						if episode.Fault == "tool" {
							cancel()
						}
					}
				}
				return service.Execute(ctx, request)
			}, runIDs...)
			executionErr = err
			return result, err
		}
		if episode.Fault == "source" {
			if err := os.Rename(source, source+"-unavailable"); err != nil {
				return report, err
			}
		}
		attempt = executeJobContextWith(ctx, definition, attempt, state, false, execute)
		if episode.Fault == "source" {
			if err := os.Rename(source+"-unavailable", source); err != nil {
				return report, err
			}
		}
		if err := store.Record(attempt); err != nil {
			return report, err
		}
		entry, err := recordJobInbox(state, definition, attempt)
		if err != nil {
			return report, err
		}
		observation.Attempt, observation.InboxID = attempt, entry.ID
		observation.Alert = entry.ID != "" && attempt.Status == "completed"
		observation.HealthNotice = entry.ID != "" && attempt.Status != "completed"
		observation.ModelCalls, observation.ProviderErrors, observation.ToolErrors = int(modelCalls.Load()), int(providerErrors.Load()), int(toolErrors.Load())
		if attempt.Status != "completed" && attempt.Status != "unchanged" {
			switch {
			case episode.Fault != "":
				observation.FailureCategory = episode.Fault
			case errors.Is(executionErr, agent.ErrBudget):
				observation.FailureCategory = "budget"
			case errors.Is(executionErr, context.DeadlineExceeded):
				observation.FailureCategory = "timeout"
			case errors.Is(executionErr, context.Canceled):
				observation.FailureCategory = "cancelled"
			case observation.ProviderErrors > 0:
				observation.FailureCategory = "provider"
			case observation.ToolErrors > 0:
				observation.FailureCategory = "tool"
			case attempt.Status == "limited":
				observation.FailureCategory = "limit"
			default:
				observation.FailureCategory = "execution"
			}
		}
		observation.OutputSHA256 = observation.OutputDigest()
		report.Observations = append(report.Observations, observation)
		if err := eval.SaveWatchReport(filepath.Join(dir, "watch-report.json"), report); err != nil {
			return report, err
		}
	}
	report.Complete = true
	if err := eval.SaveWatchReport(filepath.Join(dir, "watch-report.json"), report); err != nil {
		return report, err
	}
	return report, nil
}

type watchFailureModel struct{}

func (watchFailureModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	return agent.Turn{}, errors.New("injected permanent provider failure")
}

type watchObservedModel struct {
	Model         agent.Model
	Calls, Errors *atomic.Int32
}

func (m watchObservedModel) Complete(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.Calls.Add(1)
	turn, err := m.Model.Complete(ctx, request)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		m.Errors.Add(1)
	}
	return turn, err
}
