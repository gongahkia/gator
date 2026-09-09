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
)

func TestSupervisorPersistsEachPeerAndRecoversWithoutReexecution(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	options := Options{StatePath: t.TempDir(), ParentRun: "run-1", Source: "capture", PolicySHA256: "policy", MaxDelegations: 4, MaxParallel: 3}
	s, err := NewSupervisor(context.Background(), []Specialist{{Name: "reader", Run: func(ctx context.Context, i Invocation) (Result, error) {
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
	}}}, options)
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
	recovered, err := NewSupervisor(context.Background(), []Specialist{{Name: "reader", Run: func(context.Context, Invocation) (Result, error) {
		t.Error("recovery reexecuted work")
		return Result{}, nil
	}}}, options)
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
	s, err := NewSupervisor(context.Background(), []Specialist{{Name: "reader", Run: func(ctx context.Context, i Invocation) (Result, error) { <-ctx.Done(); return Result{}, ctx.Err() }}}, Options{StatePath: t.TempDir(), ParentRun: "run", Source: "source", MaxDelegations: 3, MaxParallel: 1})
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
	role := Specialist{Name: "reader", Run: func(_ context.Context, i Invocation) (Result, error) { return Result{Summary: i.Task}, nil }}
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
