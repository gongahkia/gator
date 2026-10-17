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

func TestPinnedGoogleReadRejectsReplacedGrantAtRequestBoundary(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/gmail/v1/users/me/messages" {
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"messages":[]}`))
	}))
	defer server.Close()
	descriptor := Descriptor{
		Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle,
		Resource: server.URL, Authentication: AuthOAuth,
		OAuthClientID: "client", OAuthAuthorizeURL: server.URL + "/authorize", OAuthTokenURL: server.URL + "/token",
		OAuthRedirectURL: "http://127.0.0.1:1457/oauth/callback", OAuthScopes: GoogleOAuthScopes + " " + GoogleDailyOAuthScopes,
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "oauth", Access: "access-A", Refresh: "refresh-A", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client(), OAuthRefreshPins: map[string]string{"google": OAuthGrantPin(descriptor, credential.Refresh)}}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err != nil || requests != 1 {
		t.Fatalf("original grant read = %v, requests=%d", err, requests)
	}
	credential.Access, credential.Refresh = "access-B", "refresh-B"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err == nil || !strings.Contains(err.Error(), "authorization changed") || requests != 1 {
		t.Fatalf("replaced grant was used: %v, requests=%d", err, requests)
	}
	changed := descriptor
	changed.Authentication = AuthNone
	changed.OAuthClientID, changed.OAuthAuthorizeURL, changed.OAuthTokenURL, changed.OAuthRedirectURL, changed.OAuthScopes = "", "", "", "", ""
	runtime.Registry, err = NewRegistry([]Descriptor{changed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err == nil || !strings.Contains(err.Error(), "authorization mode changed") || requests != 1 {
		t.Fatalf("changed authorization mode bypassed pin: %v, requests=%d", err, requests)
	}
}

func TestGoogleAccountPinAllowsSameAccountReauthorizationButRejectsSwitchAtRequest(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/oauth2/v3/userinfo":
			subject := "account-A"
			if request.Header.Get("Authorization") == "Bearer access-C" {
				subject = "account-B"
			}
			_, _ = writer.Write([]byte(`{"sub":"` + subject + `","email":"untrusted@example.com"}`))
		case "/gmail/v1/users/me/messages":
			reads++
			_, _ = writer.Write([]byte(`{"messages":[]}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	descriptor := Descriptor{
		Version: DescriptorVersion, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL,
		Authentication: AuthOAuth, OAuthClientID: "client", OAuthAuthorizeURL: server.URL + "/authorize",
		OAuthTokenURL: server.URL + "/token", OAuthRedirectURL: "http://127.0.0.1:1457/oauth/callback",
		OAuthScopes: GoogleOAuthScopes + " " + GoogleDailyOAuthScopes,
	}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "oauth", Access: "access-A", Refresh: "refresh-A", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client()}
	pin, err := runtime.GoogleAccountPin(context.Background(), "google")
	if err != nil || pin != GoogleAccountPin(descriptor, "account-A") {
		t.Fatalf("initial account pin = %q, %v", pin, err)
	}
	runtime.OAuthAccountPins = map[string]string{"google": pin}
	for _, token := range []string{"access-A", "access-B"} {
		credential.Access, credential.Refresh = token, "refresh-"+token
		if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err != nil {
			t.Fatalf("same account read with %s: %v", token, err)
		}
	}
	credential.Access, credential.Refresh = "access-C", "refresh-C"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err == nil || !strings.Contains(err.Error(), "account changed") || reads != 2 {
		t.Fatalf("different account crossed read boundary: %v, reads=%d", err, reads)
	}
	changed := descriptor
	changed.OAuthClientID = "different-client"
	runtime.Registry, err = NewRegistry([]Descriptor{changed})
	if err != nil {
		t.Fatal(err)
	}
	credential.Access = "access-B"
	if err := credentials.Put(changed.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "google", "gmail_list", json.RawMessage(`{"query":"label:offsite"}`)); err == nil || !strings.Contains(err.Error(), "account changed") || reads != 2 {
		t.Fatalf("changed connector identity crossed read boundary: %v, reads=%d", err, reads)
	}
}
