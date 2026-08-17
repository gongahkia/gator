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

func TestBedrockUsesAmbientCredentialsToSigV4SignRuntimeRequests(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "BEDROCKKEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "bedrock-secret")
	t.Setenv("AWS_REGION", "us-west-2")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "" || !strings.Contains(request.Header.Get("Authorization"), "Credential=BEDROCKKEY/") || strings.Contains(request.Header.Get("Authorization"), "Bearer") {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Model != "us.anthropic.claude-sonnet-4-6" {
			t.Fatalf("body = %#v, err = %v", body, err)
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer server.Close()

	backend, err := New(Config{Provider: AmazonBedrock, Model: "us.anthropic.claude-sonnet-4-6", BaseURL: server.URL, Client: server.Client()})
	if err != nil {
		t.Fatalf("new Bedrock backend: %v", err)
	}
	turn, err := backend.Model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
	if err != nil || turn.Text != "done" {
		t.Fatalf("complete = %#v, %v", turn, err)
	}
}
