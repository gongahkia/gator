package connector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/auth"
)

func TestReviewedBearerIdentityIsCheckedBeforeEveryRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	descriptor := Descriptor{Version: 1, ID: "reports", Name: "Reports", Kind: KindHTTPJSON, Resource: server.URL, Authentication: AuthBearer}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "bearer_token", Access: "controlled-original"}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	pin := CredentialPin(descriptor, credential)
	runtime := Runtime{Registry: registry, Credentials: credentials, HTTPClient: server.Client(), CredentialPins: map[string]string{"reports": pin}}
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`)); err != nil || calls != 1 {
		t.Fatal("reviewed identity was not used", err, calls)
	}
	credential.Access = "controlled-replacement"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`))
	var access AccessError
	if !errors.As(err, &access) || access.Kind != "changed_account" || calls != 1 {
		t.Fatal("changed account reached the source", err, calls)
	}
	// The same token under a replacement endpoint is a different reviewed
	// account boundary, even if adapter construction races a settings edit.
	credential.Access = "controlled-original"
	descriptor.Resource = server.URL + "/replacement"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	registry, err = NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Registry = registry
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`)); err == nil || calls != 1 {
		t.Fatal("replacement destination reused the reviewed token", err, calls)
	}
	descriptor.Authentication = AuthNone
	registry, err = NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Registry = registry
	if _, err := runtime.Invoke(context.Background(), action.Inspect, "reports", "fetch", json.RawMessage(`{}`)); err == nil || calls != 1 {
		t.Fatal("authentication downgrade bypassed reviewed identity", err, calls)
	}
}

func TestReviewedOAuthBindingSurvivesRefreshButNotGrantOrDescriptorReplacement(t *testing.T) {
	descriptor := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: "https://www.googleapis.com", Authentication: AuthOAuth, OAuthClientID: "client", OAuthTokenURL: GoogleOAuthTokenURL, OAuthAuthorizeURL: GoogleOAuthAuthorizeURL, OAuthRedirectURL: GoogleOAuthRedirectURL, OAuthScopes: GoogleDailyOAuthScopes}
	original := auth.Credential{Type: "oauth", Access: "controlled-access-one", Refresh: "controlled-grant-one", Expires: 100}
	pin := CredentialPin(descriptor, original)
	refreshed := original
	refreshed.Access = "controlled-access-two"
	refreshed.Expires = 200
	if CredentialPin(descriptor, refreshed) != pin {
		t.Fatal("ordinary token refresh changed the reviewed account")
	}
	refreshed.Refresh = "controlled-grant-two"
	if CredentialPin(descriptor, refreshed) == pin {
		t.Fatal("replaced OAuth grant reused a review")
	}
	changed := descriptor
	changed.OAuthScopes += " " + GoogleSendOAuthScope
	if CredentialPin(changed, original) == pin {
		t.Fatal("changed operation scope reused a review")
	}
	original.Refresh = ""
	noRefresh := CredentialPin(descriptor, original)
	original.Access = "controlled-other-account"
	if CredentialPin(descriptor, original) == noRefresh {
		t.Fatal("an OAuth token without a refresh grant could change accounts")
	}
}
