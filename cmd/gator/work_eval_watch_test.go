package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
)

type watchErrorModel struct{ err error }

func (m watchErrorModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	return agent.Turn{}, m.err
}

func TestWatchObservedModelSeparatesCancellationFromProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int32
	}{
		{"success", nil, 0},
		{"cancelled", context.Canceled, 0},
		{"deadline", context.DeadlineExceeded, 0},
		{"provider", errors.New("provider unavailable"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls, failures atomic.Int32
			model := watchObservedModel{Model: watchErrorModel{err: tc.err}, Calls: &calls, Errors: &failures}
			_, err := model.Complete(context.Background(), agent.TurnRequest{})
			if err != tc.err || calls.Load() != 1 || failures.Load() != tc.want {
				t.Fatalf("wrong error attribution: err=%v calls=%d failures=%d", err, calls.Load(), failures.Load())
			}
		})
	}
}

func TestWatchReplayRunsRealAttemptsAndSeparatesFaultsFromQuality(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	path := "../../examples/project-followup/episodes.json"
	dataset, err := eval.LoadWatchDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "replay")
	report, err := runWatchReplay(context.Background(), path, dataset, watchReplayOptions{ReportDir: directory}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || len(report.Observations) != 10 {
		t.Fatalf("incomplete replay: %+v", report)
	}
	byID := map[string]eval.WatchObservation{}
	for _, o := range report.Observations {
		byID[o.EpisodeID] = o
	}
	if byID["unchanged"].Attempt.Status != "unchanged" || byID["unchanged"].ModelCalls != 0 || byID["unchanged"].InboxID != "" {
		t.Fatalf("unchanged replay bypassed production gate: %+v", byID["unchanged"])
	}
	for _, kind := range []string{"source", "provider", "tool"} {
		failed := byID[kind+"-failure"]
		if failed.FailureCategory != kind || !failed.HealthNotice || failed.Alert || failed.Attempt.Status != "failed" {
			t.Fatalf("%s failure misreported: %+v", kind, failed)
		}
		if !byID[kind+"-recovery"].Alert {
			t.Fatalf("%s did not recover: %+v", kind, byID[kind+"-recovery"])
		}
	}
	if byID["tool-failure"].ToolErrors != 1 || byID["provider-failure"].ProviderErrors != 1 || byID["source-failure"].ModelCalls != 0 {
		t.Fatal("faults did not exercise their actual source/model/tool boundaries")
	}
	state := filepath.Join(directory, "state")
	store, _ := jobs.Open(state)
	history, err := store.History(report.Observations[0].Attempt.JobID, 0)
	if err != nil || len(history) != 10 {
		t.Fatalf("attempts not retained: %d %v", len(history), err)
	}
	if err := store.Record(report.Observations[0].Attempt); err == nil {
		t.Fatal("replay rewrote a terminal attempt")
	}
	mailbox, _ := inbox.Open(state)
	entries, err := mailbox.List(false, 0)
	if err != nil || len(entries) != 10 {
		t.Fatalf("expected four alerts, three health notices and three recovery notices: %d %v", len(entries), err)
	}
	for _, entry := range entries {
		if entry.OriginConversationID != report.ID {
			t.Fatal("replay lost origin-chat routing")
		}
	}
	files, err := os.ReadDir(filepath.Join(directory, "workspace"))
	if err != nil || len(files) != 1 || files[0].Name() != "project.md" {
		t.Fatalf("future sources/ground truth leaked into workspace: %v %v", files, err)
	}
	score, err := eval.ScoreWatch(report, eval.WatchReviews{})
	if err != nil || score.Alerts != 4 || score.HealthNotices != 3 || score.RecoveryNotices != 3 || score.UsefulAlertPrecision.Value != nil || len(score.UnassessedMaterialEvents) != 4 {
		t.Fatalf("unreviewed scripted outputs claimed quality: %+v %v", score, err)
	}
	reviews := eval.WatchReviews{Version: 1, ReportID: report.ID, Evidence: "scripted_fixture", Reviewer: "test"}
	for episode, event := range map[string]string{"deadline": "deadline-earlier", "provider-recovery": "owner-reassigned", "source-recovery": "access-confirmed", "tool-recovery": "capacity-reduced"} {
		o := byID[episode]
		yes := true
		reviews.Items = append(reviews.Items, eval.WatchReview{EpisodeID: episode, AttemptID: o.Attempt.ID, OutputSHA256: o.OutputSHA256, ReportedEventIDs: []string{event}, Useful: &yes, Actionable: &yes, Note: "Synthetic script matches the named event in the current fixture; not live quality evidence."})
	}
	score, err = eval.ScoreWatch(report, reviews)
	if err != nil || score.UsefulAlertPrecision.Numerator != 4 || score.UsefulAlertPrecision.Denominator != 4 || score.MissedMaterialChanges.Numerator != 0 || score.MissedMaterialChanges.Denominator != 4 || score.DetectionDelaySeconds.Numerator != 1300 || score.DetectionDelaySeconds.Denominator != 4 {
		t.Fatalf("incorrect explicit fixture score: %+v %v", score, err)
	}
	loaded, err := eval.LoadWatchReport(filepath.Join(directory, "watch-report.json"))
	if err != nil || loaded.ID != report.ID || !loaded.Complete {
		t.Fatalf("report did not survive reopening: %+v %v", loaded, err)
	}
}

