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

func TestHealthOllamaSchemaSmoke(t *testing.T) {
	var chatBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			fmt.Fprint(w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			chatBody = readRequestBody(t, r)
			fmt.Fprint(w, `{"message":{"content":"{\"ok\":true}"}}`)
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
			fmt.Fprint(w, `{"models":[{"name":"qwen3:8b"}]}`)
		case "/api/chat":
			fmt.Fprint(w, `{"message":{"content":"{\"ok\":\"no\"}"}}`)
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

func TestHealthOpenAILocalNoKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"data":[{"id":"local-model"}]}`)
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

func TestHealthCLIVersionProbe(t *testing.T) {
	runner := &sequenceCLIRunner{results: []cliResult{
		{Stdout: "codex-cli 1.2.3\n"},
		{Stdout: "--sandbox --model --output-schema --cd --ephemeral"},
	}}
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
	if len(runner.invocations) != 2 {
		t.Fatalf("invocations = %#v", runner.invocations)
	}
	if runner.invocations[0].Command != "codex" || !reflect.DeepEqual(runner.invocations[0].Args, []string{"--version"}) {
		t.Fatalf("version invocation = %#v", runner.invocations[0])
	}
	if !reflect.DeepEqual(runner.invocations[1].Args, []string{"exec", "--help"}) {
		t.Fatalf("help invocation = %#v", runner.invocations[1])
	}
	requireCheck(t, report, "installed", HealthOK)
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "capabilities", HealthOK)
	requireCheck(t, report, "auth", HealthUnknown)
	requireCheck(t, report, "model", HealthUnknown)
	requireCheck(t, report, "schema", HealthOK)
}

func TestHealthCLIMissingCapabilityFlag(t *testing.T) {
	runner := &sequenceCLIRunner{results: []cliResult{
		{Stdout: "2.1.119\n"},
		{Stdout: "--print --permission-mode --output-format --model"},
	}}
	report := EndpointHealthChecker{
		CLIRunner: runner,
		LookPath: func(command string) (string, error) {
			return "/usr/local/bin/" + command, nil
		},
	}.Check(context.Background(), EndpointConfig{Transport: "claude-cli"})
	check := requireCheck(t, report, "capabilities", HealthFail)
	if !strings.Contains(check.Detail, "--json-schema") || !strings.Contains(check.Action, "upgrade claude") {
		t.Fatalf("check = %#v", check)
	}
}

func TestHealthNewCLICapabilitySpecs(t *testing.T) {
	tests := []struct {
		transport string
		binary    string
		helpArgs  []string
		help      string
	}{
		{
			transport: "aider-cli",
			binary:    "aider",
			helpArgs:  []string{"--help"},
			help:      "--message --dry-run --no-git --no-auto-commits --no-auto-lint --no-auto-test --no-suggest-shell-commands --model",
		},
		{
			transport: "goose-cli",
			binary:    "goose",
			helpArgs:  []string{"run", "--help"},
			help:      "--no-session --quiet --output-format --no-profile --max-turns --provider --model --text",
		},
		{
			transport: "qwen-cli",
			binary:    "qwen",
			helpArgs:  []string{"--help"},
			help:      "--prompt --approval-mode --output-format --model",
		},
		{
			transport: "cursor-cli",
			binary:    "cursor-agent",
			helpArgs:  []string{"--help"},
			help:      "--print --output-format --mode --model",
		},
	}
	for _, tc := range tests {
		t.Run(tc.transport, func(t *testing.T) {
			runner := &sequenceCLIRunner{results: []cliResult{
				{Stdout: tc.binary + " 1.0\n"},
				{Stdout: tc.help},
			}}
			report := EndpointHealthChecker{
				CLIRunner: runner,
				LookPath: func(command string) (string, error) {
					if command != tc.binary {
						t.Fatalf("command = %q; want %q", command, tc.binary)
					}
					return "/usr/local/bin/" + command, nil
				},
			}.Check(context.Background(), EndpointConfig{Transport: tc.transport, Model: "test-model"})
			if len(runner.invocations) != 2 {
				t.Fatalf("invocations = %#v", runner.invocations)
			}
			if !reflect.DeepEqual(runner.invocations[1].Args, tc.helpArgs) {
				t.Fatalf("help args = %#v; want %#v", runner.invocations[1].Args, tc.helpArgs)
			}
			requireCheck(t, report, "installed", HealthOK)
			requireCheck(t, report, "running", HealthOK)
			requireCheck(t, report, "capabilities", HealthOK)
			requireCheck(t, report, "model", HealthUnknown)
		})
	}
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

type sequenceCLIRunner struct {
	invocations []cliInvocation
	results     []cliResult
	errs        []error
}

func (r *sequenceCLIRunner) Run(_ context.Context, inv cliInvocation) (cliResult, error) {
	r.invocations = append(r.invocations, inv)
	var result cliResult
	if len(r.results) > 0 {
		result = r.results[0]
		r.results = r.results[1:]
	}
	var err error
	if len(r.errs) > 0 {
		err = r.errs[0]
		r.errs = r.errs[1:]
	}
	return result, err
}
