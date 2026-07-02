package config

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/llm"
)

func TestLoadPrecedence(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `
max_turns = 7
max_brain_tokens = 123
call_timeout = "3s"

[brain]
transport = "openai"
base_url = "https://file.example/v1"
api_key = "file-key"
model = "file-brain"

[drone]
transport = "ollama"
base_url = "http://file-drone"
model = "file-drone"

[gather]
max_depth = 2
max_file_bytes = 99
`)
	t.Setenv("PAW_BRAIN_MODEL", "env-brain")
	t.Setenv("PAW_MAX_TURNS", "9")
	t.Setenv("PAW_CALL_TIMEOUT", "5s")
	t.Setenv("PAW_GATHER_MAX_FILE_BYTES", "256")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.BaseURL != "https://file.example/v1" || cfg.Brain.APIKey != "file-key" {
		t.Fatalf("brain file overrides failed: %#v", cfg.Brain)
	}
	if cfg.Brain.Model != "env-brain" {
		t.Fatalf("brain env override failed: %#v", cfg.Brain)
	}
	if cfg.Drone.BaseURL != "http://file-drone" || cfg.Drone.Model != "file-drone" {
		t.Fatalf("drone file overrides failed: %#v", cfg.Drone)
	}
	if cfg.MaxTurns != 9 || cfg.MaxBrainTokens != 123 || cfg.CallTimeout != 5*time.Second {
		t.Fatalf("top-level precedence failed: %#v", cfg)
	}
	if cfg.Gather.MaxDepth != 2 || cfg.Gather.MaxFileBytes != 256 {
		t.Fatalf("gather precedence failed: %#v", cfg.Gather)
	}
}

func TestLoadAllowsMissingBrainKeyUntilChat(t *testing.T) {
	clearPawEnv(t)
	path := writeConfig(t, `
[brain]
transport = "openai"
base_url = "https://brain.example/v1"
model = "glm-test"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Brain.APIKey != "" {
		t.Fatalf("brain api key = %q", cfg.Brain.APIKey)
	}
	client, err := llm.NewBrainClient(llm.FactoryConfig{
		Brain: llm.EndpointConfig{
			Transport: cfg.Brain.Transport,
			BaseURL:   cfg.Brain.BaseURL,
			APIKey:    cfg.Brain.APIKey,
			Model:     cfg.Brain.Model,
		},
	})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	_, err = client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_API_KEY") {
		t.Fatalf("expected missing key chat error, got %v", err)
	}
}

func clearPawEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PAW_BRAIN_TRANSPORT",
		"PAW_BRAIN_BASE_URL",
		"PAW_BRAIN_API_KEY",
		"PAW_BRAIN_MODEL",
		"PAW_DRONE_TRANSPORT",
		"PAW_DRONE_BASE_URL",
		"PAW_DRONE_API_KEY",
		"PAW_DRONE_MODEL",
		"PAW_MAX_TURNS",
		"PAW_MAX_BRAIN_TOKENS",
		"PAW_CALL_TIMEOUT",
		"PAW_GATHER_MAX_DEPTH",
		"PAW_GATHER_MAX_FILE_BYTES",
	} {
		t.Setenv(key, "")
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/config.toml"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
