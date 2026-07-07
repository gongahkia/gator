package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestListEndpointModelsOpenAILocal(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		mustWriteResponse(t, w, `{"data":[{"id":"local-a"},{"id":"local-b"}]}`)
	}))
	defer srv.Close()

	models, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "openai",
		BaseURL:   srv.URL,
	}, ModelListOptions{})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if auth != "" {
		t.Fatalf("authorization = %q", auth)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "local-a"}, {ID: "local-b"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestListEndpointModelsAnthropic(t *testing.T) {
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

	models, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "anthropic",
		BaseURL:   srv.URL,
		APIKey:    "key",
	}, ModelListOptions{})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if apiKey != "key" || version != "2023-06-01" {
		t.Fatalf("headers = key:%q version:%q", apiKey, version)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "claude-test"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestListEndpointModelsOpenCode(t *testing.T) {
	runner := &fakeCLIRunner{stdout: "anthropic/claude-sonnet\nopenai/gpt-5\n"}
	models, err := EndpointHealthChecker{CLIRunner: runner}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "opencode-cli",
	}, ModelListOptions{Provider: "anthropic"})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if runner.inv.Command != "opencode" || !reflect.DeepEqual(runner.inv.Args, []string{"models", "anthropic"}) {
		t.Fatalf("invocation = %#v", runner.inv)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "anthropic/claude-sonnet"}, {ID: "openai/gpt-5"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestListEndpointModelsAiderRequiresQuery(t *testing.T) {
	_, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "aider-cli",
	}, ModelListOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires --query") {
		t.Fatalf("err = %v", err)
	}
}

func TestListEndpointModelsAider(t *testing.T) {
	runner := &fakeCLIRunner{stdout: "- openai/gpt-4o\n- openai/gpt-5\n"}
	models, err := EndpointHealthChecker{CLIRunner: runner}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "aider-cli",
	}, ModelListOptions{Query: "gpt"})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if runner.inv.Command != "aider" || !reflect.DeepEqual(runner.inv.Args, []string{"--list-models", "gpt"}) {
		t.Fatalf("invocation = %#v", runner.inv)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "openai/gpt-4o"}, {ID: "openai/gpt-5"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestListEndpointModelsCursor(t *testing.T) {
	runner := &fakeCLIRunner{stdout: "gpt-5\nclaude-sonnet-4.6\n"}
	models, err := EndpointHealthChecker{CLIRunner: runner}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "cursor-cli",
	}, ModelListOptions{})
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if runner.inv.Command != "cursor-agent" || !reflect.DeepEqual(runner.inv.Args, []string{"models"}) {
		t.Fatalf("invocation = %#v", runner.inv)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "gpt-5"}, {ID: "claude-sonnet-4.6"}}) {
		t.Fatalf("models = %#v", models)
	}
}

func TestListEndpointModelsGooseUnsupportedDiagnostic(t *testing.T) {
	_, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "goose-cli",
	}, ModelListOptions{})
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_PROVIDER") {
		t.Fatalf("err = %v", err)
	}
}

func TestListEndpointModelsQwenUnsupportedDiagnostic(t *testing.T) {
	_, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "qwen-cli",
	}, ModelListOptions{})
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_MODEL") {
		t.Fatalf("err = %v", err)
	}
}

func TestListEndpointModelsUnsupportedTransport(t *testing.T) {
	_, err := EndpointHealthChecker{}.ListEndpointModels(context.Background(), EndpointConfig{
		Transport: "gemini-cli",
	}, ModelListOptions{})
	if err == nil || !strings.Contains(err.Error(), "model listing unsupported") {
		t.Fatalf("err = %v", err)
	}
}
