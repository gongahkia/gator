package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/state"
)

func nativeConnectionFixture(t *testing.T, serverURL string) (string, connector.Descriptor, auth.Store) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	d := connector.Descriptor{Version: 1, ID: "google", Name: "Personal Google", Kind: connector.KindGoogle, Resource: serverURL, Authentication: connector.AuthOAuth, OAuthClientID: "desktop-client", OAuthAuthorizeURL: serverURL + "/authorize", OAuthTokenURL: serverURL + "/token", OAuthRedirectURL: connector.GoogleOAuthRedirectURL, OAuthScopes: "openid https://www.googleapis.com/auth/userinfo.email " + connector.GoogleDailyOAuthScopes}
	if err := addNativeConnection(d); err != nil {
		t.Fatal(err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	return root, d, credentials
}
func finishNativeSignIn(t *testing.T, s *connectionSignIn) connectionSignInStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := s.Status()
		if status.Done {
			return status
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("sign-in did not settle")
	return connectionSignInStatus{}
}
func authorizeNativeSignIn(t *testing.T, status connectionSignInStatus) {
	t.Helper()
	u, err := url.Parse(status.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" || u.Query().Get("access_type") != "offline" {
		t.Fatal("missing PKCE or offline authorization")
	}
	callback, err := url.Parse(u.Query().Get("redirect_uri"))
	if err != nil {
		t.Fatal(err)
	}
	q := callback.Query()
	q.Set("state", u.Query().Get("state"))
	q.Set("code", "callback-code")
	callback.RawQuery = q.Encode()
	response, err := http.Get(callback.String())
	if err == nil {
		response.Body.Close()
	}
}
func TestNativeConnectionSignInCallbackAndLifecycle(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = r.ParseForm()
		if r.Form.Get("code_verifier") == "" || r.Form.Get("code") != "callback-code" || r.Form.Get("client_secret") != "private-secret" {
			t.Error("incorrect token exchange")
		}
		_, _ = w.Write([]byte(`{"access_token":"private-access","refresh_token":"private-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()
	root, d, credentials := nativeConnectionFixture(t, server.URL)
	s := &connectionSignIn{}
	t.Cleanup(func() { s.Cancel(d.ID) })
	status, err := s.Begin(root, d.ID, connectorLoginReview(d), "private-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := credentials.Read(d.CredentialRef()); found {
		t.Fatal("credentials saved before browser authorization")
	}
	if release, err := state.Guard(root, true); !errors.Is(err, state.ErrBusy) {
		if release != nil {
			release()
		}
		t.Fatal("maintenance raced sign-in", err)
	}
	if _, err := s.Begin(root, d.ID, connectorLoginReview(d), ""); err == nil {
		t.Fatal("overlapping sign-in accepted")
	}
	authorizeNativeSignIn(t, status)
	done := finishNativeSignIn(t, s)
	if done.Issue != "" || done.URL != "" || requests != 1 {
		t.Fatal("sign-in failed", done, requests)
	}
	credential, found, err := credentials.Read(d.CredentialRef())
	if err != nil || !found || credential.Access != "private-access" || credential.OAuthClientSecret != "private-secret" {
		t.Fatal("credential missing", err)
	}
}
func TestNativeConnectionSignInRejectsLateAccountReplacementAndConfigChange(t *testing.T) {
	for _, change := range []string{"credential", "config", "cancel"} {
		t.Run(change, func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-finish
				_, _ = w.Write([]byte(`{"access_token":"late-token","token_type":"Bearer","expires_in":3600}`))
			}))
			defer server.Close()
			root, d, credentials := nativeConnectionFixture(t, server.URL)
			s := &connectionSignIn{}
			t.Cleanup(func() { s.Cancel(d.ID) })
			status, err := s.Begin(root, d.ID, connectorLoginReview(d), "")
			if err != nil {
				t.Fatal(err)
			}
			authorizeNativeSignIn(t, status)
			<-started
			switch change {
			case "credential":
				if err := credentials.Put(d.CredentialRef(), auth.Credential{Type: "oauth", Access: "new-account"}); err != nil {
					t.Error(err)
				}
			case "config":
				store, _ := config.DefaultStore()
				if err := store.Update(func(settings *config.Settings) error { settings.Connectors[0].Name = "Changed"; return nil }); err != nil {
					t.Error(err)
				}
			case "cancel":
				s.Cancel(d.ID)
			}
			close(finish)
			if change != "cancel" {
				done := finishNativeSignIn(t, s)
				if done.Issue == "" {
					t.Fatal("stale sign-in accepted")
				}
			}
			// The callback lease is released only after cancellation/commit has settled.
			deadline := time.Now().Add(time.Second * 3)
			for {
				release, err := state.Guard(root, true)
				if err == nil {
					release()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal(err)
				}
				time.Sleep(time.Millisecond * 5)
			}
			current, found, _ := credentials.Read(d.CredentialRef())
			if change == "credential" {
				if current.Access != "new-account" {
					t.Fatal("new account overwritten")
				}
			} else if found {
				t.Fatal("cancelled/stale sign-in saved credentials")
			}
		})
	}
}
func TestNativeConnectionReviewRejectsStaleDisconnectAndProtectsSecrets(t *testing.T) {
	root, d, credentials := nativeConnectionFixture(t, "https://example.com")
	controller := &connectionControls{stateDir: root}
	old := auth.Credential{Type: "oauth", Access: "old-secret"}
	if err := credentials.Put(d.CredentialRef(), old); err != nil {
		t.Fatal(err)
	}
	screen, err := controller.call(context.Background(), "status", []string{d.ID})
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	for _, a := range screen.Actions {
		if a.Action == "disconnect" {
			args = a.Args
		}
	}
	if len(args) != 3 {
		t.Fatal("disconnect not available")
	}
	if strings.Contains(screen.Panel.Subtitle, "authenticated") {
		t.Fatal("local credentials claimed verified access")
	}
	next := auth.Credential{Type: "oauth", Access: "new-secret"}
	_ = credentials.Put(d.CredentialRef(), next)
	if _, err = controller.call(context.Background(), "disconnect", args); !errors.Is(err, auth.ErrCredentialChanged) {
		t.Fatal("stale disconnect accepted", err)
	}
	current, _, _ := credentials.Read(d.CredentialRef())
	if current.Access != next.Access {
		t.Fatal("new credentials removed")
	}
}

func TestNativeConnectionChecksEachGrantedReadAndReportsPartialFailure(t *testing.T) {
	calendarFails := true
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		switch r.URL.Path {
		case "/oauth2/v3/userinfo":
			_, _ = w.Write([]byte(`{"sub":"stable-account","email":"me@example.com","email_verified":true}`))
		case "/gmail/v1/users/me/messages":
			_, _ = w.Write([]byte(`{"messages":[]}`))
		case "/calendar/v3/calendars/primary/events":
			if calendarFails {
				w.WriteHeader(http.StatusForbidden)
			} else {
				_, _ = w.Write([]byte(`{"items":[]}`))
			}
		default:
			t.Error("unexpected read", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	root, d, credentials := nativeConnectionFixture(t, server.URL)
	_ = credentials.Put(d.CredentialRef(), auth.Credential{Type: "oauth", Access: "fixture-token"})
	controls := &connectionControls{stateDir: root}
	_, err := controls.call(context.Background(), "verify", []string{d.ID, connectorLoginReview(d)})
	if err == nil || !strings.Contains(err.Error(), "calendar read check failed") {
		t.Fatal("partial read presented as fully healthy", err)
	}
	calendarFails = false
	screen, err := controls.call(context.Background(), "verify", []string{d.ID, connectorLoginReview(d)})
	if err != nil || !strings.Contains(screen.Panel.Notice, "me@example.com") || !strings.Contains(screen.Panel.Notice, "email, calendar") {
		t.Fatal("verification missing", err, screen.Panel.Notice)
	}
	if seen["/gmail/v1/users/me/messages"] != 2 || seen["/calendar/v3/calendars/primary/events"] != 2 {
		t.Fatal("granted surfaces not checked", seen)
	}
}
