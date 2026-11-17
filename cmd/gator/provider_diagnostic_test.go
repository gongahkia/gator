package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

func TestCustomAndManagedLocalKeepDiagnosticIdentityThroughMetering(t *testing.T) {
	useGenerousLocalModelHost(t)
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("FIXTURE_API_KEY", "synthetic-private-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"synthetic-private-key PRIVATE_PROMPT"}}`))
	}))
	defer server.Close()
	for _, test := range []struct{ id, keyEnv, source string }{{"custom-fixture", "FIXTURE_API_KEY", "environment"}, {"keyless-fixture", "", "none"}, {localmodel.ProviderID, "", "none"}} {
		t.Run(test.id, func(t *testing.T) {
			provider := config.CustomProvider{ID: test.id, APIKeyEnv: test.keyEnv, BaseURL: server.URL + "/v1/chat/completions", Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b"}
			executor, err := newCustomExecutorConfiguration(config.Settings{}, provider, "", "")
			if err != nil {
				t.Fatal(err)
			}
			_, err = executor.Model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "PRIVATE_PROMPT"}}})
			d := agent.ProviderDiagnosticFrom(err)
			if d == nil || d.Provider != test.id || d.Status != 401 || d.CredentialSource != test.source {
				t.Fatalf("wrong custom/local diagnostic: %v", err)
			}
			if strings.Contains(err.Error(), "synthetic-private-key") || strings.Contains(err.Error(), "PRIVATE_PROMPT") {
				t.Fatal("private content retained")
			}
		})
	}
}
