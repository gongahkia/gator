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

func TestGoogleGoalWatchStopsBeforeNetworkWhenAuthorizationChanges(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var networkCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		networkCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
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
		} else if saved.OAuthRefreshPins["google"] == "" {
			t.Fatalf("scheduled goal did not pin Google authorization: %#v", saved)
		}
	}
	if definition.OAuthRefreshPins["google"] == "" || strings.Contains(definition.OAuthRefreshPins["google"], credential.Refresh) {
		t.Fatalf("OAuth grant was not safely pinned: %#v", definition.OAuthRefreshPins)
	}
	credential.Access, credential.Refresh = "access-B", "refresh-B"
	if err := credentials.Put(descriptor.CredentialRef(), credential); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connectedGoalWatchSnapshot(context.Background(), definition, stateDir, time.Now()); err == nil || !strings.Contains(err.Error(), "authorization changed") || networkCalls.Load() != 0 {
		t.Fatalf("watch crossed OAuth grant: %v, calls=%d", err, networkCalls.Load())
	}
	output.Reset()
	if err := probeGoalAccess(context.Background(), stateDir, record.ID, &output); err != nil || !strings.Contains(output.String(), "authorization changed") || networkCalls.Load() != 0 {
		t.Fatalf("access probe crossed OAuth grant: %v, calls=%d, output=%q", err, networkCalls.Load(), output.String())
	}
	called := false
	if _, err := runJobProcessWith(context.Background(), definition, stateDir, func(context.Context, string, string, string, workrun.Request) (workrun.Outcome, error) {
		called = true
		return workrun.Outcome{}, nil
	}); err == nil || !strings.Contains(err.Error(), "authorization changed") || called {
		t.Fatalf("scheduled Work crossed OAuth grant: %v, executed=%t", err, called)
	}
	readiness := strings.Join(goalConnectorReadiness(definition), "\n")
	if !strings.Contains(readiness, "authorization changed") {
		t.Fatalf("goal activity omitted authorization change: %s", readiness)
	}
}
