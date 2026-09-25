package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/connector"
)

func TestConnectorCommandAddsAuthenticatesAndRemovesResourceBoundSource(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("METRICS_TOKEN", "never-print-this-token")
	var output bytes.Buffer
	if err := connectorCommandWithIO([]string{"add", "metrics", "--url", "https://example.com/metrics.json", "--name", "Team metrics", "--auth", "bearer"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "never-print-this-token") {
		t.Fatal("connector add leaked a credential")
	}
	output.Reset()
	if err := connectorCommandWithIO([]string{"login", "metrics", "--from-env", "METRICS_TOKEN"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "never-print-this-token") || !strings.Contains(output.String(), "resource-bound") {
		t.Fatalf("connector login output = %q", output.String())
	}
	output.Reset()
	if err := connectorCommandWithIO([]string{"list"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "metrics") || !strings.Contains(output.String(), "authenticated") || strings.Contains(output.String(), "never-print-this-token") {
		t.Fatalf("connector list output = %q", output.String())
	}
	if err := connectorCommandWithIO([]string{"remove", "metrics"}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("connector removal without --yes succeeded")
	}
	output.Reset()
	if err := connectorCommandWithIO([]string{"remove", "metrics", "--yes"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "cannot be undone") {
		t.Fatalf("connector removal output = %q", output.String())
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	providers, err := credentials.Providers()
	if err != nil || len(providers) != 0 {
		t.Fatalf("orphaned connector credentials = %#v, %v", providers, err)
	}
}

func TestConnectorCommandTestsConfiguredJSONSourceWithoutPrintingData(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"private":"payload"}`))
	}))
	defer server.Close()
	if err := connectorCommandWithIO([]string{"add", "source", "--url", server.URL}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := connectorCommandWithIO([]string{"test", "source"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "is reachable") || !strings.Contains(output.String(), "sha256:") || strings.Contains(output.String(), "payload") {
		t.Fatalf("connector test output = %q", output.String())
	}
}

func TestConnectorTestInvocationUsesMappedRemoteMCPReadOperation(t *testing.T) {
	tests := []struct {
		name       string
		descriptor connector.Descriptor
		operation  string
	}{
		{name: "search preferred", descriptor: connector.Descriptor{Kind: connector.KindRemoteMCP, SearchTool: "search_issues", ReadTool: "get_issue"}, operation: "search"},
		{name: "read fallback", descriptor: connector.Descriptor{Kind: connector.KindRemoteMCP, ReadTool: "get_issue"}, operation: "read"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation, input := connectorTestInvocation(test.descriptor)
			if operation != test.operation || string(input) != `{"arguments":{}}` {
				t.Fatalf("invocation = %q, %s", operation, input)
			}
		})
	}
}

func TestConnectorLoginReadsTokenFromStdin(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	if err := connectorCommandWithIO([]string{"add", "source", "--url", "https://example.com/data", "--auth", "bearer"}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := connectorCommandWithIO([]string{"login", "source", "--token-stdin"}, strings.NewReader("stdin-token\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read(settings.Connectors[0].CredentialRef())
	if err != nil || !found || credential.Access != "stdin-token" {
		t.Fatalf("credential = %#v, found=%v, err=%v", credential, found, err)
	}
	if _, err := os.Stat(credentials.Path()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorCommandConfiguresWebhookWithoutTestingItsSideEffect(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	var output bytes.Buffer
	if err := connectorCommandWithIO([]string{"add", "release", "--kind", "webhook", "--url", server.URL}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := connectorCommandWithIO([]string{"status", "release"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "http_webhook") || !strings.Contains(output.String(), "publish: publish") {
		t.Fatalf("webhook status = %q", output.String())
	}
	output.Reset()
	err := connectorCommandWithIO([]string{"test", "release"}, strings.NewReader(""), &output)
	if err == nil || !strings.Contains(err.Error(), "--actions draft") || requests != 0 {
		t.Fatalf("webhook test error=%v requests=%d", err, requests)
	}
}

func TestConnectorCommandDefaultsGoogleToUserOwnedDesktopOAuthAndGranularOperations(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	if err := connectorCommandWithIO([]string{"add", "google-work", "--kind", "google", "--oauth-client-id", "desktop-client"}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	settings, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Connectors) != 1 {
		t.Fatalf("connectors = %#v", settings.Connectors)
	}
	descriptor := settings.Connectors[0]
	if descriptor.Authentication != connector.AuthOAuth || descriptor.OAuthAuthorizeURL != connector.GoogleOAuthAuthorizeURL || descriptor.OAuthTokenURL != connector.GoogleOAuthTokenURL || descriptor.OAuthRedirectURL != connector.GoogleOAuthRedirectURL || strings.Contains(descriptor.OAuthScopes, "/auth/tasks") || strings.Contains(descriptor.OAuthScopes, "/auth/calendar") {
		t.Fatalf("Google descriptor = %#v", descriptor)
	}
	operations := map[string]bool{}
	for _, operation := range descriptor.Operations() {
		operations[operation.ID] = true
	}
	for _, required := range []string{"drive_search", "docs_update", "sheets_values_update", "batch"} {
		if !operations[required] {
			t.Fatalf("Google operation %q is missing from %#v", required, operations)
		}
	}
	for _, removed := range []string{
		"tasklists_list", "tasklists_get", "tasks_list", "tasks_get", "tasks_create", "tasks_update", "tasks_delete", "tasks_move",
		"calendars_list", "calendars_get", "calendars_create", "calendars_update", "calendars_delete", "calendars_subscribe", "calendars_unsubscribe", "calendars_list_update", "calendar_colors",
		"events_list", "events_get", "events_create", "events_update", "events_delete", "events_move", "events_respond", "freebusy", "local_search", "quick_capture", "task_metadata_encode", "task_metadata_decode", "reminders_due",
	} {
		if operations[removed] {
			t.Fatalf("removed Google operation %q remains in %#v", removed, operations)
		}
	}
}

func TestWithoutGooglePlannerScopesPreservesOtherOAuthScopes(t *testing.T) {
	scopes := withoutGooglePlannerScopes([]string{"openid", "https://www.googleapis.com/auth/tasks", "https://www.googleapis.com/auth/documents", "https://www.googleapis.com/auth/calendar"})
	if actual := strings.Join(scopes, " "); actual != "openid https://www.googleapis.com/auth/documents" {
		t.Fatalf("scopes = %q", actual)
	}
}
