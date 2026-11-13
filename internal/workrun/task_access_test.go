package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"testing"
)

func TestTaskAccessNeedCannotCreateAnExecutionGrant(t *testing.T) {
	registry, err := connector.NewRegistry([]connector.Descriptor{{Version: 1, ID: "reports", Name: "Reports", Kind: connector.KindHTTPJSON, Resource: "https://example.com/report", Authentication: connector.AuthBearer}})
	if err != nil {
		t.Fatal(err)
	}
	needs := &taskAccessNeeds{}
	tool := taskAccessTool{registry: registry, needs: needs}
	for _, raw := range []string{`{"kind":"http_json","operation":"publish"}`, `{"kind":"slack","connector_id":"reports","operation":"search"}`, `{"kind":"http_json","connector_id":"missing","operation":"fetch"}`, `{"kind":"http_json","operation":"fetch","approved":true}`} {
		if _, err := tool.Execute(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatal("invalid access requirement admitted", raw)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"http_json","operation":"fetch"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if len(needs.snapshot()) != 1 {
		t.Fatal("repeated need was duplicated")
	}
	if effects, err := planToolEffects(tool, agent.ToolCall{Name: "gator_request_access"}); err != nil || len(effects) != 1 || effects[0] != "read" {
		t.Fatal("setup request acquired mutating effects", effects, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tool.Execute(ctx, json.RawMessage(`{"kind":"http_json","operation":"fetch"}`)); err == nil {
		t.Fatal("cancelled setup request was applied")
	}
}

func TestTaskAccessRequirementFencesLaterToolsInTheSameModelTurn(t *testing.T) {
	fixture := &appControllerFixture{}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{
			{ID: "setup-first", Name: "gator_request_access", Arguments: json.RawMessage(`{"kind":"http_json","operation":"fetch"}`)},
			{ID: "must-not-apply", Name: "gator_app_update", Arguments: json.RawMessage(`{"setting":"theme","value":{"theme":"mono"}}`)},
		}},
		{Text: "Account setup is required before this task can finish."},
	}}
	outcome, err := (Executor{Model: model, App: fixture, StateDir: t.TempDir()}).Execute(context.Background(), Request{ForegroundApp: true, SourcePath: t.TempDir(), Objective: "Read my report", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if !errors.Is(err, errTaskAccess) || outcome.Recovery == nil || !outcome.Recovery.RetrySafe || len(outcome.Recovery.Needs) != 1 {
		t.Fatal("setup outcome was lost or claimed completion", err, outcome.Recovery)
	}
	if fixture.applied != 0 {
		t.Fatal("a later tool in the same model turn executed after setup was requested")
	}
	if outcome.Manifest.Status != artifact.Failed || outcome.RevisionID == "" {
		t.Fatal("the actual setup failure was not retained")
	}
}
