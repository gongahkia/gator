package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestListOllamaModelsUsesTags(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		mustWriteResponse(t, w, `{"models":[{"name":"qwen3:8b"},{"model":"gpt-oss:20b"}]}`)
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

func TestGetModelInfosParsesIDNameAndModelFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mustWriteResponse(t, w, `{"data":[{"id":"id-a"},{"name":"name-b"},{"model":"model-c"}]}`)
	}))
	defer srv.Close()

	models, err := EndpointHealthChecker{}.getModelInfos(context.Background(), http.MethodGet, srv.URL, nil, "data")
	if err != nil {
		t.Fatalf("get models: %v", err)
	}
	want := []ModelInfo{{ID: "id-a"}, {ID: "name-b"}, {ID: "model-c"}}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %#v; want %#v", models, want)
	}
}