func TestWatchReplayAcceptsInjectedProviderAndProtectsExistingDirectories(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	path := "../../examples/project-followup/episodes.json"
	dataset, _ := eval.LoadWatchDataset(path)
	dataset.Episodes, dataset.Events = dataset.Episodes[:2], dataset.Events[:1]
	dataset.Events[0].DeadlineSecond = 600
	calls := 0
	factory := func(episode eval.WatchEpisode, state string, request *workrun.Request) (workrun.Service, error) {
		if request.GoalID == "" || request.SourcePath == "" || request.RunID == "" || request.Budget == nil {
			t.Fatal("provider factory did not receive the actual admitted watch request")
		}
		calls++
		return workrun.Service{Executor: workrun.Executor{StateDir: state, Model: &eval.ScriptedModel{Turns: append([]agent.Turn(nil), episode.Turns...)}}}, nil
	}
	directory := filepath.Join(t.TempDir(), "fresh")
	report, err := runWatchReplay(context.Background(), path, dataset, watchReplayOptions{ReportDir: directory, Live: true, MaxRequests: 10}, factory)
	if err != nil || calls != 2 || report.Mode != "live" {
		t.Fatalf("injected provider was not exercised: calls=%d err=%v", calls, err)
	}
	if _, err := runWatchReplay(context.Background(), path, dataset, watchReplayOptions{ReportDir: directory}, nil); err == nil {
		t.Fatal("reused an existing report/state directory")
	}
	var out bytes.Buffer
	if err := workEvalCommand([]string{"watch", "run", path, "--report-dir", filepath.Join(t.TempDir(), "bad"), "--live"}, &out); err == nil {
		t.Fatal("live replay accepted no model or budgets")
	}
	if err := workEvalCommand([]string{"watch", "validate", path}, &out); err != nil {
		t.Fatal(err)
	}
	if err := workEvalCommand([]string{"watch", "score", filepath.Join(directory, "watch-report.json")}, &out); err != nil {
		t.Fatal(err)
	}
}

func TestWatchReplayBudgetAndCancellationRemainUnscored(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	path := "../../examples/project-followup/episodes.json"
	dataset, _ := eval.LoadWatchDataset(path)
	dataset.Episodes, dataset.Events = dataset.Episodes[:2], dataset.Events[:1]
	dataset.Events[0].DeadlineSecond = 600
	directory := filepath.Join(t.TempDir(), "budget")
	report, err := runWatchReplay(context.Background(), path, dataset, watchReplayOptions{ReportDir: directory, MaxRequests: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	score, err := eval.ScoreWatch(report, eval.WatchReviews{})
	if err != nil || score.ModelCalls != 1 || score.Failures["budget"] != 2 || score.MissedMaterialChanges.Denominator != 0 || len(score.UnassessedMaterialEvents) != 1 {
		t.Fatalf("budget failures became quality judgments: %#v %v", score, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	directory = filepath.Join(t.TempDir(), "cancelled")
	_, err = runWatchReplay(ctx, path, dataset, watchReplayOptions{ReportDir: directory}, nil)
	if err != context.Canceled {
		t.Fatalf("cancelled replay: %v", err)
	}
	report, err = eval.LoadWatchReport(filepath.Join(directory, "watch-report.json"))
	if err != nil || report.Complete || report.StopReason == "" {
		t.Fatalf("interruption was not retained: %#v %v", report, err)
	}
	score, err = eval.ScoreWatch(report, eval.WatchReviews{})
	if err != nil || score.Episodes != 0 || score.MissedMaterialChanges.Value != nil {
		t.Fatalf("cancelled replay fabricated misses: %#v %v", score, err)
	}
	t.Setenv("GATOR_INTERNAL_COMMANDS", "0")
	if err := runDesktopRuntime([]string{"work", "eval", "watch", "validate", path}, &bytes.Buffer{}); err == nil {
		t.Fatal("watch evaluation bypassed the developer-only command guard")
	}
}
