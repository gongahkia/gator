package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestBrainClientDefaultsToOllama(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOllama("local", "ok")
	t.Setenv("PAW_BRAIN_BASE_URL", srv.URL)
	t.Setenv("PAW_BRAIN_MODEL", "local-brain")

	client, err := NewBrainClient(FactoryConfig{})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "local"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if srv.LastRequest().Path != "/api/chat" {
		t.Fatalf("path = %q", srv.LastRequest().Path)
	}
}

func TestBrainOpenAIAllowsLocalNoKey(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("local", "ok")
	t.Setenv("PAW_BRAIN_TRANSPORT", "openai")
	t.Setenv("PAW_BRAIN_BASE_URL", srv.URL)
	t.Setenv("PAW_BRAIN_MODEL", "local-openai")

	client, err := NewBrainClient(FactoryConfig{})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "local"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if got := srv.LastRequest().Header.Get("Authorization"); got != "" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestNewBrainClient_ErrorsOnMissingKey(t *testing.T) {
	tests := []struct {
		name    string
		brain   EndpointConfig
		wantErr bool
	}{
		{
			name: "openai remote",
			brain: EndpointConfig{
				Transport: "openai",
				BaseURL:   "https://api.openai.com/v1",
				Model:     "gpt-test",
			},
			wantErr: true,
		},
		{
			name: "openai loopback",
			brain: EndpointConfig{
				Transport: "openai",
				BaseURL:   "http://localhost:1234/v1",
				Model:     "local-test",
			},
		},
		{
			name: "anthropic",
			brain: EndpointConfig{
				Transport: "anthropic",
				BaseURL:   "https://api.anthropic.com",
				Model:     "claude-test",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PAW_BRAIN_TRANSPORT", "")
			t.Setenv("PAW_BRAIN_BASE_URL", "")
			t.Setenv("PAW_BRAIN_API_KEY", "")
			t.Setenv("PAW_BRAIN_MODEL", "")

			client, err := NewBrainClient(FactoryConfig{Brain: tt.brain})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_API_KEY") {
					t.Fatalf("expected PAW_BRAIN_API_KEY error, got client=%T err=%v", client, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("brain client: %v", err)
			}
			if client == nil {
				t.Fatal("brain client is nil")
			}
		})
	}
}

func TestNewDroneClient_ErrorsOnMissingKey(t *testing.T) {
	t.Setenv("PAW_DRONE_TRANSPORT", "")
	t.Setenv("PAW_DRONE_BASE_URL", "")
	t.Setenv("PAW_DRONE_API_KEY", "")
	t.Setenv("PAW_DRONE_MODEL", "")

	client, err := NewDroneClient(FactoryConfig{
		Drone: EndpointConfig{
			Transport: "openai",
			BaseURL:   "https://api.openai.com/v1",
			Model:     "gpt-test",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "PAW_DRONE_API_KEY") {
		t.Fatalf("expected PAW_DRONE_API_KEY error, got client=%T err=%v", client, err)
	}
}

func TestFactoryHTTPClientsUseConfiguredTimeout(t *testing.T) {
	timeout := 25 * time.Millisecond
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	tests := []struct {
		name     string
		endpoint EndpointConfig
	}{
		{
			name: "openai",
			endpoint: EndpointConfig{
				Transport: "openai",
				BaseURL:   srv.URL,
				APIKey:    "test-key",
				Model:     "gpt-test",
			},
		},
		{
			name: "anthropic",
			endpoint: EndpointConfig{
				Transport: "anthropic",
				BaseURL:   srv.URL,
				APIKey:    "test-key",
				Model:     "claude-test",
			},
		},
		{
			name: "ollama",
			endpoint: EndpointConfig{
				Transport: "ollama",
				BaseURL:   srv.URL,
				Model:     "llama-test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewBrainClient(FactoryConfig{
				Brain:       tt.endpoint,
				CallTimeout: timeout,
			})
			if err != nil {
				t.Fatalf("brain client: %v", err)
			}
			got, ok := httpClientTimeout(client)
			if !ok {
				t.Fatalf("no http client timeout for %T", client)
			}
			if got != timeout {
				t.Fatalf("http timeout = %s, want %s", got, timeout)
			}

			start := time.Now()
			_, err = client.Chat(context.Background(), ChatRequest{
				Messages: []ChatMessage{{Role: "user", Content: "hang"}},
			})
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("expected timeout error")
			}
			if elapsed > timeout+5*time.Second {
				t.Fatalf("timeout took %s, want <= %s", elapsed, timeout+5*time.Second)
			}
		})
	}
}

func httpClientTimeout(client Client) (time.Duration, bool) {
	if retry, ok := client.(*retryClient); ok {
		client = retry.inner
	}
	switch c := client.(type) {
	case *openAIClient:
		return c.httpClient.Timeout, true
	case *anthropicClient:
		return c.httpClient.Timeout, true
	case *ollamaClient:
		return c.httpClient.Timeout, true
	default:
		return 0, false
	}
}
