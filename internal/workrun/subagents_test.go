package workrun

import (
	"context"
	"testing"

	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestHostedCodeSpecialistUsesHostedBackend(t *testing.T) {
	executor := Executor{Code: func(context.Context, CodeRequest) (CodeResult, error) {
		return CodeResult{Summary: "unchanged"}, nil
	}}
	specialist := executor.hostedCodeSpecialist(Request{RunID: "work-test"}, workspace.Work{})
	if specialist.Backend != orchestrator.BackendHosted || specialist.Name != "code" {
		t.Fatalf("specialist=%#v", specialist)
	}
}
