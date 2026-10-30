package main

import (
	"bytes"
	"context"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCLIConfigurationLifecycle(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := agentFamilyCommand([]string{"cli", "enable", "codex", "--executable", executable, "--model", "fixture-model", "--base-url", "https://us.api.openai.com/v1", "--allow-edits"}, &output); err != nil {
		t.Fatal(err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.ExternalAgents) != 1 || !settings.ExternalAgents[0].AllowEdits || settings.ExternalAgents[0].APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("config: %+v", settings.ExternalAgents)
	}
	output.Reset()
	if err := agentFamilyCommand([]string{"cli", "list"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "codex  enabled  "+executable) {
		t.Fatal(output.String())
	}
	if err := agentFamilyCommand([]string{"cli", "enable", "codex", "--executable", executable, "--api-key-env", "actual-secret-value"}, &output); err == nil {
		t.Fatal("secret accepted as variable name")
	}
	if err := agentFamilyCommand([]string{"cli", "disable", "codex"}, &output); err != nil {
		t.Fatal(err)
	}
	settings, err = store.Load()
	if err != nil || len(settings.ExternalAgents) != 0 {
		t.Fatal("disable did not remove adapter", err)
	}
}

type cliDiscoveryModel struct {
	step  int
	tools string
}

func (m *cliDiscoveryModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.step++
	if m.step == 1 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "discover", Name: "list_agents", Arguments: []byte(`{}`)}}}, nil
	}
	for _, message := range r.Messages {
		if message.Role == "tool" {
			m.tools += message.Content
		}
	}
	return agent.Turn{Text: "Inspected."}, nil
}
func TestWorkCLIExternalAgentsRequireOptIn(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "fixture")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := agentCLICommand([]string{"enable", "codex", "--executable", executable}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		m := &cliDiscoveryModel{}
		args := []string{"--source", t.TempDir(), "--mode", "inspect", "--json"}
		if enabled {
			args = append(args, "--external-agents")
		}
		args = append(args, "List agents")
		if err := runWorkTask(args, strings.NewReader(""), &bytes.Buffer{}, func(_, _, _ string) (agent.Model, error) { return m, nil }); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(m.tools, `"name":"cli_codex"`) != enabled {
			t.Fatalf("unexpected authority: %s", m.tools)
		}
	}
}
