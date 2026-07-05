package llm

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCLITransportE2ESmoke(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		modelEnv  string
		binary    string
		newClient func(string) Client
	}{
		{name: "codex", env: "PAW_E2E_CODEX_CLI", modelEnv: "PAW_E2E_CODEX_MODEL", binary: "codex", newClient: NewCodexCLIClient},
		{name: "gemini", env: "PAW_E2E_GEMINI_CLI", modelEnv: "PAW_E2E_GEMINI_MODEL", binary: "gemini", newClient: NewGeminiCLIClient},
		{name: "claude", env: "PAW_E2E_CLAUDE_CLI", modelEnv: "PAW_E2E_CLAUDE_MODEL", binary: "claude", newClient: NewClaudeCLIClient},
		{name: "opencode", env: "PAW_E2E_OPENCODE_CLI", modelEnv: "PAW_E2E_OPENCODE_MODEL", binary: "opencode", newClient: NewOpenCodeCLIClient},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !truthyEnv(tc.env) {
				t.Skipf("set %s=1 to run logged-in %s smoke test", tc.env, tc.name)
			}
			if _, err := exec.LookPath(tc.binary); err != nil {
				t.Fatalf("%s not found: %v", tc.binary, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			resp, err := tc.newClient(os.Getenv(tc.modelEnv)).Chat(ctx, ChatRequest{
				Messages: []ChatMessage{{
					Role:    "user",
					Content: `Return exactly {"ok":true} as JSON. Do not inspect files. Do not edit files.`,
				}},
				JSONSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"type":"boolean"}}}`),
			})
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			assertSmokeOK(t, resp.Content)
		})
	}
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func assertSmokeOK(t *testing.T, raw string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode response: %v: %s", err, raw)
	}
	ok, hasOK := doc["ok"].(bool)
	if !hasOK || !ok || len(doc) != 1 {
		t.Fatalf("response mismatch: %s", raw)
	}
}
