package faketest

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerServesCannedProviderFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "openai-chat.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	defer server.Close()
	server.Respond("fixture-marker", http.StatusOK, string(fixture))
	response := post(t, server.URL+"/chat/completions", `{"model":"test","prompt":"fixture-marker"}`)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(body, fixture) {
		t.Fatalf("response = %d %s", response.StatusCode, body)
	}
	request := server.LastRequest()
	if request.Path != "/chat/completions" || !strings.Contains(request.Body, "fixture-marker") {
		t.Fatalf("request = %#v", request)
	}
}

func TestServerReturnsFailureWhenResponsesAreExhausted(t *testing.T) {
	server := NewServer()
	defer server.Close()
	response := post(t, server.URL+"/api/chat", `{}`)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusInternalServerError || string(body) != `{"error":"no canned response"}` {
		t.Fatalf("response = %d %s", response.StatusCode, body)
	}
	if len(server.Requests()) != 1 {
		t.Fatalf("requests = %#v", server.Requests())
	}
}

func TestRequestSnapshotsAreIndependent(t *testing.T) {
	server := NewServer()
	defer server.Close()
	server.Respond("", http.StatusOK, `{}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test", "original")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	last := server.LastRequest()
	last.Header.Set("X-Test", "changed")
	if got := server.LastRequest().Header.Get("X-Test"); got != "original" {
		t.Fatalf("recorded header = %q", got)
	}
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	response, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return response
}
