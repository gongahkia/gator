package auth

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBrowserFlowCallbackAndExchangeUsesPKCE(t *testing.T) {
	var received url.Values
	tokens := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s", request.Method)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		received = request.PostForm
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "expires_in": 3600})
	}))
	defer tokens.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve callback port: %v", err)
	}
	redirect := "http://" + listener.Addr().String() + "/callback"
	if err := listener.Close(); err != nil {
		t.Fatalf("release callback port: %v", err)
	}
	attempt, err := BeginBrowserFlow(BrowserFlow{ClientID: "gator-client", AuthorizationURL: "https://auth.example.test/authorize", TokenURL: tokens.URL, RedirectURL: redirect, Scopes: []string{"openid", "offline_access"}, AuthorizeParams: map[string]string{"audience": "gator"}})
	if err != nil {
		t.Fatalf("begin flow: %v", err)
	}
	authorize, err := url.Parse(attempt.AuthorizationURL())
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := authorize.Query()
	if query.Get("client_id") != "gator-client" || query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" || query.Get("audience") != "gator" {
		t.Fatalf("authorization query = %#v", query)
	}
	callback, err := attempt.StartCallback()
	if err != nil {
		t.Fatalf("start callback: %v", err)
	}
	go func() {
		_, _ = http.Get(redirect + "?code=one-time-code&state=" + url.QueryEscape(query.Get("state")))
	}()
	context, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	code, err := callback.Wait(context)
	if err != nil {
		t.Fatalf("wait for callback: %v", err)
	}
	credential, err := attempt.Exchange(context, code)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if !credential.IsOAuth() || credential.Access != "access" || credential.Refresh != "refresh" || credential.Expires <= time.Now().UnixMilli() {
		t.Fatalf("credential = %#v", credential)
	}
	if received.Get("grant_type") != "authorization_code" || received.Get("code") != "one-time-code" || received.Get("code_verifier") == "" || received.Get("redirect_uri") != redirect || strings.Contains(attempt.AuthorizationURL(), received.Get("code_verifier")) {
		t.Fatalf("token form = %#v", received)
	}
}

func TestBrowserFlowRefreshRetainsUnrotatedRefreshToken(t *testing.T) {
	tokens := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse refresh form: %v", err)
		}
		if request.PostForm.Get("grant_type") != "refresh_token" || request.PostForm.Get("refresh_token") != "original-refresh" {
			t.Fatalf("refresh form = %#v", request.PostForm)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "new-access", "expires_in": 7200})
	}))
	defer tokens.Close()
	flow := BrowserFlow{ClientID: "client", TokenURL: tokens.URL}
	credential, err := flow.Refresh(context.Background(), Credential{Type: oauthType, Access: "old-access", Refresh: "original-refresh", Expires: time.Now().Add(-time.Minute).UnixMilli(), Extra: map[string]string{"account": "one"}})
	if err != nil {
		t.Fatalf("refresh credential: %v", err)
	}
	if credential.Access != "new-access" || credential.Refresh != "original-refresh" || credential.Extra["account"] != "one" || credential.Expired(time.Now()) {
		t.Fatalf("refreshed credential = %#v", credential)
	}
}

func TestBrowserFlowJSONExchangeIncludesState(t *testing.T) {
	var received map[string]string
	tokens := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatalf("decode JSON token request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "expires_in": 3600})
	}))
	defer tokens.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve callback port: %v", err)
	}
	redirect := "http://" + listener.Addr().String() + "/callback"
	_ = listener.Close()
	attempt, err := BeginBrowserFlow(BrowserFlow{ClientID: "gator-client", AuthorizationURL: "https://auth.example.test/authorize", TokenURL: tokens.URL, RedirectURL: redirect, TokenRequestJSON: true, TokenIncludesState: true})
	if err != nil {
		t.Fatalf("begin flow: %v", err)
	}
	credential, err := attempt.Exchange(context.Background(), "one-time-code")
	if err != nil {
		t.Fatalf("exchange JSON credential: %v", err)
	}
	if credential.Access != "access" || received["code"] != "one-time-code" || received["state"] != attempt.State() || received["redirect_uri"] != redirect {
		t.Fatalf("credential = %#v, request = %#v", credential, received)
	}
}

func TestBrowserFlowRejectsUnsafeCallback(t *testing.T) {
	if _, err := BeginBrowserFlow(BrowserFlow{ClientID: "client", AuthorizationURL: "https://example.test/authorize", TokenURL: "https://example.test/token", RedirectURL: "https://example.test/callback"}); err == nil {
		t.Fatal("non-loopback callback was accepted")
	}
}
