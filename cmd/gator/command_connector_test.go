package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
