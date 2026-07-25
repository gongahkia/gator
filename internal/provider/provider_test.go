package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

func TestOpenAIResponsesAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing auth")
		}
		w.Header().Set("x-ratelimit-remaining-requests", "7")
		w.Header().Set("x-ratelimit-reset-requests", "1s")
		_, _ = io.WriteString(w, `{"id":"resp_1","output_text":"planned"}`)
	}))
	defer server.Close()
	t.Setenv("TEST_OPENAI", "test-key")
	invoker := Invoker{HTTPClient: server.Client()}
	result, err := invoker.Invoke(context.Background(), config.Provider{ID: "openai", Kind: "openai_responses", Model: "model", BaseURL: server.URL, CredentialEnv: "TEST_OPENAI"}, Request{RunID: "run", Stage: domain.StagePlanner, Prompt: "plan"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "planned" || result.Provider != "openai" {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.RateLimit.RemainingRequests == nil || *result.RateLimit.RemainingRequests != 7 || result.RateLimit.ResetAt == nil { t.Fatalf("rate limit=%#v", result.RateLimit) }
	_ = os.Unsetenv("TEST_OPENAI")
}

func TestMissingCredentialFailsClosed(t *testing.T) {
	invoker := Invoker{}
	_, err := invoker.Invoke(context.Background(), config.Provider{ID: "openai", Kind: "openai_responses", Model: "model", BaseURL: "https://example.test", CredentialEnv: "MISSING_TEST_KEY"}, Request{})
	if err == nil {
		t.Fatal("expected credential failure")
	}
}
