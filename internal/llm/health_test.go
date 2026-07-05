package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestHealthOllamaTagsCheck(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		fmt.Fprint(w, `{"models":[{"name":"qwen3:8b"}]}`)
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
		fmt.Fprint(w, `{"models":[{"name":"other:latest"}]}`)
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
			fmt.Fprint(w, `{"models":[{"name":"other:latest"}]}`)
		case "/api/pull":
			pullBody = readRequestBody(t, r)
			fmt.Fprint(w, `{"status":"success"}`)
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

func TestListOllamaModelsUsesTags(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"models":[{"name":"qwen3:8b"},{"model":"gpt-oss:20b"}]}`)
	}))
	defer srv.Close()

	models, err := EndpointHealthChecker{}.ListOllamaModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if gotPath != "/api/tags" {
		t.Fatalf("path = %q", gotPath)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "qwen3:8b"}, {ID: "gpt-oss:20b"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestHealthOpenAICompatibleModelsCheck(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"data":[{"id":"glm-test"}]}`)
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

func TestHealthCLIVersionProbe(t *testing.T) {
	runner := &fakeCLIRunner{stdout: "codex-cli 1.2.3\n"}
	report := EndpointHealthChecker{
		CLIRunner: runner,
		LookPath: func(command string) (string, error) {
			if command != "codex" {
				t.Fatalf("command = %q", command)
			}
			return "/usr/local/bin/codex", nil
		},
	}.Check(context.Background(), EndpointConfig{
		Transport: "codex-cli",
		Model:     "gpt-test",
	})
	if runner.inv.Command != "codex" || !reflect.DeepEqual(runner.inv.Args, []string{"--version"}) {
		t.Fatalf("invocation = %#v", runner.inv)
	}
	requireCheck(t, report, "installed", HealthOK)
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "auth", HealthUnknown)
	requireCheck(t, report, "model", HealthUnknown)
	requireCheck(t, report, "schema", HealthOK)
}

func TestHealthCLIMissingBinary(t *testing.T) {
	report := EndpointHealthChecker{
		LookPath: func(string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}.Check(context.Background(), EndpointConfig{Transport: "gemini-cli"})
	check := requireCheck(t, report, "installed", HealthFail)
	if !strings.Contains(check.Action, "install gemini") {
		t.Fatalf("action = %q", check.Action)
	}
	requireCheck(t, report, "auth", HealthUnknown)
	requireCheck(t, report, "model", HealthUnknown)
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
