package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func TestDelegationRunsSpecialistsAndRecordsBoundedEvidence(t *testing.T) {
	var active, maximum atomic.Int32
	var records []Record
	var events []agent.Event
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	specialist := Specialist{Name: "research", Description: "inspect one bounded question", Run: func(_ context.Context, invocation Invocation) (Result, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := maximum.Load()
			if current <= seen || maximum.CompareAndSwap(seen, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		return Result{Summary: "found " + invocation.Task, Steps: 2}, nil
	}}
	tools, err := Tools([]Specialist{specialist}, Options{
		Now:      func() time.Time { return time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC) },
		OnEvent:  func(event agent.Event) { events = append(events, event) },
		OnRecord: func(record Record) { records = append(records, record) },
	})
	if err != nil {
		t.Fatal(err)
	}
	type execution struct {
		result agent.ToolResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, executeErr := tools[0].Execute(context.Background(), json.RawMessage(`{"tasks":[{"agent":"research","task":"alpha"},{"agent":"research","task":"beta"}]}`))
		done <- execution{result: result, err: executeErr}
	}()
	<-started
	<-started
	close(release)
	executed := <-done
	result, err := executed.result, executed.err
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "found alpha") || len(records) != 2 || records[0].TaskSHA256 == "" || records[0].OutputSHA256 == "" || len(events) != 2 {
		t.Fatalf("result=%s records=%#v events=%#v", result.Content, records, events)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent specialists = %d", maximum.Load())
	}
}

func TestDelegationContainsFailuresAndEnforcesBudget(t *testing.T) {
	tools, err := Tools([]Specialist{{Name: "review", Description: "review output", Run: func(context.Context, Invocation) (Result, error) {
		return Result{}, errors.New("review unavailable")
	}}}, Options{MaxDelegations: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"tasks":[{"agent":"review","task":"check it"}]}`))
	if err != nil || !strings.Contains(result.Content, `"status":"failed"`) {
		t.Fatalf("result=%s err=%v", result.Content, err)
	}
	if _, err := tools[0].Execute(context.Background(), json.RawMessage(`{"tasks":[{"agent":"review","task":"again"}]}`)); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("budget error = %v", err)
	}
}

func TestLLMSpecialistStartsWithFreshContext(t *testing.T) {
	model := &oneTurnModel{}
	specialist := LLMSpecialist("research", "read", model, nil, "specialist system", 2, nil)
	result, err := specialist.Run(context.Background(), Invocation{ID: "subagent-001", Task: "inspect this"})
	if err != nil || result.Summary != "fresh" || len(model.request.Messages) != 1 || model.request.Messages[0].Content != "inspect this" {
		t.Fatalf("result=%#v err=%v request=%#v", result, err, model.request)
	}
}

func TestDelegationRejectsTrailingJSON(t *testing.T) {
	tools, err := Tools([]Specialist{{Name: "research", Description: "read", Run: func(context.Context, Invocation) (Result, error) {
		return Result{Summary: "unused"}, nil
	}}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tools[0].Execute(context.Background(), json.RawMessage(`{"tasks":[{"agent":"research","task":"one"}]} {"extra":true}`)); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing JSON error = %v", err)
	}
}

type oneTurnModel struct{ request agent.TurnRequest }

func (m *oneTurnModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.request = request
	return agent.Turn{Text: "fresh"}, nil
}
