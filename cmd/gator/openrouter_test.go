package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestOpenRouterLoginMintsAndStoresUserAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/auth/keys" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode key exchange: %v", err)
		}
		if body["code"] != "authorization-code" || body["code_verifier"] == "" || body["code_challenge_method"] != "S256" {
			t.Fatalf("key exchange body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"key":"openrouter-key"}`))
	}))
	defer server.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve callback port: %v", err)
	}
	redirectURL := "http://" + listener.Addr().String() + "/callback"
	if err := listener.Close(); err != nil {
		t.Fatalf("release callback port: %v", err)
	}
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_OPENROUTER_BASE_URL", server.URL)
	t.Setenv("GATOR_OPENROUTER_OAUTH_REDIRECT_URL", redirectURL)
	login, err := beginOpenRouterLogin()
	if err != nil {
		t.Fatalf("begin OpenRouter login: %v", err)
	}
	authorize, err := url.Parse(login.URL())
	if err != nil || authorize.Path != "/auth" || authorize.Query().Get("callback_url") != redirectURL || authorize.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL = %q, error = %v", login.URL(), err)
	}
	go func() {
		_, _ = http.Get(redirectURL + "?code=authorization-code")
	}()
	context, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := login.Complete(context); err != nil {
		t.Fatalf("complete OpenRouter login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	credential, found, err := credentials.Read("openrouter")
	if err != nil || !found || !credential.IsAPIKey() || credential.Key != "openrouter-key" {
		t.Fatalf("OpenRouter credential = %#v, found=%v, error=%v", credential, found, err)
	}
}
