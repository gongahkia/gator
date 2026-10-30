package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

type completionFixture struct {
	tool        *codeCompletion
	work        workspace.Work
	assignments []string
	reviews     int
	question    chan struct{}
}

func newCompletionFixture(t *testing.T, values []string, rejectFirst, ask bool) *completionFixture {
	t.Helper()
	if _, err := sandbox.StrictAvailability(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	work, err := workspace.CreateWork(source, state, "completion-run", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := Request{RunID: "completion-run", StateDir: state, Mode: action.Draft, Contract: artifact.DefaultContract("report.md"), Code: CodePolicy{Verification: [][]string{{"sh", "-c", `case "$(cat note.txt)" in fixed*) exit 0;; *) echo 'note must be fixed'; exit 1;; esac`}}, Sandbox: sandbox.Policy{Mode: sandbox.Strict}}}
	r.integration = &codeIntegration{request: r, work: work, patches: map[string]string{}, accepted: map[string]string{}}
	f := &completionFixture{work: work, question: make(chan struct{})}
	worker := orchestrator.HostedSpecialist("cli_codex", "Fixture native worker", func(ctx context.Context, i orchestrator.Invocation) (orchestrator.Result, error) {
		f.assignments = append(f.assignments, i.Task)
		index := len(f.assignments) - 1
		if len(values) > 0 && values[0] == "block" {
			close(f.question)
			<-ctx.Done()
			return orchestrator.Result{}, ctx.Err()
		}
		if ask && index == 0 {
			if err := i.OnState("native-one", cliagent.NativeState{Status: "awaiting_input", QuestionID: strings.Repeat("a", 64), Questions: []cliagent.NativeQuestion{{ID: "format", Text: "Which format?"}}}); err != nil {
				return orchestrator.Result{}, err
			}
			close(f.question)
			select {
			case <-i.Controls:
			case <-ctx.Done():
				return orchestrator.Result{}, ctx.Err()
			}
		}
		result := orchestrator.Result{SessionID: "native-one", Summary: "I fixed it."}
		if len(values) == 0 {
			return result, nil
		}
		value := values[min(index, len(values)-1)]
		payload := []byte(fmt.Sprintf("diff --git a/note.txt b/note.txt\n--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-before\n+%s\n", value))
		if value == "conflict" {
			payload = []byte("diff --git a/note.txt b/note.txt\n--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-not-the-baseline\n+fixed\n")
		}
		name := "code/" + i.ID + ".patch"
		if err := artifact.WriteBinary(work.Output, r.Contract, name, payload); err != nil {
			return result, err
		}
		hash := sha256.Sum256(payload)
		result.ArtifactPath, result.ArtifactSHA256 = name, hex.EncodeToString(hash[:])
		r.integration.mu.Lock()
		r.integration.patches[name] = result.ArtifactSHA256
		r.integration.mu.Unlock()
		return result, nil
	})
	worker.Sessions = true
	reviewer := orchestrator.HostedSpecialist("artifact_reviewer", "Read-only independent review fixture", func(_ context.Context, i orchestrator.Invocation) (orchestrator.Result, error) {
		f.reviews++
		payload, err := r.integration.baseline(i.Baseline)
		if err != nil {
			return orchestrator.Result{}, err
		}
		hash := sha256.Sum256(payload)
		passed, findings := true, []string{}
		if rejectFirst && f.reviews == 1 {
			passed, findings = false, []string{"The requested review marker is missing."}
		}
		data, _ := json.Marshal(completionReview{Passed: &passed, CandidateSHA256: hex.EncodeToString(hash[:]), Findings: findings})
		return orchestrator.Result{Summary: string(data)}, nil
	})
	s, err := orchestrator.NewSupervisor(context.Background(), []orchestrator.Specialist{worker, reviewer}, orchestrator.Options{StatePath: filepath.Join(state, "tasks"), ParentRun: r.RunID, Source: work.Source.Path()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	f.tool = &codeCompletion{supervisor: s, request: r, eligible: map[string]bool{"cli_codex": true}, policy: "fixed-host-policy"}
	return f
}

func (f *completionFixture) execute(t *testing.T, ctx context.Context, args string) completionRecord {
	t.Helper()
	result, err := f.tool.Execute(ctx, []byte(args))
	if err != nil {
		t.Fatal(err)
	}
	var record completionRecord
	if err := json.Unmarshal([]byte(result.Content), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

const completionArgs = `{"agent":"cli_codex","objective":"Fix note.txt","acceptance":["note.txt has the requested fixed content"]}`

func TestCompletionRepairsVerificationFailureAndKeepsEvidence(t *testing.T) {
	f := newCompletionFixture(t, []string{"broken", "fixed"}, false, false)
	r := f.execute(t, context.Background(), completionArgs)
	if r.Status != "completed" || len(r.Attempts) != 2 || f.reviews != 1 || !strings.Contains(f.assignments[1], "Deterministic verification failed") {
		t.Fatalf("wrong completion: %+v", r)
	}
	if r.Attempts[0].Candidate == nil || r.Attempts[0].Candidate.Status == "verified" {
		t.Fatal("failed attempt lost")
	}
	for _, a := range r.Attempts {
		if _, err := f.work.Output.ReadRegularFile(a.Artifact, 1024*1024); err != nil {
			t.Fatal(err)
		}
		if a.SessionID != "native-one" {
			t.Fatal("lost worker identity")
		}
	}
	unchanged, err := f.work.Source.ReadRegularFile("note.txt", 1024)
	if err != nil || string(unchanged) != "before\n" {
		t.Fatal("source mutated", err)
	}
	before := len(f.assignments)
	resumed := f.execute(t, context.Background(), `{"resume":"`+r.ID+`"}`)
	if resumed.Status != "completed" || len(f.assignments) != before {
		t.Fatal("replayed completed work")
	}
}

func TestCompletionRepairsIndependentReviewFailure(t *testing.T) {
	f := newCompletionFixture(t, []string{"fixed", "fixed-reviewed"}, true, false)
	r := f.execute(t, context.Background(), completionArgs)
	if r.Status != "completed" || len(r.Attempts) != 2 || f.reviews != 2 || !strings.Contains(f.assignments[1], "review marker") {
		t.Fatalf("review loop failed: %+v", r)
	}
}

func TestCompletionStopsFalseSuccessRepeatedFailureConflictsAndBudgets(t *testing.T) {
	for _, test := range []struct {
		name     string
		values   []string
		status   string
		attempts int
	}{
		{"false-success", nil, "stalled", 2},
		{"repeated-failure", []string{"broken"}, "stalled", 2},
		{"conflict", []string{"conflict"}, "stalled", 2},
		{"budget", []string{"broken-1", "broken-2", "broken-3"}, "exhausted", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompletionFixture(t, test.values, false, false)
			r := f.execute(t, context.Background(), completionArgs)
			if r.Status != test.status || len(r.Attempts) != test.attempts || f.reviews != 0 {
				t.Fatalf("false completion: %+v", r)
			}
		})
	}
}

func TestCompletionWaitsForQuestionAndResumesExistingWorker(t *testing.T) {
	f := newCompletionFixture(t, []string{"fixed"}, false, true)
	r := f.execute(t, context.Background(), completionArgs)
	if r.Status != "waiting_input" || len(r.Attempts) != 1 {
		t.Fatal(r)
	}
	worker := r.Attempts[0].WorkerID
	if err := f.tool.supervisor.Answer(worker, strings.Repeat("a", 64), map[string][]string{"format": {"Markdown"}}); err != nil {
		t.Fatal(err)
	}
	r = f.execute(t, context.Background(), `{"resume":"`+r.ID+`"}`)
	if r.Status != "completed" || len(f.assignments) != 1 || r.Attempts[0].WorkerID != worker {
		t.Fatalf("question replayed worker: %+v", r)
	}
}

func TestCompletionCancellationStopsWaitingWorker(t *testing.T) {
	f := newCompletionFixture(t, []string{"fixed"}, false, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := f.execute(t, ctx, completionArgs)
	if r.Status != "cancelled" || len(f.assignments) != 0 {
		t.Fatal(r)
	}
	// A fresh loop pauses on input; an interrupted owner is never auto-replayed.
	r = f.execute(t, context.Background(), completionArgs)
	r.Status, r.Attempts[0].Stage = "running", "implementing"
	data, _ := json.Marshal(r)
	path := filepath.Join(f.tool.request.StateDir, "gator", "completions", f.tool.request.RunID, r.ID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	r = f.execute(t, context.Background(), `{"resume":"`+r.ID+`"}`)
	if r.Status != "needs_reconciliation" || len(f.assignments) != 1 {
		t.Fatal("replayed uncertain phase", r)
	}
}

func TestCompletionCancelsAnActiveWorkerBeforeReturning(t *testing.T) {
	f := newCompletionFixture(t, []string{"block"}, false, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan completionRecord, 1)
	failed := make(chan error, 1)
	go func() {
		result, err := f.tool.Execute(ctx, []byte(completionArgs))
		if err != nil {
			failed <- err
			return
		}
		var record completionRecord
		err = json.Unmarshal([]byte(result.Content), &record)
		if err != nil {
			failed <- err
			return
		}
		done <- record
	}()
	<-f.question
	cancel()
	select {
	case err := <-failed:
		t.Fatal(err)
	case r := <-done:
		if r.Status != "cancelled" || len(r.Attempts) != 1 {
			t.Fatal(r)
		}
		worker, err := f.tool.supervisor.Inspect(r.Attempts[0].WorkerID)
		if err != nil || worker.Status != "cancelled" {
			t.Fatalf("returned while worker active: %+v %v", worker, err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("cancellation hung")
	}
}

func TestCompletionRejectsAuthorityChangesInArguments(t *testing.T) {
	f := newCompletionFixture(t, nil, false, false)
	for _, args := range []string{`{"agent":"shell","objective":"run","acceptance":["yes"]}`, `{"agent":"cli_codex","objective":"run","acceptance":["yes"],"verification":[["sh"]]}`, `{"resume":"../../outside"}`} {
		if _, err := f.tool.Execute(context.Background(), []byte(args)); err == nil {
			t.Fatal("accepted widened authority", args)
		}
	}
}
