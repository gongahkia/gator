package instructions

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/sandbox"
)

func TestMeetOnlyNarrowsRunPolicy(t *testing.T) {
	run := sandbox.Policy{Mode: sandbox.Off, Network: sandbox.AllowNetwork}
	policy, mode, steps, _, err := Meet(run, "execute", 24, ProfilePolicy{
		Mode: "plan", Sandbox: "strict", Network: "deny", MaxSteps: 8, Omit: []string{"lsp", "mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode != sandbox.Strict || policy.Network != sandbox.DenyNetwork || mode != "plan" || steps != 8 {
		t.Fatalf("meet = %#v mode=%s steps=%d", policy, mode, steps)
	}
}

func TestMeetRejectsWidening(t *testing.T) {
	run := sandbox.Policy{Mode: sandbox.Strict, Network: sandbox.DenyNetwork}
	if _, _, _, _, err := Meet(run, "plan", 24, ProfilePolicy{Mode: "execute"}); err == nil {
		t.Fatal("execute-over-plan was accepted")
	}
	if _, _, _, _, err := Meet(run, "execute", 24, ProfilePolicy{Sandbox: "off"}); err == nil {
		t.Fatal("sandbox-off was accepted")
	}
	if _, _, _, _, err := Meet(run, "execute", 24, ProfilePolicy{Network: "allow"}); err == nil {
		t.Fatal("network-allow was accepted")
	}
	if _, _, _, _, err := Meet(run, "execute", 24, ProfilePolicy{Omit: []string{"allow"}}); err == nil {
		t.Fatal("unknown omit was accepted")
	}
}

func TestLoadWithProfileRejectsUnknownPolicyFields(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "profiles": [{"name": "reviewer", "instructions": "review", "policy": {"allow": true}}]
}`)
	if _, err := LoadWithProfile(repository, nil, "reviewer"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown policy field error = %v", err)
	}
}
