package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
)

func TestOAuthLoginDiscoversRegistersAndBindsCredentialToResource(t *testing.T) {
	var serverURL string
	var registration struct {
		RedirectURI string
		ClientID    string
	}
	var tokenRequest url.Values
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/mcp":
			writer.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+serverURL+`/.well-known/oauth-protected-resource/mcp", scope="issues:read"`)
			writer.WriteHeader(http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(writer).Encode(map[string]any{"resource": serverURL + "/mcp", "authorization_servers": []string{serverURL + "/issuer"}, "scopes_supported": []string{"profile"}})
		case "/.well-known/oauth-authorization-server/issuer":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"authorization_endpoint":           serverURL + "/authorize",
				"token_endpoint":                   serverURL + "/token",
				"registration_endpoint":            serverURL + "/register",
				"code_challenge_methods_supported": []string{"S256"},
			})
		case "/register":
			var payload struct {
				RedirectURIs            []string `json:"redirect_uris"`
				TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.RedirectURIs) != 1 || payload.TokenEndpointAuthMethod != "none" {
				t.Fatalf("dynamic registration = %#v", payload)
			}
			mutex.Lock()
			registration.RedirectURI = payload.RedirectURIs[0]
			registration.ClientID = "registered-gator"
			mutex.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]string{"client_id": "registered-gator"})
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Fatal(err)
			}
			mutex.Lock()
			tokenRequest = request.Form
			mutex.Unlock()
			if request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("client_id") != "registered-gator" || request.Form.Get("resource") != serverURL+"/mcp" || request.Form.Get("code_verifier") == "" {
				t.Fatalf("token form = %#v", request.Form)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "mcp-access", "refresh_token": "mcp-refresh", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	repository := oauthRepository(t, server.URL+"/mcp")
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatalf("hash trusted MCP bundle: %v", err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	login, err := BeginOAuthLogin(context.Background(), repository, "remote", digest, credentials, OAuthLoginOptions{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("begin MCP OAuth login: %v", err)
	}
	defer login.Cancel()
	authorizationURL, err := url.Parse(login.URL())
	if err != nil {
		t.Fatal(err)
	}
	if authorizationURL.Query().Get("client_id") != "registered-gator" || authorizationURL.Query().Get("resource") != server.URL+"/mcp" || authorizationURL.Query().Get("scope") != "issues:read" || authorizationURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL = %s", authorizationURL)
	}
	mutex.Lock()
	registeredRedirect := registration.RedirectURI
	mutex.Unlock()
	if registeredRedirect != login.RedirectURL() || !strings.HasPrefix(registeredRedirect, "http://127.0.0.1:") {
		t.Fatalf("registered redirect = %q, login redirect = %q", registeredRedirect, login.RedirectURL())
	}
	completed := make(chan error, 1)
	go func() { completed <- login.Complete(context.Background()) }()
	callbackURL, err := url.Parse(login.RedirectURL())
	if err != nil {
		t.Fatal(err)
	}
	query := callbackURL.Query()
	query.Set("state", authorizationURL.Query().Get("state"))
	query.Set("code", "approved-code")
	callbackURL.RawQuery = query.Encode()
	response, err := server.Client().Get(callbackURL.String())
	if err != nil {
		t.Fatalf("deliver OAuth callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("callback status = %s", response.Status)
	}
	if err := <-completed; err != nil {
		t.Fatalf("complete MCP OAuth login: %v", err)
	}
	key, err := OAuthCredentialKey(server.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read(key)
	if err != nil || !found || credential.Access != "mcp-access" || credential.Extra[extraResource] != server.URL+"/mcp" || credential.Extra[extraClientID] != "registered-gator" {
		t.Fatalf("stored MCP credential = %#v, found=%t err=%v", credential, found, err)
	}
	mutex.Lock()
	if tokenRequest.Get("redirect_uri") != login.RedirectURL() {
		t.Fatalf("token request = %#v", tokenRequest)
	}
	mutex.Unlock()
}

func TestAccessTokenRefreshesOnlyMatchingMCPResource(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/token" {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("resource") != serverURL+"/mcp" || request.Form.Get("client_id") != "registered" {
			t.Fatalf("refresh form = %#v", request.Form)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "fresh", "token_type": "bearer", "expires_in": 1800})
	}))
	defer server.Close()
	serverURL = server.URL
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := OAuthCredentialKey(server.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(key, auth.Credential{Type: "oauth", Access: "old", Refresh: "refresh", Expires: time.Now().Add(time.Minute).UnixMilli(), Extra: map[string]string{extraResource: server.URL + "/mcp", extraTokenURL: server.URL + "/token", extraClientID: "registered"}}); err != nil {
		t.Fatal(err)
	}
	token, configured, err := accessToken(context.Background(), credentials, server.URL+"/mcp", time.Now())
	if err != nil || !configured || token != "fresh" {
		t.Fatalf("refreshed access token = %q configured=%t err=%v", token, configured, err)
	}
	if _, configured, err := accessToken(context.Background(), credentials, server.URL+"/other", time.Now()); err != nil || configured {
		t.Fatalf("foreign resource credential reused: configured=%t err=%v", configured, err)
	}
}

func TestOAuthLoginRequiresTrustedProjectBundle(t *testing.T) {
	repository := oauthRepository(t, "http://127.0.0.1:1/mcp")
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = BeginOAuthLogin(context.Background(), repository, "remote", "not-the-bundle-hash", credentials, OAuthLoginOptions{})
	if err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("untrusted OAuth login error = %v", err)
	}
}

func TestLoadWithCredentialsSendsOAuthTokenOnlyToMatchingServer(t *testing.T) {
	var authorization []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = append(authorization, request.Header.Get("Authorization"))
		var call struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			t.Fatal(err)
		}
		result := any(map[string]any{})
		if call.Method == "tools/list" {
			result = map[string]any{"tools": []map[string]any{{"name": "lookup", "inputSchema": map[string]any{"type": "object"}}}}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": result})
	}))
	defer server.Close()
	repository := oauthRepository(t, server.URL)
	digest, err := BundleHash(repository)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, err := OAuthCredentialKey(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(key, auth.Credential{Type: "oauth", Access: "bound-token", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: map[string]string{extraResource: server.URL, extraTokenURL: server.URL + "/token", extraClientID: "registered"}}); err != nil {
		t.Fatal(err)
	}
	set, err := LoadWithCredentials(context.Background(), repository, digest, credentials)
	if err != nil {
		t.Fatalf("load authenticated MCP server: %v", err)
	}
	defer set.Close()
	tool := set.Tools(func(context.Context, string, string) error { return nil })[0]
	if _, err := tool.Execute(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("execute authenticated MCP tool: %v", err)
	}
	if len(authorization) < 3 {
		t.Fatalf("MCP calls = %d, want initialize/list/call", len(authorization))
	}
	for _, value := range authorization {
		if value != "Bearer bound-token" {
			t.Fatalf("OAuth authorization header = %q", value)
		}
	}
}

func oauthRepository(t *testing.T, endpoint string) string {
	t.Helper()
	repository := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"servers":[{"name":"remote","transport":"streamable_http","url":` + quote(endpoint) + `}]}`
	if err := os.WriteFile(filepath.Join(repository, ".gator", "mcp.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return repository
}

func quote(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
