package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workapp"
)

func sendingControl(t *testing.T, screen workapp.ControlScreen, operation string) workapp.ControlAction {
	t.Helper()
	for _, a := range screen.Actions {
		if a.Action == operation {
			if !a.Review {
				t.Fatal("permission change skipped review")
			}
			return a
		}
	}
	t.Fatal("missing permission control", operation)
	return workapp.ControlAction{}
}

func TestNativeEmailSendingPermissionReviewConsentAndLocalDisable(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/token" {
			t.Error("unexpected account mutation", r.URL.Path)
		}
		_ = r.ParseForm()
		if r.Form.Get("client_secret") != "stored-private-secret" || r.Form.Get("code_verifier") == "" {
			t.Error("private exchange changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "refresh_token": "new-refresh", "token_type": "Bearer", "expires_in": 3600, "scope": "openid " + connector.GoogleDailyOAuthScopes + " " + connector.GoogleSendOAuthScope})
	}))
	defer server.Close()
	root, d, credentials := nativeConnectionFixture(t, server.URL)
	old := auth.Credential{Type: "oauth", Access: "old-private-access", Refresh: "old-private-refresh", OAuthClientSecret: "stored-private-secret"}
	if err := credentials.Put(d.CredentialRef(), old); err != nil {
		t.Fatal(err)
	}
	c := &connectionControls{stateDir: root}
	t.Cleanup(func() { c.signIn.Cancel(d.ID) })
	screen, err := c.screen(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	enable := sendingControl(t, screen, "enable-email-sending")
	if len(enable.Args) != 3 || len(enable.Fields) != 1 || !enable.Fields[0].Secret || !strings.Contains(enable.Description, connector.GoogleSendOAuthScope) {
		t.Fatal("permission review incomplete")
	}
	data, _ := json.Marshal(screen)
	for _, secret := range []string{old.Access, old.Refresh, old.OAuthClientSecret} {
		if strings.Contains(string(data), secret) {
			t.Fatal("private credential in review")
		}
	}
	if _, err := c.call(context.Background(), enable.Action, append(enable.Args, "")); err != nil {
		t.Fatal(err)
	}
	status := c.signIn.Status()
	u, _ := url.Parse(status.URL)
	scopes := strings.Fields(u.Query().Get("scope"))
	if !slices.Contains(scopes, connector.GoogleSendOAuthScope) || !slices.Contains(scopes, connector.GoogleDailyOAuthScopes) || u.Query().Get("prompt") != "consent" || u.Query().Get("include_granted_scopes") != "" {
		t.Fatal("wrong installed-app consent scopes")
	}
	before, _, _ := configuredConnector(d.ID)
	credential, _, _ := credentials.Read(d.CredentialRef())
	if connectorLoginReview(before) != connectorLoginReview(d) || !reflect.DeepEqual(credential, old) || requests != 0 {
		t.Fatal("permission published before consent")
	}
	authorizeNativeSignIn(t, status)
	done := finishNativeSignIn(t, &c.signIn)
	if done.Issue != "" || !strings.Contains(done.Notice, "exact approval") {
		t.Fatal("upgrade failed", done.Issue)
	}
	after, _, _ := configuredConnector(d.ID)
	credential, _, _ = credentials.Read(d.CredentialRef())
	if !slices.Contains(strings.Fields(after.OAuthScopes), connector.GoogleSendOAuthScope) || credential.Access != "new-access" || credential.Refresh != "new-refresh" || credential.OAuthClientSecret != old.OAuthClientSecret || requests != 1 {
		t.Fatal("upgrade was not committed correctly")
	}
	found := false
	for _, op := range after.Operations() {
		found = found || op.ID == "gmail_send"
	}
	if !found {
		t.Fatal("completed upgrade has no sending operation")
	}
	screen, err = c.screen(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	disable := sendingControl(t, screen, "disable-email-sending")
	screen, err = c.call(context.Background(), disable.Action, disable.Args)
	if err != nil {
		t.Fatal(err)
	}
	afterDisable, _, _ := configuredConnector(d.ID)
	if !strings.Contains(screen.Panel.Notice, "Google account settings") || connectorLoginReview(afterDisable) != connectorLoginReview(d) || requests != 1 {
		t.Fatal("local disable changed other permissions or contacted provider")
	}
	current, _, _ := credentials.Read(d.CredentialRef())
	if !reflect.DeepEqual(current, credential) {
		t.Fatal("local disable replaced sign-in")
	}
	for _, op := range afterDisable.Operations() {
		if op.ID == "gmail_send" {
			t.Fatal("send still exposed")
		}
	}
}

func TestNativeEmailSendingRejectsPartialOrMissingGrants(t *testing.T) {
	for _, c := range []struct{ name, scope, refresh string }{
		{"missing send", "openid " + connector.GoogleDailyOAuthScopes, "new-refresh"},
		{"missing old read", "openid " + connector.GoogleSendOAuthScope, "new-refresh"},
		{"missing identity", connector.GoogleDailyOAuthScopes + " " + connector.GoogleSendOAuthScope, "new-refresh"},
		{"omitted scope", "", "new-refresh"},
		{"missing offline grant", "openid " + connector.GoogleDailyOAuthScopes + " " + connector.GoogleSendOAuthScope, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "unusable-new-token", "refresh_token": c.refresh, "token_type": "Bearer", "expires_in": 3600, "scope": c.scope})
			}))
			defer server.Close()
			root, d, credentials := nativeConnectionFixture(t, server.URL)
			old := auth.Credential{Type: "oauth", Access: "old-access", Refresh: "old-refresh"}
			if err := credentials.Put(d.CredentialRef(), old); err != nil {
				t.Fatal(err)
			}
			s := &connectionSignIn{}
			t.Cleanup(func() { s.Cancel(d.ID) })
			status, err := s.EnableGmailSending(root, d.ID, connectorLoginReview(d), credentialReview(&old), "")
			if err != nil {
				t.Fatal(err)
			}
			authorizeNativeSignIn(t, status)
			if finishNativeSignIn(t, s).Issue == "" {
				t.Fatal("unconfirmed permission upgrade accepted")
			}
			current, _, _ := configuredConnector(d.ID)
			credential, found, _ := credentials.Read(d.CredentialRef())
			if connectorLoginReview(current) != connectorLoginReview(d) || !found || !reflect.DeepEqual(credential, old) {
				t.Fatal("failed upgrade altered existing connection")
			}
		})
	}
}

