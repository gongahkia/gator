package main

import (
	"context"
	"errors"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workrun"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

type auditStreamingModel struct{}

func (auditStreamingModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	return agent.Turn{Text: "done"}, nil
}
func (auditStreamingModel) CompleteStream(context.Context, agent.TurnRequest, func(string)) (agent.Turn, error) {
	return agent.Turn{Text: "done"}, nil
}
func (auditStreamingModel) SupportsVisualInput() bool { return false }

func TestWorkBackendForwardsOptionalInterfaces(t *testing.T) {
	var model agent.Model = &nativeWorkBackend{Model: auditStreamingModel{}}
	if _, ok := model.(agent.StreamingModel); !ok {
		t.Fatal("missing streaming support")
	}
	if _, ok := model.(agent.VisualInputModel); !ok {
		t.Fatal("missing visual support interface")
	}
}

type depthStreaming struct{ streamed bool }

func (m *depthStreaming) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	panic("streaming path lost")
}
func (m *depthStreaming) CompleteStream(ctx context.Context, _ agent.TurnRequest, delta func(string)) (agent.Turn, error) {
	if ctx.Err() != nil {
		return agent.Turn{}, ctx.Err()
	}
	m.streamed = true
	delta("incremental")
	return agent.Turn{Text: "complete"}, nil
}
func (*depthStreaming) SupportsVisualInput() bool { return false }
func TestAssembledWorkBackendStreamingFallbackCancellationAndVisualBoundary(t *testing.T) {
	streaming := &depthStreaming{}
	backend := &nativeWorkBackend{Model: streaming}
	text := ""
	turn, err := backend.CompleteStream(context.Background(), agent.TurnRequest{}, func(s string) { text += s })
	if err != nil || !streaming.streamed || text != "incremental" || turn.Text != "complete" {
		t.Fatal("streaming was not forwarded")
	}
	if backend.SupportsVisualInput() {
		t.Fatal("text-only declaration lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.CompleteStream(ctx, agent.TurnRequest{}, func(string) {}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	backend.Model = depthModel(func(context.Context, agent.TurnRequest) (agent.Turn, error) { return agent.Turn{Text: "fallback"}, nil })
	turn, err = backend.CompleteStream(context.Background(), agent.TurnRequest{}, func(string) { t.Error("nonstream emitted a delta") })
	if err != nil || turn.Text != "fallback" {
		t.Fatal("nonstream fallback lost")
	}
	backend.Model = streaming
	_, err = (workrun.Service{Executor: workrun.Executor{Model: backend, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: t.TempDir(), Objective: "Inspect PDF", Mode: action.Inspect, Contract: artifact.InspectionContract(), Attachments: []agent.Attachment{{Name: "reference.pdf", MediaType: "application/pdf", Data: []byte("%PDF-1.4\n")}}})
	if err == nil || !strings.Contains(err.Error(), "does not support PDF") {
		t.Fatalf("unsupported attachment: %v", err)
	}
}
