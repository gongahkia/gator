package engine

import (
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestValidateAgentToolUsesRunPolicy(t *testing.T) {
	policy := domain.RunAgentPolicy{Tools: map[string]domain.AgentToolPolicy{
		"http_get":      {Enabled: true, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 2},
		"artifact_read": {Enabled: true, Roles: []string{"researcher"}, AllowedPathPrefixes: []string{"agent-input/"}, MaxCalls: 2},
	}}
	s := &Service{}
	if _, err := s.validateAgentTool(policy, "researcher", agentToolCall{Tool: "http_get", Params: map[string]any{"url": "https://other.example.test/a"}}); err == nil {
		t.Fatal("unallowlisted host accepted")
	}
	if _, err := s.validateAgentTool(policy, "researcher", agentToolCall{Tool: "artifact_read", Params: map[string]any{"path": "generated-app/a.txt"}}); err == nil {
		t.Fatal("path outside operator prefix accepted")
	}
	if _, err := s.validateAgentTool(policy, "operator", agentToolCall{Tool: "http_get", Params: map[string]any{"url": "https://api.example.test/a"}}); err == nil {
		t.Fatal("role outside operator policy accepted")
	}
}
