package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/worksession"
)

var errTaskAccess = errors.New("this task requires reviewed connection setup before further execution")

type taskAccessNeeds struct {
	mu    sync.Mutex
	needs []worksession.TaskAccessNeed
}

func (s *taskAccessNeeds) add(need worksession.TaskAccessNeed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, old := range s.needs {
		if old == need {
			return nil
		}
	}
	next := append(append([]worksession.TaskAccessNeed(nil), s.needs...), need)
	if err := worksession.ValidateTaskAccessNeeds(next); err != nil {
		return err
	}
	s.needs = next
	return nil
}
func (s *taskAccessNeeds) snapshot() []worksession.TaskAccessNeed {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]worksession.TaskAccessNeed(nil), s.needs...)
}

type taskAccessTool struct {
	registry connector.Registry
	needs    *taskAccessNeeds
}

func (t taskAccessTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "gator_request_access", Description: "Retain this task's missing account, source or permission requirement for the user to repair and review. Inspect connections first. Choose an exact configured connector_id and operation when known; use kind and operation when no appropriate account is configured or selected. This does not sign in, select an account, grant permission, or execute the operation. It stops further task tools until setup and a fresh plan review. Explain the missing access; do not claim the task completed. Never ask for tokens in chat.", Parameters: json.RawMessage(`{"type":"object","properties":{"connector_id":{"type":"string","maxLength":64},"kind":{"type":"string","enum":["http_json","http_webhook","google_workspace","slack","atlassian","notion","remote_mcp"]},"operation":{"type":"string","maxLength":128}},"required":["kind","operation"],"additionalProperties":false}`)}
}
func (t taskAccessTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}
	var need worksession.TaskAccessNeed
	if err := DecodeAppArguments(raw, &need); err != nil {
		return agent.ToolResult{}, err
	}
	if err := worksession.ValidateTaskAccessNeeds([]worksession.TaskAccessNeed{need}); err != nil {
		return agent.ToolResult{}, err
	}
	descriptor := connector.Descriptor{Kind: need.Kind, Authentication: connector.AuthNone, SearchTool: "search", ReadTool: "read", ActionTool: "action"}
	if need.ConnectorID != "" {
		var ok bool
		descriptor, ok = t.registry.Get(need.ConnectorID)
		if !ok || descriptor.Kind != need.Kind {
			return agent.ToolResult{}, errors.New("inspect current connections; the requested account is missing or changed")
		}
		descriptor.Authentication = connector.AuthNone
	}
	found := false
	for _, op := range descriptor.Operations() {
		found = found || op.ID == need.Operation
	}
	if !found {
		return agent.ToolResult{}, errors.New("the requested operation is not supported by this connection kind")
	}
	if err := t.needs.add(need); err != nil {
		return agent.ToolResult{}, err
	}
	return appResult(map[string]any{"status": "needs_setup", "requirement": need, "note": "The original task and inputs will be retained for account setup and fresh review. No access has been granted."})
}
