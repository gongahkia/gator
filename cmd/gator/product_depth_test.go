package main

import (
	"context"
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
