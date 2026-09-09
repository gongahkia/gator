package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestRuntimePreparesAndPublishesExactApprovedJSON(t *testing.T) {
	t.Parallel()
	const token = "private-token"
	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer "+token || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request = %s auth=%q content-type=%q", request.Method, request.Header.Get("Authorization"), request.Header.Get("Content-Type"))
		}
		var err error
		received, err = io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "release-hook", Name: "Release hook", Kind: KindHTTPWebhook, Resource: server.URL, Authentication: AuthBearer}
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
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "publish", json.RawMessage(`{"payload": {"title": "v1", "ready": true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Proposal.Target != server.URL || prepared.Proposal.Preview != `{"title":"v1","ready":true}` || prepared.Proposal.PayloadSHA256 == "" {
		t.Fatalf("proposal = %#v", prepared.Proposal)
	}
	if err := runtime.ExecutePrepared(context.Background(), action.Draft, prepared); err == nil {
		t.Fatal("draft mode executed a prepared action")
	}
	if received != nil {
		t.Fatal("request was sent before act-mode execution")
	}
	if err := runtime.ExecutePrepared(context.Background(), action.Act, prepared); err != nil {
		t.Fatal(err)
	}
	if string(received) != prepared.Proposal.Preview {
		t.Fatalf("received payload = %q", received)
	}
}

func TestRuntimeRejectsTamperedPreparedActionAndMarksRemoteErrorUncertain(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "hook", Name: "Hook", Kind: KindHTTPWebhook, Resource: server.URL, Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, HTTPClient: server.Client()}
	prepared, err := runtime.PrepareAction(descriptor.ID, "publish", json.RawMessage(`{"payload":{"message":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	tampered := prepared
	tampered.Proposal.PayloadSHA256 = strings.Repeat("0", 64)
	if err := runtime.ExecutePrepared(context.Background(), action.Act, tampered); err == nil || strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("tampered proposal reached network: %v", err)
	}
	err = runtime.ExecutePrepared(context.Background(), action.Act, prepared)
	var uncertain interface{ OutcomeUncertain() bool }
	if err == nil || !errors.As(err, &uncertain) || !uncertain.OutcomeUncertain() {
		t.Fatalf("remote action error = %v", err)
	}
}

func TestRuntimeRejectsUnboundedOrRedirectableActionInput(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor{Version: DescriptorVersion, ID: "hook", Name: "Hook", Kind: KindHTTPWebhook, Resource: "https://example.com/hook", Authentication: AuthNone}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry}
	for _, input := range []string{
		`{}`,
		`{"payload":"text"}`,
		`{"payload":{},"url":"https://attacker.example"}`,
		`{"payload":{},"extra":true}`,
	} {
		if _, err := runtime.PrepareAction(descriptor.ID, "publish", json.RawMessage(input)); err == nil {
			t.Fatalf("unsafe action input accepted: %s", input)
		}
	}
}
