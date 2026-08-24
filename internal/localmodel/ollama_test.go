package localmodel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogResolvesOnlyReviewedModels(t *testing.T) {
	model, found := Resolve("qwen3-coder-30b")
	if !found || model.OllamaModel != "qwen3-coder:30b" {
		t.Fatalf("catalog model = %#v, found = %t", model, found)
	}
	if _, found := Resolve("arbitrary/model:latest"); found {
		t.Fatal("unreviewed local model was accepted")
	}
	copy := Catalog()
	copy[0].ID = "changed"
	if model, _ := Resolve("qwen2.5-coder-7b"); model.ID != "qwen2.5-coder-7b" {
		t.Fatalf("catalog was mutable: %#v", model)
	}
}

func TestClientRestrictsRuntimeToLoopbackHTTP(t *testing.T) {
	for _, value := range []string{"https://127.0.0.1:11434", "http://example.com:11434", "http://user:pass@127.0.0.1:11434", "http://127.0.0.1:0"} {
		if _, err := NewClient(value); err == nil {
			t.Fatalf("accepted unsafe runtime URL %q", value)
		}
	}
	client, err := NewClient("http://localhost:11434/")
	if err != nil || client.BaseURL() != "http://localhost:11434" || client.ChatCompletionsURL() != "http://localhost:11434/v1/chat/completions" {
		t.Fatalf("client = %#v, err = %v", client, err)
	}
}

func TestClientManagesOllamaCatalogAndModelLifecycle(t *testing.T) {
	var pulled, removed string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/version":
			_, _ = io.WriteString(writer, `{"version":"0.13.4"}`)
		case "/api/tags":
			_, _ = io.WriteString(writer, `{"models":[{"name":"qwen2.5-coder:7b","size":4700000000},{"name":"qwen2.5-coder:7b","size":1}]}`)
		case "/api/pull":
			if request.Method != http.MethodPost {
				t.Fatalf("pull method = %s", request.Method)
			}
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			pulled = body.Model
			_, _ = io.WriteString(writer, "{\"status\":\"pulling manifest\"}\n{\"status\":\"pulling layers\",\"completed\":5,\"total\":10}\n{\"status\":\"success\"}\n")
		case "/api/delete":
			if request.Method != http.MethodDelete {
				t.Fatalf("delete method = %s", request.Method)
			}
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			removed = body.Model
		default:
			t.Fatalf("unexpected endpoint %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	context := context.Background()
	version, err := client.Version(context)
	if err != nil || version != "0.13.4" {
		t.Fatalf("version = %q, err = %v", version, err)
	}
	models, err := client.Models(context)
	if err != nil || len(models) != 1 || models[0].Name != "qwen2.5-coder:7b" {
		t.Fatalf("models = %#v, err = %v", models, err)
	}
	model, _ := Resolve("qwen2.5-coder-7b")
	var statuses []string
	if err := client.Pull(context, model, func(progress Progress) { statuses = append(statuses, progress.Status) }); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if pulled != model.OllamaModel || strings.Join(statuses, ",") != "pulling manifest,pulling layers,success" {
		t.Fatalf("pull = %q, statuses = %#v", pulled, statuses)
	}
	if err := client.Remove(context, model); err != nil || removed != model.OllamaModel {
		t.Fatalf("remove = %q, err = %v", removed, err)
	}
}
