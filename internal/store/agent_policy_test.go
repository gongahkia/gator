package store

import (
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestPolicyRestrictionCannotRestoreCapability(t *testing.T) {
	base := domain.RunAgentPolicy{Stages: map[domain.Stage]domain.InternalAgentPolicy{}, Tools: map[string]domain.AgentToolPolicy{"http_get": {Enabled: true, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 4}}}
	for _, stage := range domain.Stages {
		base.Stages[stage] = domain.InternalAgentPolicy{Enabled: true, AllowModel: stage != domain.StageDeployer, AllowNetwork: true, AllowCLI: stage == domain.StageDeployer}
	}
	restricted := base
	restricted.Stages = cloneStages(base.Stages)
	restricted.Tools = map[string]domain.AgentToolPolicy{"http_get": {Enabled: false, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 4}}
	if err := policyIsRestriction(restricted, base, base); err != nil {
		t.Fatalf("restrict policy: %v", err)
	}
	restored := restricted
	restored.Tools = map[string]domain.AgentToolPolicy{"http_get": {Enabled: true, Roles: []string{"researcher"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 4}}
	if err := policyIsRestriction(restored, restricted, base); err == nil {
		t.Fatal("restoring a disabled tool succeeded")
	}
}

func TestPolicyRestrictionRejectsBroaderHostAndApproval(t *testing.T) {
	baseTool := domain.AgentToolPolicy{Enabled: true, ApprovalRequired: true, Roles: []string{"operator"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 4}
	if toolPolicyRestricted(domain.AgentToolPolicy{Enabled: true, ApprovalRequired: false, Roles: []string{"operator"}, AllowedHosts: []string{"api.example.test"}, MaxCalls: 4}, baseTool, baseTool) {
		t.Fatal("removing approval succeeded")
	}
	if toolPolicyRestricted(domain.AgentToolPolicy{Enabled: true, ApprovalRequired: true, Roles: []string{"operator"}, AllowedHosts: []string{"api.example.test", "other.example.test"}, MaxCalls: 4}, baseTool, baseTool) {
		t.Fatal("adding an HTTPS host succeeded")
	}
}

func cloneStages(value map[domain.Stage]domain.InternalAgentPolicy) map[domain.Stage]domain.InternalAgentPolicy {
	result := map[domain.Stage]domain.InternalAgentPolicy{}
	for key, item := range value {
		result[key] = item
	}
	return result
}