func TestNativeEmailSendingRejectsChangedOrCancelledConsent(t *testing.T) {
	for _, change := range []string{"config", "credential", "remove", "cancel", "denied"} {
		t.Run(change, func(t *testing.T) {
			started, finish := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-finish
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "late-token", "refresh_token": "late-refresh", "token_type": "Bearer", "expires_in": 3600, "scope": "openid " + connector.GoogleDailyOAuthScopes + " " + connector.GoogleSendOAuthScope})
			}))
			defer server.Close()
			root, d, credentials := nativeConnectionFixture(t, server.URL)
			old := auth.Credential{Type: "oauth", Access: "old-access", Refresh: "old-refresh"}
			if err := credentials.Put(d.CredentialRef(), old); err != nil {
				t.Fatal(err)
			}
			s := &connectionSignIn{}
			t.Cleanup(func() { s.Cancel(d.ID) })
			status, err := s.EnableGmailSending(root, d.ID, connectorLoginReview(d), credentialReview(&old), "")
			if err != nil {
				t.Fatal(err)
			}
			if change == "denied" {
				u, _ := url.Parse(status.URL)
				callback, _ := url.Parse(u.Query().Get("redirect_uri"))
				q := callback.Query()
				q.Set("state", u.Query().Get("state"))
				q.Set("error", "access_denied")
				callback.RawQuery = q.Encode()
				response, err := http.Get(callback.String())
				if err != nil {
					t.Fatal(err)
				}
				page, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				if readErr != nil || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(page), "Return to Gator.") {
					t.Fatal("browser denial page was truncated", readErr)
				}
			} else {
				authorizeNativeSignIn(t, status)
				<-started
			}
			store, _ := config.DefaultStore()
			switch change {
			case "config":
				err = store.Update(func(s *config.Settings) error { s.Connectors[0].Name = "Edited during consent"; return nil })
			case "remove":
				err = store.Update(func(s *config.Settings) error { s.Connectors = nil; return nil })
			case "credential":
				err = credentials.Put(d.CredentialRef(), auth.Credential{Type: "oauth", Access: "newer-account"})
			case "cancel":
				s.Cancel(d.ID)
			}
			close(finish)
			if err != nil {
				t.Fatal(err)
			}
			if change != "cancel" && finishNativeSignIn(t, s).Issue == "" {
				t.Fatal("stale or denied upgrade accepted")
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				release, err := state.Guard(root, true)
				if err == nil {
					release()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("callback did not settle", err)
				}
				time.Sleep(5 * time.Millisecond)
			}
			current, found, _ := credentials.Read(d.CredentialRef())
			if !found || change == "credential" && current.Access != "newer-account" || change != "credential" && !reflect.DeepEqual(current, old) {
				t.Fatal("late grant replaced saved account")
			}
			settings, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			for _, current := range settings.Connectors {
				if slices.Contains(strings.Fields(current.OAuthScopes), connector.GoogleSendOAuthScope) {
					t.Fatal("stale permission published")
				}
			}
		})
	}
}

