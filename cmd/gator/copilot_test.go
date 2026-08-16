package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExchangeCopilotTokenDiscoversEnabledToolModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/token":
			if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer github-token" || request.Header.Get("Copilot-Integration-Id") != "vscode-chat" {
				t.Fatalf("token request = %#v", request)
			}
			_, _ = fmt.Fprintf(writer, `{"token":"inference-token","expires_at":%d}`, time.Now().Add(time.Hour).Unix())
		case "/api/models":
			if request.Header.Get("Authorization") != "Bearer inference-token" || request.Header.Get("X-GitHub-Api-Version") != copilotAPIVersion {
				t.Fatalf("model request headers = %#v", request.Header)
			}
			_, _ = fmt.Fprint(writer, `{"data":[{"id":"gpt-enabled","model_picker_enabled":true,"policy":{"state":"enabled"},"capabilities":{"supports":{"tool_calls":true}}},{"id":"no-tools","model_picker_enabled":true,"policy":{"state":"enabled"},"capabilities":{"supports":{"tool_calls":false}}},{"id":"disabled","model_picker_enabled":true,"policy":{"state":"disabled"},"capabilities":{"supports":{"tool_calls":true}}}]}`)
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	credential, err := exchangeCopilotToken(context.Background(), copilotEndpoints{tokenURL: server.URL + "/token", apiURL: server.URL + "/api"}, "github-token")
	if err != nil {
		t.Fatalf("exchange Copilot token: %v", err)
	}
	if credential.Access != "inference-token" || credential.Refresh != "github-token" || credential.Extra["base_url"] != server.URL+"/api" || credential.Extra["available_model_ids"] != "gpt-enabled" || credential.Expired(time.Now()) {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestCopilotAPIURLUsesTrustedProxyEndpoint(t *testing.T) {
	if got, want := copilotAPIURL("tid=x;proxy-ep=proxy.enterprise.example;exp=x", "https://fallback.example"), "https://api.enterprise.example"; got != want {
		t.Fatalf("Copilot API URL = %q, want %q", got, want)
	}
	if got := copilotAPIURL("proxy-ep=unsafe.example/path", "https://fallback.example"); got != "https://fallback.example" {
		t.Fatalf("unsafe Copilot endpoint = %q", got)
	}
	if strings.TrimSpace(copilotAPIURL("", "https://fallback.example")) == "" {
		t.Fatal("Copilot fallback endpoint was empty")
	}
}
