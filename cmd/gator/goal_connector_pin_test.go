package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestGoogleGoalWatchFollowsSameAccountAndStopsBeforeReadingDifferentAccount(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var connectedReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/oauth2/v3/userinfo" {
			if request.Header.Get("Authorization") == "Bearer access-C" {
				_, _ = writer.Write([]byte(`{"sub":"account-B"}`))
			} else {
				_, _ = writer.Write([]byte(`{"sub":"account-A"}`))
			}
			return
		}
		connectedReads.Add(1)
		if request.URL.Path == "/gmail/v1/users/me/profile" {
			_, _ = writer.Write([]byte(`{"historyId":"100"}`))
			return
		}
		if request.URL.Path == "/gmail/v1/users/me/messages" {
			_, _ = writer.Write([]byte(`{"messages":[]}`))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	descriptor := connector.Descriptor{
		Version: connector.DescriptorVersion, ID: "google", Name: "Google", Kind: connector.KindGoogle,
		Resource: server.URL, Authentication: connector.AuthOAuth,
		OAuthClientID: "client", OAuthAuthorizeURL: server.URL + "/authorize",
		OAuthTokenURL: server.URL + "/token", OAuthRedirectURL: "http://127.0.0.1:1457/oauth/callback",
		OAuthScopes: connector.GoogleOAuthScopes + " " + connector.GoogleDailyOAuthScopes,
	}
	settings := config.Default()
	settings.Connectors = []connector.Descriptor{descriptor}
	configStore, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := configStore.Save(settings); err != nil {
		t.Fatal(err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	credential := auth.Credential{Type: "oauth", Access: "access-A", Refresh: "refresh-A", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	goalStore, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goalStore.Add("Watch offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goalStore.InitializePlan(record.ID, "Choose a venue", []string{"Check availability"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"watch", record.ID, "--connector", "google", "--gmail-query", "label:offsite"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := goalCommand([]string{"schedule", record.ID, "--cron", "0 9 * * *", "--connector", "google", "--task", "Review offsite changes"}, &output); err != nil {
		t.Fatal(err)
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := jobStore.List()
	if err != nil || len(definitions) != 2 {
		t.Fatalf("saved watch = %#v, %v", definitions, err)
	}
	var definition jobs.Definition
	for _, saved := range definitions {
		if saved.GoalWatch {
			definition = saved
		} else if saved.OAuthAccountPins["google"] == "" {
			t.Fatalf("scheduled goal did not pin Google authorization: %#v", saved)
		}
	}
	if definition.OAuthAccountPins["google"] != connector.GoogleAccountPin(descriptor, "account-A") || len(definition.OAuthRefreshPins) != 0 {
		t.Fatalf("Google account was not pinned by stable subject: %#v", definition)
	}
	credential.Access, credential.Refresh = "access-B", "refresh-B"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connectedGoalWatchSnapshot(context.Background(), definition, stateDir, time.Now()); err != nil || connectedReads.Load() == 0 {
		t.Fatalf("same account was blocked after reauthorization: %v, reads=%d", err, connectedReads.Load())
	}
	output.Reset()
	if err := probeGoalAccess(context.Background(), stateDir, record.ID, &output); err != nil || !strings.Contains(output.String(), "list reachable") {
		t.Fatalf("same-account access probe = %v, output=%q", err, output.String())
	}
	called := false
	if _, err := runJobProcessWith(context.Background(), definition, stateDir, func(_ context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
		called = true
		if request.OAuthAccountPins["google"] != definition.OAuthAccountPins["google"] {
			t.Fatal("account pin was not carried into scheduled Work")
		}
		return workrun.Outcome{}, nil
	}); err != nil || !called {
		t.Fatalf("same-account scheduled Work = %v, executed=%t", err, called)
	}
	credential.Access, credential.Refresh = "access-C", "refresh-C"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	before := connectedReads.Load()
	if _, _, err := connectedGoalWatchSnapshot(context.Background(), definition, stateDir, time.Now()); err == nil || !strings.Contains(err.Error(), "account changed") || connectedReads.Load() != before {
		t.Fatalf("watch crossed account: %v, reads=%d", err, connectedReads.Load()-before)
	}
	output.Reset()
	if err := probeGoalAccess(context.Background(), stateDir, record.ID, &output); err != nil || !strings.Contains(output.String(), "account changed") || connectedReads.Load() != before {
		t.Fatalf("access probe crossed account: %v, reads=%d, output=%q", err, connectedReads.Load()-before, output.String())
	}
	called = false
	if _, err := runJobProcessWith(context.Background(), definition, stateDir, func(context.Context, string, string, string, workrun.Request) (workrun.Outcome, error) {
		called = true
		return workrun.Outcome{}, nil
	}); err == nil || !strings.Contains(err.Error(), "account changed") || called {
		t.Fatalf("scheduled Work crossed account: %v, executed=%t", err, called)
	}
	readiness := strings.Join(goalConnectorReadiness(definition), "\n")
	if !strings.Contains(readiness, "account pinned") {
		t.Fatalf("goal activity omitted account pin: %s", readiness)
	}
}
