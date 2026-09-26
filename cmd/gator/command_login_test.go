package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoginStoresAPIKeyFromEnvironment(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-key")
	var output bytes.Buffer
	if err := login([]string{"azure-openai-responses", "--from-env", "AZURE_OPENAI_API_KEY"}, &output); err != nil {
		t.Fatalf("login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	stored, found, err := credentials.Read("azure-openai-responses")
	if err != nil || !found || !stored.IsAPIKey() || stored.Key != "azure-key" {
		t.Fatalf("stored credential = %#v found=%v err=%v", stored, found, err)
	}
	if strings.Contains(output.String(), "azure-key") {
		t.Fatalf("login output leaked API key: %q", output.String())
	}
}

func TestLoginRejectsMixedCredentialModes(t *testing.T) {
	var output bytes.Buffer
	err := login([]string{"openai", "--api-key", "api-key", "--from-env", "OPENAI_API_KEY"}, &output)
	if err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("mixed login modes error = %v", err)
	}
}

func TestLoginRejectsRemovedSubscriptionProvider(t *testing.T) {
	var output bytes.Buffer
	err := login([]string{"codex", "--api-key", "must-not-store"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("removed provider error = %v", err)
	}
}
