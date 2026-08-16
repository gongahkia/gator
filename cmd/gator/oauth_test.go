package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
)

func TestRefreshProviderCredentialRefreshesExpiringOAuthToken(t *testing.T) {
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	now := time.Now()
	if err := credentials.Put("codex", auth.Credential{Type: "oauth", Access: "old-access", Refresh: "refresh-token", Expires: now.Add(time.Minute).UnixMilli(), Extra: map[string]string{"chatgpt_account_id": "account_123"}}); err != nil {
		t.Fatalf("store credential: %v", err)
	}
	tokens := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if request.PostForm.Get("grant_type") != "refresh_token" || request.PostForm.Get("refresh_token") != "refresh-token" {
			t.Fatalf("refresh form = %#v", request.PostForm)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "new-access", "expires_in": 3600})
	}))
	defer tokens.Close()
	flowFor := func(provider model.Provider) (auth.BrowserFlow, error) {
		if provider != model.Codex {
			t.Fatalf("provider = %q", provider)
		}
		return auth.BrowserFlow{ClientID: "gator-client", TokenURL: tokens.URL}, nil
	}
	if err := refreshProviderCredential(context.Background(), model.Codex, credentials, flowFor, now); err != nil {
		t.Fatalf("refresh provider credential: %v", err)
	}
	credential, found, err := credentials.Read("codex")
	if err != nil || !found || credential.Access != "new-access" || credential.Refresh != "refresh-token" || credential.Extra["chatgpt_account_id"] != "account_123" || credential.Expired(now) {
		t.Fatalf("credential = %#v, found=%v, error=%v", credential, found, err)
	}
}

func TestRefreshProviderCredentialLeavesCurrentCredentialUnchanged(t *testing.T) {
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	now := time.Now()
	if err := credentials.Put("claude", auth.Credential{Type: "oauth", Access: "access", Refresh: "refresh", Expires: now.Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("store credential: %v", err)
	}
	called := false
	flowFor := func(model.Provider) (auth.BrowserFlow, error) {
		called = true
		return auth.BrowserFlow{}, nil
	}
	if err := refreshProviderCredential(context.Background(), model.Claude, credentials, flowFor, now); err != nil {
		t.Fatalf("refresh current credential: %v", err)
	}
	if called {
		t.Fatal("refresh flow was invoked for a current credential")
	}
}
