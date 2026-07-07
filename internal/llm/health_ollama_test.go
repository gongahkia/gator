package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthOllamaTagsCheck(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		mustWriteResponse(t, w, `{"models":[{"name":"qwen3:8b"}]}`)
	}))
	defer srv.Close()

	report := EndpointHealthChecker{}.Check(context.Background(), EndpointConfig{
		Transport: "ollama",
		BaseURL:   srv.URL,
		Model:     "qwen3:8b",
	})
	if gotPath != "/api/tags" || gotMethod != http.MethodGet {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "auth", HealthOK)
	requireCheck(t, report, "model", HealthOK)
	requireCheck(t, report, "schema", HealthUnknown)
}

func TestHealthOllamaMissingModelSuggestsPull(t *testing.T) {
	pulls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pulls++
		mustWriteResponse(t, w, `{"models":[{"name":"other:latest"}]}`)
	}))
	defer srv.Close()

	report := EndpointHealthChecker{}.Check(context.Background(), EndpointConfig{
		Transport: "ollama",
		BaseURL:   srv.URL,
		Model:     "qwen3:8b",
	})
	check := requireCheck(t, report, "model", HealthFail)
	if check.Action != "ollama pull qwen3:8b" {
		t.Fatalf("action = %q", check.Action)
	}
	if pulls != 1 {
		t.Fatalf("requests = %d", pulls)
	}
}

func TestHealthOllamaAutoPullsWhenExplicitlyEnabled(t *testing.T) {
	var pullBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			mustWriteResponse(t, w, `{"models":[{"name":"other:latest"}]}`)
		case "/api/pull":
			pullBody = readRequestBody(t, r)
			mustWriteResponse(t, w, `{"status":"success"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	report := EndpointHealthChecker{AutoPullOllama: true}.Check(context.Background(), EndpointConfig{
		Transport: "ollama",
		BaseURL:   srv.URL,
		Model:     "qwen3:8b",
	})
	check := requireCheck(t, report, "model", HealthOK)
	if check.Detail != "pulled qwen3:8b" {
		t.Fatalf("detail = %q", check.Detail)
	}
	if !strings.Contains(pullBody, `"model":"qwen3:8b"`) || !strings.Contains(pullBody, `"stream":false`) {
		t.Fatalf("pull body = %q", pullBody)
	}
}

func TestHealthOllamaSchemaSmoke(t *testing.T) {
	var chatBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			mustWriteResponse(t, w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			chatBody = readRequestBody(t, r)
			mustWriteResponse(t, w, `{"message":{"content":"{\"ok\":true}"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	report := EndpointHealthChecker{SchemaSmokeOllama: true}.Check(context.Background(), EndpointConfig{
		Transport: "ollama",
		BaseURL:   srv.URL,
		Model:     "qwen3:8b",
	})
	requireCheck(t, report, "schema", HealthOK)
	if !strings.Contains(chatBody, `"format"`) || !strings.Contains(chatBody, `"additionalProperties":false`) {
		t.Fatalf("chat body = %q", chatBody)
	}
}

func TestHealthOllamaSchemaSmokeFailsOnMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			mustWriteResponse(t, w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			mustWriteResponse(t, w, `{"message":{"content":"{\"ok\":\"no\"}"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	report := EndpointHealthChecker{SchemaSmokeOllama: true}.Check(context.Background(), EndpointConfig{
		Transport: "ollama",
		BaseURL:   srv.URL,
		Model:     "qwen3:8b",
	})
	check := requireCheck(t, report, "schema", HealthFail)
	if !strings.Contains(check.Detail, "schema smoke response mismatch") {
		t.Fatalf("detail = %q", check.Detail)
	}
}