func TestNativeEmailPermissionReviewRejectsNewerAccountAndInvalidService(t *testing.T) {
	root, d, credentials := nativeConnectionFixture(t, "https://example.com")
	old := auth.Credential{Type: "oauth", Access: "old-access"}
	if err := credentials.Put(d.CredentialRef(), old); err != nil {
		t.Fatal(err)
	}
	c := &connectionControls{stateDir: root}
	screen, err := c.screen(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := sendingControl(t, screen, "enable-email-sending")
	if err := credentials.Put(d.CredentialRef(), auth.Credential{Type: "oauth", Access: "new-access"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.call(context.Background(), a.Action, append(a.Args, "")); !errors.Is(err, auth.ErrCredentialChanged) {
		t.Fatal("stale account review accepted", err)
	}
	for _, mutate := range []func(*connector.Descriptor){func(d *connector.Descriptor) { d.Kind = connector.KindSlack }, func(d *connector.Descriptor) { d.Authentication = connector.AuthBearer }, func(d *connector.Descriptor) { d.OAuthScopes += " " + connector.GoogleSendOAuthScope }} {
		changed := d
		mutate(&changed)
		if _, err := gmailSendingDescriptor(changed, true); err == nil {
			t.Fatal("invalid permission upgrade accepted")
		}
	}
}

func TestNativeEmailPermissionPublicationRestoresPriorGrantOnStaleConfig(t *testing.T) {
	_, d, credentials := nativeConnectionFixture(t, "https://example.com")
	old := auth.Credential{Type: "oauth", Access: "old-access", Refresh: "old-refresh"}
	if err := credentials.Put(d.CredentialRef(), old); err != nil {
		t.Fatal(err)
	}
	store, _ := config.DefaultStore()
	if err := store.Update(func(s *config.Settings) error { s.Connectors[0].Name = "Concurrent edit"; return nil }); err != nil {
		t.Fatal(err)
	}
	next, err := gmailSendingDescriptor(d, true)
	if err != nil {
		t.Fatal(err)
	}
	err = publishGmailSendingPermission(d, next, credentials, &old, auth.Credential{Type: "oauth", Access: "new-access", Refresh: "new-refresh"})
	if err == nil {
		t.Fatal("stale publication accepted")
	}
	current, found, _ := credentials.Read(d.CredentialRef())
	if !found || !reflect.DeepEqual(current, old) {
		t.Fatal("failed settings publication left replacement grant")
	}
	configured, _, _ := configuredConnector(d.ID)
	if configured.Name != "Concurrent edit" || slices.Contains(strings.Fields(configured.OAuthScopes), connector.GoogleSendOAuthScope) {
		t.Fatal("concurrent settings edit overwritten")
	}
}
