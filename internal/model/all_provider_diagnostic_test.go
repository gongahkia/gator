package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestEveryDirectProviderPreservesSafeFailureIdentity(t *testing.T) {
	const secret = "synthetic-private-credential"
	const prompt = "PRIVATE_USER_PROMPT"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_fixture")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"` + secret + ` ` + prompt + `","code":"invalid_api_key","type":"authentication_error"}}`))
	}))
	defer server.Close()
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			config := Config{Provider: Provider(name), APIKey: secret, Model: "test-model", BaseURL: server.URL, Client: server.Client()}
			switch config.Provider {
			case OpenCode:
				config.Model = "gpt-5.6-terra"
			case OpenCodeGo:
				config.Model = "kimi-k2.6"
			case CloudflareGateway:
				config.Model = "openai/test-model"
				config.CloudflareAccountID = "fixture-account"
				config.CloudflareGatewayID = "fixture-gateway"
				config.CloudflareGatewayProtocol = "openai-responses"
			}
			assertSafeProviderFailure(t, config, secret, prompt)
		})
	}
	// Catalog and gateway routing select different wire adapters for the same provider.
	for _, item := range []struct{ model, protocol string }{
		{"claude-sonnet-4-6", ""}, {"gemini-3.1-pro", ""}, {"kimi-k2.6", ""},
		{"anthropic/test", "anthropic-messages"}, {"@cf/test", "workers-ai-chat-completions"},
	} {
		t.Run(item.model, func(t *testing.T) {
			config := Config{Provider: OpenCode, APIKey: secret, Model: item.model, BaseURL: server.URL, Client: server.Client()}
			if item.protocol != "" {
				config.Provider = CloudflareGateway
				config.CloudflareAccountID = "fixture-account"
				config.CloudflareGatewayID = "fixture-gateway"
				config.CloudflareGatewayProtocol = item.protocol
			}
			assertSafeProviderFailure(t, config, secret, prompt)
		})
	}
}

func assertSafeProviderFailure(t *testing.T, config Config, secret, prompt string) {
	t.Helper()
	backend, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}}})
	d := agent.ProviderDiagnosticFrom(err)
	if d == nil {
		t.Fatalf("missing typed diagnostic: %v", err)
	}
	if d.Provider != string(config.Provider) || d.Model != config.Model || d.Status != 401 || d.CredentialSource != "explicit" || d.RequestID != "req_fixture" || d.Method != "POST" {
		t.Fatalf("wrong diagnostic: %+v", d)
	}
	data, _ := json.Marshal(d)
	for _, private := range []string{secret, prompt, "authentication_error message"} {
		if strings.Contains(err.Error()+string(data), private) {
			t.Fatalf("private content retained: %q", private)
		}
	}
	if agent.IsTransient(err) {
		t.Fatal("401 must not become transient")
	}
}
