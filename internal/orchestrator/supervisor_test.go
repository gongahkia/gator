package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/cliagent"
)

func hostedReader(run func(context.Context, Invocation) (Result, error)) Specialist {
	return HostedSpecialist("reader", "Inspect frozen source without mutation.", run)
}

func TestDiscoveryIndexesLatestRetainedNativeThreads(t *testing.T) {
	role := hostedReader(func(context.Context, Invocation) (Result, error) { return Result{}, nil })
	role.Sessions = true
	options := Options{StatePath: t.TempDir(), ParentRun: "index", Source: "source", SourceCapture: "capture", Retained: map[string]Task{}}
	for _, id := range []string{"older", "newer", "other-capture"} {
		task := Task{GlobalID: id + "/subagent-001", Role: "reader", SourceCapture: "capture", Status: "completed", Result: Result{SessionID: "same-thread", Summary: strings.Repeat("z", 2000)}}
		if id == "newer" {
			task.CreatedAt = time.Now()
		}
		if id == "other-capture" {
			task.SourceCapture = "elsewhere"
			task.Result.SessionID = "different-thread"
		}
		options.Retained[task.GlobalID] = task
	}
	s, err := NewSupervisor(context.Background(), []Specialist{role}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	result, err := (taskTool{s, "list_agents"}).Execute(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Threads []retainedThread `json:"retained_threads"`
	}
	if err := json.Unmarshal([]byte(result.Content), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Threads) != 1 || decoded.Threads[0].ID != "newer/subagent-001" || len(decoded.Threads[0].Summary) > 1024 {
		t.Fatalf("wrong retained index: %+v", decoded.Threads)
	}
}

func TestSupervisorNativeContinuationUsesSessionNotSummary(t *testing.T) {
	role := hostedReader(func(_ context.Context, i Invocation) (Result, error) {
		if i.Task == "follow-up" && i.SessionID != "host-retained-session" {
			return Result{}, errors.New("session identity lost")
		}
		if strings.Contains(i.Task, "UNTRUSTED-OLD-SUMMARY") {
			return Result{}, errors.New("native follow-up replaced with summary")
		}
		return Result{Summary: "UNTRUSTED-OLD-SUMMARY", SessionID: "host-retained-session"}, nil
	})
	role.Sessions, role.Steering = true, true
	s, err := NewSupervisor(context.Background(), []Specialist{role}, Options{StatePath: t.TempDir(), ParentRun: "native", Source: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Start(StartRequest{Agent: "reader", Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.Await(context.Background(), first.ID)
	if err != nil || first.Status != "completed" {
		t.Fatal(first, err)
	}
	if _, err := s.Start(StartRequest{Agent: "reader", Task: "follow-up", Continue: first.ID, Baseline: "different.patch"}); err == nil {
		t.Fatal("replaced native baseline")
	}
	next, err := s.Start(StartRequest{Agent: "reader", Task: "follow-up", Continue: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	next, err = s.Await(context.Background(), next.ID)
	if err != nil || next.Status != "completed" || next.Result.SessionID != first.Result.SessionID {
		t.Fatal(next, err)
	}
}

func TestSupervisorRoutesExactNativeQuestion(t *testing.T) {
	question := make(chan struct{})
	role := hostedReader(func(ctx context.Context, i Invocation) (Result, error) {
		state := cliagent.NativeState{Status: "awaiting_input", QuestionID: "question-one", Questions: []cliagent.NativeQuestion{{ID: "format", Text: "Which format?"}}}
		if err := i.OnState("native-session", state); err != nil {
			return Result{}, err
		}
		close(question)
		select {
		case control := <-i.Controls:
			if control.Kind != "answer" || control.QuestionID != "question-one" || len(control.Answers["format"]) != 1 {
				return Result{}, errors.New("wrong answer routed")
			}
			return Result{SessionID: "native-session", Summary: control.Answers["format"][0]}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	})
	role.Sessions = true
	s, err := NewSupervisor(context.Background(), []Specialist{role}, Options{StatePath: t.TempDir(), ParentRun: "question", Source: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task, err := s.Start(StartRequest{Agent: "reader", Task: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	<-question
	if err := s.Answer(task.ID, "stale", map[string][]string{"format": {"Markdown"}}); err == nil {
		t.Fatal("stale answer accepted")
	}
	if err := s.Answer(task.ID, "question-one", map[string][]string{"other": {"Markdown"}}); err == nil {
		t.Fatal("wrong question key accepted")
	}
	for _, tool := range s.Tools() {
		if tool.Definition().Name == "answer_agent" {
			payload, _ := json.Marshal(map[string]any{"id": task.ID, "question_id": "question-one", "answers": map[string][]string{"format": {"Markdown"}}})
			if _, err := tool.Execute(context.Background(), payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := s.Await(context.Background(), task.ID)
	if err != nil || result.Status != "completed" || result.Result.Summary != "Markdown" {
		t.Fatal(result, err)
	}
	if err := s.Answer(task.ID, "question-one", map[string][]string{"format": {"Markdown"}}); err == nil {
		t.Fatal("duplicate answer accepted")
	}
}

func TestSupervisorPersistsEachPeerAndRecoversWithoutReexecution(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	options := Options{StatePath: t.TempDir(), ParentRun: "run-1", Source: "capture", PolicySHA256: "policy", MaxDelegations: 4, MaxParallel: 3}
	s, err := NewSupervisor(context.Background(), []Specialist{hostedReader(func(ctx context.Context, i Invocation) (Result, error) {
		if i.Task == "wait" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		if i.Task == "fail" {
			return Result{}, errors.New("bad source")
		}
		return Result{Summary: i.Task}, nil
	})}, options)
	if err != nil {
		t.Fatal(err)
	}
	slow, _ := s.Start(StartRequest{Agent: "reader", Task: "wait"})
	<-started
	fast, _ := s.Start(StartRequest{Agent: "reader", Task: "fast"})
	failed, _ := s.Start(StartRequest{Agent: "reader", Task: "fail"})
	task, err := s.Await(context.Background(), fast.ID)
	if err != nil || task.Status != "completed" {
		t.Fatalf("%+v %v", task, err)
	}
	if err := s.Cancel(fast.ID); err == nil {
		t.Fatal("completed specialist accepted cancellation")
	}
	data, err := os.ReadFile(filepath.Join(options.StatePath, fast.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var retained Task
	if err := json.Unmarshal(data, &retained); err != nil || retained.Status != "completed" {
		t.Fatalf("completed peer not durable: %s", data)
	}
	task, _ = s.Await(context.Background(), failed.ID)
	if task.Category != "task" {
		t.Fatal(task)
	}
	if err := s.Cancel(slow.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = s.Await(context.Background(), slow.ID)
	if task.Status != "cancelled" {
		t.Fatal(task)
	}
	s.Close()
	recovered, err := NewSupervisor(context.Background(), []Specialist{hostedReader(func(context.Context, Invocation) (Result, error) {
		t.Error("recovery reexecuted work")
		return Result{}, nil
	})}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	task, err = recovered.Await(context.Background(), fast.ID)
	if err != nil || task.Result.Summary != "fast" {
		t.Fatalf("%+v %v", task, err)
	}
}
func TestSupervisorBudgetDependenciesAndSubtreeCancellation(t *testing.T) {
	s, err := NewSupervisor(context.Background(), []Specialist{hostedReader(func(ctx context.Context, i Invocation) (Result, error) { <-ctx.Done(); return Result{}, ctx.Err() })}, Options{StatePath: t.TempDir(), ParentRun: "run", Source: "source", MaxDelegations: 3, MaxParallel: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Start(StartRequest{Agent: "reader", Task: "bad", Dependencies: []string{"future"}}); err == nil {
		t.Fatal("forward dependency accepted")
	}
	if _, err := s.Start(StartRequest{Agent: "publisher", Task: "send"}); err == nil {
		t.Fatal("unknown authority accepted")
	}
	parent, err := s.Start(StartRequest{Agent: "reader", Task: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Start(StartRequest{Agent: "reader", Task: "child", Dependencies: []string{parent.ID}, Parent: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Start(StartRequest{Agent: "reader", Task: "queued"})
			if err == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if accepted != 1 {
		t.Fatalf("budget admitted %d", accepted)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := s.Await(ctx, parent.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := s.Cancel(parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Await(context.Background(), child.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorCrashRecoveryExplicitContinuationAndAtomicBatchValidation(t *testing.T) {
	options := Options{StatePath: t.TempDir(), ParentRun: "old", Source: "frozen", PolicySHA256: "policy"}
	role := hostedReader(func(_ context.Context, i Invocation) (Result, error) { return Result{Summary: i.Task}, nil })
	supervisor, err := NewSupervisor(context.Background(), []Specialist{role}, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.StartBatch([]StartRequest{{Agent: "reader", Task: "allowed"}, {Agent: "publisher", Task: "invalid"}}); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if tasks, err := ReadTasks(options.StatePath); err != nil || len(tasks) != 0 {
		t.Fatalf("partial invalid batch: %+v %v", tasks, err)
	}
	original, err := supervisor.Start(StartRequest{Agent: "reader", Task: "amber retained finding"})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := supervisor.Await(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	supervisor.Close()
	nextOptions := Options{StatePath: t.TempDir(), ParentRun: "new", Source: "frozen", PolicySHA256: "policy", Retained: map[string]Task{completed.GlobalID: completed}}
	next, err := NewSupervisor(context.Background(), []Specialist{role}, nextOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	continued, err := next.Start(StartRequest{Agent: "reader", Task: "followup", Continue: completed.GlobalID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := next.Await(context.Background(), continued.ID)
	if err != nil || !strings.Contains(result.Result.Summary, "amber retained finding") || result.Attempt != 2 {
		t.Fatalf("retained continuation: %+v %v", result, err)
	}
	completed.Status = "running"
	completed.FinishedAt = time.Time{}
	payload, _ := json.Marshal(completed)
	if err := os.WriteFile(filepath.Join(options.StatePath, completed.ID+".json"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewSupervisor(context.Background(), []Specialist{role}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	task, err := recovered.Inspect(completed.ID)
	if err != nil || task.Status != "interrupted" {
		t.Fatalf("crash reconciliation: %+v %v", task, err)
	}
	if _, err := recovered.Start(StartRequest{Agent: "reader", Task: "repeat unknown effect", Continue: completed.ID}); err == nil {
		t.Fatal("interrupted effect silently repeated")
	}
}

func TestManagerCanDiscoverSteerAndPollAgents(t *testing.T) {
	role := Specialist{Name: "reader", Description: "Read only", Backend: BackendLLM, Run: func(ctx context.Context, i Invocation) (Result, error) {
		select {
		case message := <-i.Steering:
			return Result{Summary: message}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}}
	s, err := NewSupervisor(context.Background(), []Specialist{role}, Options{StatePath: t.TempDir(), ParentRun: "managed", Source: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	invoke := func(name, args string) string {
		t.Helper()
		for _, tool := range s.Tools() {
			if tool.Definition().Name == name {
				result, err := tool.Execute(context.Background(), json.RawMessage(args))
				if err != nil {
					t.Fatal(err)
				}
				return result.Content
			}
		}
		t.Fatalf("missing tool %s", name)
		return ""
	}
	if !strings.Contains(invoke("list_agents", `{}`), `"steering":true`) {
		t.Fatal("no capability discovery")
	}
	task, err := s.Start(StartRequest{Agent: "reader", Task: "Wait for guidance"})
	if err != nil {
		t.Fatal(err)
	}
	state := invoke("await_agent", `{"id":"`+task.ID+`","timeout_seconds":1}`)
	if !strings.Contains(state, `"status":"running"`) {
		t.Fatalf("poll must return live status: %s", state)
	}
	invoke("steer_agent", `{"id":"`+task.ID+`","message":"Focus on the budget"}`)
	result, err := s.Await(context.Background(), task.ID)
	if err != nil || result.Result.Summary != "Focus on the budget" {
		t.Fatalf("steering: %#v %v", result, err)
	}
	retained, err := ReadTasks(s.options.StatePath)
	if err != nil || len(retained) != 1 || len(retained[0].SteeringRequests) != 1 {
		t.Fatalf("steering evidence: %#v %v", retained, err)
	}
}
