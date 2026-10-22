package connector

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
)

func TestLateRefreshCannotUndoDisconnect(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-finish
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"refreshed","refresh_token":"replacement","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()
	d := Descriptor{Version: 1, ID: "google", Name: "Google", Kind: KindGoogle, Resource: server.URL, Authentication: AuthOAuth, OAuthClientID: "client", OAuthAuthorizeURL: server.URL + "/authorize", OAuthTokenURL: server.URL + "/token", OAuthRedirectURL: "http://127.0.0.1:12345/oauth/callback", OAuthScopes: GoogleOAuthScopes}
	s, _ := auth.New(t.TempDir())
	old := auth.Credential{Type: "oauth", Access: "old", Refresh: "refresh", Expires: time.Now().Add(-time.Hour).UnixMilli()}
	if err := s.Put(d.CredentialRef(), old); err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Credentials: s, HTTPClient: server.Client()}
	done := make(chan error, 1)
	go func() { _, err := runtime.unverifiedBearerToken(context.Background(), d); done <- err }()
	<-started
	if err := s.Delete(d.CredentialRef()); err != nil {
		close(finish)
		t.Fatal(err)
	}
	close(finish)
	if err := <-done; !errors.Is(err, auth.ErrCredentialChanged) {
		t.Fatal("late refresh accepted", err)
	}
	if _, found, err := s.Read(d.CredentialRef()); err != nil || found {
		t.Fatal("refresh restored disconnected account", err)
	}
}
