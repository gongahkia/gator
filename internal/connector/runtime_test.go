package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
)

func TestRuntimeFetchesAuthenticatedJSONWithProvenance(t *testing.T) {
	const token = "private-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"value":42}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "metrics", Name: "Metrics", Kind: KindHTTPJSON, Resource: server.URL, Authentication: AuthBearer}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(descriptor.CredentialRef(), auth.Credential{Type: "bearer_token", Access: token}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 1, 2, 3, 0, time.UTC)
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client(), Now: func() time.Time { return now }}
	result, err := runtime.Invoke(context.Background(), action.Inspect, "metrics", "fetch", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Data) != `{"value":42}` || result.Provenance.ConnectorID != "metrics" || result.Provenance.SHA256 == "" || !result.Provenance.RetrievedAt.Equal(now) {
		t.Fatalf("result = %#v", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), token) {
		t.Fatal("connector result leaked bearer token")
	}
}

func TestRuntimeRejectsRedirectsInvalidInputAndNonJSON(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, &http.Request{}, "https://example.com/elsewhere", http.StatusFound)
	}))
	defer redirect.Close()
	nonJSON := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("not json"))
	}))
	defer nonJSON.Close()
	for _, resource := range []string{redirect.URL, nonJSON.URL} {
		descriptor := Descriptor{Version: DescriptorVersion, ID: "source", Name: "Source", Kind: KindHTTPJSON, Resource: resource, Authentication: AuthNone}
		registry, err := NewRegistry([]Descriptor{descriptor})
		if err != nil {
			t.Fatal(err)
		}
		runtime := Runtime{Registry: registry}
		if _, err := runtime.Invoke(context.Background(), action.Draft, "source", "fetch", json.RawMessage(`{}`)); err == nil {
			t.Fatalf("unsafe response from %s was accepted", resource)
		}
		if _, err := runtime.Invoke(context.Background(), action.Draft, "source", "fetch", json.RawMessage(`{"url":"https://attacker.example"}`)); err == nil {
			t.Fatal("model-supplied connector redirect input was accepted")
		}
	}
}

func TestRuntimeRequiresConfiguredCredential(t *testing.T) {
	descriptor := Descriptor{Version: DescriptorVersion, ID: "private", Name: "Private", Kind: KindHTTPJSON, Resource: "https://example.com/data", Authentication: AuthBearer}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Runtime{Registry: registry, Credentials: credentials}).Invoke(context.Background(), action.Inspect, "private", "fetch", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("credential error = %v", err)
	}
}
