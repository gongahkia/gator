package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestCloudflareGatewayRoutesConfiguredNativeProtocols(t *testing.T) {
	for _, test := range []struct {
		name         string
		protocol     string
		model        string
		path         string
		bodyField    string
		responseBody string
	}{
		{
			name:         "OpenAI Responses",
			protocol:     "openai-responses",
			model:        "openai/gpt-5.6",
			path:         "/ai/v1/responses",
			bodyField:    "input",
			responseBody: `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`,
		},
		{
			name:         "Anthropic Messages",
			protocol:     "anthropic-messages",
			model:        "anthropic/claude-sonnet-4-6",
			path:         "/ai/v1/messages",
			bodyField:    "messages",
			responseBody: `{"content":[{"type":"text","text":"done"}]}`,
		},
		{
			name:         "Workers AI Chat Completions",
			protocol:     "workers-ai-chat-completions",
			model:        "@cf/moonshotai/kimi-k2.6",
			path:         "/ai/v1/chat/completions",
			bodyField:    "messages",
			responseBody: `{"choices":[{"message":{"content":"done"}}]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != test.path {
					t.Fatalf("request = %s %s", request.Method, request.URL.Path)
				}
				if request.Header.Get("Authorization") != "Bearer cloudflare-token" || request.Header.Get("Cf-Aig-Gateway-Id") != "gator-gateway" {
					t.Fatalf("headers = %#v", request.Header)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body[test.bodyField] == nil || body["model"] == nil {
					t.Fatalf("body = %#v, err = %v", body, err)
				}
				_, _ = io.WriteString(writer, test.responseBody)
			}))
			defer server.Close()

			backend, err := New(Config{
				Provider:                  CloudflareGateway,
				APIKey:                    "cloudflare-token",
				Model:                     test.model,
				BaseURL:                   server.URL + "/ai/v1",
				CloudflareAccountID:       "account-123",
				CloudflareGatewayID:       "gator-gateway",
				CloudflareGatewayProtocol: test.protocol,
				Client:                    server.Client(),
			})
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			turn, err := backend.Model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
			if err != nil || turn.Text != "done" {
				t.Fatalf("complete = %#v, %v", turn, err)
			}
		})
	}
}

func TestCloudflareGatewayRejectsAmbiguousOrMismatchedConfiguration(t *testing.T) {
	base := Config{
		Provider:            CloudflareGateway,
		APIKey:              "cloudflare-token",
		Model:               "openai/gpt-5.6",
		CloudflareAccountID: "account-123",
		CloudflareGatewayID: "gator-gateway",
	}
	for _, test := range []struct {
		name   string
		config Config
		want   string
	}{
		{name: "protocol is required", config: base, want: "GATOR_CLOUDFLARE_GATEWAY_PROTOCOL"},
		{name: "model and protocol disagree", config: func() Config { copy := base; copy.CloudflareGatewayProtocol = "anthropic-messages"; return copy }(), want: "incompatible"},
		{name: "gateway identifier is required", config: func() Config {
			copy := base
			copy.CloudflareGatewayID = ""
			copy.CloudflareGatewayProtocol = "openai-responses"
			return copy
		}(), want: "CLOUDFLARE_AI_GATEWAY_ID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("configuration error = %v", err)
			}
		})
	}
}
