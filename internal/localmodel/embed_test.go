package localmodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedUsesNativeEndpointWithoutTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/embed" || request.Method != http.MethodPost {
			t.Fatalf("unexpected endpoint: %s %s", request.Method, request.URL.Path)
		}
		var payload struct {
			Model    string   `json:"model"`
			Input    []string `json:"input"`
			Truncate bool     `json:"truncate"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "embeddinggemma:latest" || len(payload.Input) != 2 || payload.Truncate {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = writer.Write([]byte(`{"embeddings":[[0.1],[0.2]]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := client.Embed(context.Background(), "embeddinggemma:latest", []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || vectors[0][0] != 0.1 || vectors[1][0] != 0.2 {
		t.Fatalf("unexpected vectors: %v", vectors)
	}
}
