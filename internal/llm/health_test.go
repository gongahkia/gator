package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthOpenAICompatibleModelsCheck(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		mustWriteResponse(t, w, `{"data":[{"id":"glm-test"}]}`)
	}))
	defer srv.Close()

	report := EndpointHealthChecker{}.Check(context.Background(), EndpointConfig{
		Transport: "openai",
		BaseURL:   srv.URL,
		APIKey:    "key",
		Model:     "glm-test",
	})
	if auth != "Bearer key" {
		t.Fatalf("authorization = %q", auth)
	}
	requireCheck(t, report, "auth", HealthOK)
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "model", HealthOK)
	requireCheck(t, report, "schema", HealthUnknown)
}

func TestHealthOpenAILocalNoKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		mustWriteResponse(t, w, `{"data":[{"id":"local-model"}]}`)
	}))
	defer srv.Close()

	report := EndpointHealthChecker{}.Check(context.Background(), EndpointConfig{
		Transport: "openai",
		BaseURL:   srv.URL,
		Model:     "local-model",
	})
	check := requireCheck(t, report, "auth", HealthOK)
	if !strings.Contains(check.Detail, "local") {
		t.Fatalf("detail = %q", check.Detail)
	}
	if auth != "" {
		t.Fatalf("authorization = %q", auth)
	}
	requireCheck(t, report, "model", HealthOK)
}

func TestHealthAnthropicModelsCheck(t *testing.T) {
	var apiKey, version string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		apiKey = r.Header.Get("x-api-key")
		version = r.Header.Get("anthropic-version")
		mustWriteResponse(t, w, `{"data":[{"id":"claude-test"}]}`)
	}))
	defer srv.Close()

	report := EndpointHealthChecker{}.Check(context.Background(), EndpointConfig{
		Transport: "anthropic",
		BaseURL:   srv.URL,
		APIKey:    "key",
		Model:     "claude-test",
	})
	if apiKey != "key" || version != "2023-06-01" {
		t.Fatalf("headers = key:%q version:%q", apiKey, version)
	}
	requireCheck(t, report, "auth", HealthOK)
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "model", HealthOK)
	requireCheck(t, report, "schema", HealthUnknown)
}

func requireCheck(t *testing.T, report HealthReport, name string, status HealthStatus) HealthCheck {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			if check.Status != status {
				t.Fatalf("%s status = %q; want %q; report = %#v", name, check.Status, status, report)
			}
			return check
		}
	}
	t.Fatalf("missing %s in %#v", name, report)
	return HealthCheck{}
}

func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func mustWriteResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := fmt.Fprint(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
