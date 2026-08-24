package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

func TestLocalUseConfiguresNativeProviderForInstalledCuratedModel(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	server := newLocalModelServer(t)
	defer server.Close()
	var output bytes.Buffer
	if err := localCommand([]string{"use", "qwen2.5-coder-7b", "--url", server.URL}, &output); err != nil {
		t.Fatalf("local use: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	provider, found := configuredCustomProvider(settings, localmodel.ProviderID)
	if !found || provider.BaseURL != server.URL+"/v1/chat/completions" || provider.DefaultModel != "qwen2.5-coder:7b" || settings.Defaults.Provider != localmodel.ProviderID || settings.Defaults.Model != "qwen2.5-coder:7b" {
		t.Fatalf("local settings = %#v", settings)
	}
	executor, err := newExecutor(localmodel.ProviderID, "", "")
	if err != nil || executor.Model == nil {
		t.Fatalf("local executor = %#v, err = %v", executor, err)
	}
	if !strings.Contains(output.String(), "Native TUI, run, resume") {
		t.Fatalf("local use output = %q", output.String())
	}
	output.Reset()
	if err := doctor([]string{"--provider", localmodel.ProviderID}, &output); err != nil {
		t.Fatalf("doctor local provider: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Authentication (no API key): not required") || !strings.Contains(got, "gator local status") {
		t.Fatalf("local doctor output = %q", got)
	}
}

func TestLocalPullRequiresConfirmationAndUsesCuratedTag(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	server := newLocalModelServer(t)
	defer server.Close()
	var output bytes.Buffer
	err := localCommand([]string{"pull", "qwen3-coder-30b", "--url", server.URL}, &output)
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "19 GB") {
		t.Fatalf("unconfirmed local pull = %v", err)
	}
	if err := localCommand([]string{"pull", "qwen3-coder-30b", "--yes", "--url", server.URL}, &output); err != nil {
		t.Fatalf("confirmed local pull: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "qwen3-coder:30b") || !strings.Contains(got, "pulling layers (50%)") {
		t.Fatalf("local pull output = %q", got)
	}
}

func TestLocalRemoveUpdatesSelectedProviderConfiguration(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	server := newLocalModelServer(t)
	defer server.Close()
	var output bytes.Buffer
	if err := localCommand([]string{"use", "qwen2.5-coder-7b", "--url", server.URL}, &output); err != nil {
		t.Fatal(err)
	}
	if err := localCommand([]string{"remove", "qwen2.5-coder-7b", "--yes", "--url", server.URL}, &output); err != nil {
		t.Fatalf("local remove: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, found := configuredCustomProvider(settings, localmodel.ProviderID); found || settings.Defaults.Provider != "" || settings.Defaults.Model != "" {
		t.Fatalf("local settings after remove = %#v", settings)
	}
}

func TestLocalRejectsRemoteRuntimeURL(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	err := localCommand([]string{"list", "--url", "http://models.example.com:11434"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("remote local runtime error = %v", err)
	}
}

func newLocalModelServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/version":
			_, _ = io.WriteString(writer, `{"version":"test"}`)
		case "/api/tags":
			_, _ = io.WriteString(writer, `{"models":[{"name":"qwen2.5-coder:7b","size":4700000000}]}`)
		case "/api/pull":
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Model != "qwen3-coder:30b" {
				t.Fatalf("pull model = %q", body.Model)
			}
			_, _ = io.WriteString(writer, "{\"status\":\"pulling layers\",\"completed\":5,\"total\":10}\n{\"status\":\"success\"}\n")
		case "/api/delete":
			if request.Method != http.MethodDelete {
				t.Fatalf("delete method = %s", request.Method)
			}
		default:
			t.Fatalf("unexpected local endpoint %s", request.URL.Path)
		}
	}))
}
