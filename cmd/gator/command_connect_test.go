package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectUsesProviderOwnedCLIWhereAvailable(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "command.log")
	script := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_DELEGATE_LOG\"\n")
	t.Setenv("GATOR_CODEX_COMMAND", script)
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	if err := connect([]string{"codex", "--device"}, &output); err != nil {
		t.Fatalf("connect Codex: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); got != "login\n--device-auth\n" {
		t.Fatalf("Codex connect args = %q", got)
	}
}

func TestConnectXAIUsesOpenCodeAndClaudeStoresAPIKey(t *testing.T) {
	opencodeLog := filepath.Join(t.TempDir(), "opencode.log")
	opencodeScript := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_OPENCODE_LOG\"\n")
	t.Setenv("GATOR_OPENCODE_COMMAND", opencodeScript)
	t.Setenv("GATOR_OPENCODE_LOG", opencodeLog)
	var output bytes.Buffer
	if err := connect([]string{"xai"}, &output); err != nil {
		t.Fatalf("connect xAI: %v", err)
	}
	logged, err := os.ReadFile(opencodeLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); got != "auth\nlogin\n--provider\nxai\n" {
		t.Fatalf("xAI connect args = %q", got)
	}
	if !strings.Contains(output.String(), "Starting OpenCode's xAI provider login") {
		t.Fatalf("xAI connect output = %q", output.String())
	}

	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-connect-key")
	output.Reset()
	if err := connect([]string{"claude"}, &output); err != nil {
		t.Fatalf("connect Claude: %v", err)
	}
	if strings.Contains(output.String(), "anthropic-connect-key") {
		t.Fatalf("Claude connect exposed API key: %q", output.String())
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read("anthropic")
	if err != nil || !found || credential.Key != "anthropic-connect-key" {
		t.Fatalf("Claude connect credential = %#v, found=%v, err=%v", credential, found, err)
	}
}

func TestConnectDirectAPIKeyProvidersFromEnvironment(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "openai-connect-key")
	t.Setenv("GEMINI_API_KEY", "gemini-connect-key")
	var output bytes.Buffer
	for _, provider := range []string{"openai", "gemini"} {
		if err := connect([]string{provider}, &output); err != nil {
			t.Fatalf("connect %s: %v", provider, err)
		}
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	for provider, expected := range map[string]string{"openai": "openai-connect-key", "gemini": "gemini-connect-key"} {
		credential, found, err := credentials.Read(provider)
		if err != nil || !found || credential.Key != expected {
			t.Fatalf("%s credential = %#v, found=%v, err=%v", provider, credential, found, err)
		}
	}
	if strings.Contains(output.String(), "openai-connect-key") || strings.Contains(output.String(), "gemini-connect-key") {
		t.Fatalf("connect output leaked a key: %q", output.String())
	}
}
