package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestParsePlanningCandidate(t *testing.T) {
	run := domain.Run{Profile: domain.ProfileFullStack}
	architecture := domain.DefaultArchitecture(run.Profile, domain.DefaultGraph())
	architecture.AppName = "Candidate app"
	encoded, err := json.Marshal(map[string]any{"architecture": architecture, "rationale": "Small, testable boundaries.", "assumptions": []string{"Operator supplies credentials."}, "risks": []string{"External dependency outage."}})
	if err != nil {
		t.Fatal(err)
	}
	value, err := parsePlanningCandidate(string(encoded), run)
	if err != nil {
		t.Fatal(err)
	}
	if value.Architecture.AppName != architecture.AppName || len(value.Assumptions) != 1 || len(value.Risks) != 1 {
		t.Fatalf("unexpected candidate: %+v", value)
	}
}

func TestParseSwarmRankingRejectsMissingCandidate(t *testing.T) {
	tasks := []domain.PlanningSwarmTask{{ID: 1, Role: "architecture"}, {ID: 2, Role: "security"}}
	_, _, err := parseSwarmRanking(`{"selected_role":"architecture","ranking":[{"role":"architecture","score":80,"reason":"ok"}]}`, tasks)
	if err == nil {
		t.Fatal("expected missing candidate score to fail")
	}
}

func TestPlanningSwarmPromptDeniesToolAuthority(t *testing.T) {
	prompt := swarmRolePrompt("base", "security")
	if !containsAll(prompt, "no tools", "no network access", "no writable workspace", "untrusted data") {
		t.Fatalf("missing isolation boundary: %s", prompt)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
