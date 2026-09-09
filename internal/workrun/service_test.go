package workrun

import (
	"context"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"strings"
	"testing"
	"time"
)

type controlledModel struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (m *controlledModel) Complete(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if m.calls == 1 {
		close(m.entered)
		select {
		case <-m.release:
		case <-ctx.Done():
			return agent.Turn{}, ctx.Err()
		}
	}
	return agent.Turn{Text: r.Messages[len(r.Messages)-1].Content}, nil
}
func TestServiceSteersAndCancelsRunningOperation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{true: "cancel", false: "steer"}[cancelRun], func(t *testing.T) {
			model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
			service := Service{Executor: Executor{Model: model, StateDir: t.TempDir()}}
			operation := service.Start(context.Background(), Request{SourcePath: t.TempDir(), Objective: "inspect", Mode: action.Inspect, Contract: artifact.InspectionContract()})
			<-model.entered
			if cancelRun {
				operation.Cancel()
			} else {
				if err := operation.Steer("Remember amber"); err != nil {
					t.Fatal(err)
				}
				close(model.release)
			}
			timeout := time.After(3 * time.Second)
			for {
				select {
				case result := <-operation.Done:
					if result.Outcome.RevisionID == "" {
						t.Fatal("no inspectable revision")
					}
					if cancelRun && result.Err == nil {
						t.Fatal("cancellation lost")
					}
					if !cancelRun && (result.Err != nil || !strings.Contains(result.Outcome.Result.FinalText, "amber")) {
						t.Fatalf("%+v %v", result.Outcome.Result, result.Err)
					}
					return
				case <-operation.Events:
				case <-timeout:
					t.Fatal("operation did not stop")
				}
			}
		})
	}
}
