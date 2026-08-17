package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoginStoresAzureResponsesBearerTokenFromEnvironment(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("AZURE_OPENAI_AUTH_TOKEN", "entra-token")
	var output bytes.Buffer
	if err := login([]string{"azure-openai-responses", "--bearer-token-from-env", "AZURE_OPENAI_AUTH_TOKEN"}, &output); err != nil {
		t.Fatalf("login: %v", err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	stored, found, err := credentials.Read("azure-openai-responses")
	if err != nil || !found || !stored.IsBearerToken() || stored.Access != "entra-token" {
		t.Fatalf("stored credential = %#v, found=%v, err=%v", stored, found, err)
	}
	if strings.Contains(output.String(), "entra-token") {
		t.Fatalf("login output leaked bearer token: %q", output.String())
	}
}

func TestLoginRejectsMixedCredentialModes(t *testing.T) {
	var output bytes.Buffer
	err := login([]string{"azure-openai-responses", "--api-key", "api-key", "--bearer-token", "entra-token"}, &output)
	if err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("mixed login modes error = %v", err)
	}
}

func TestLoginRejectsClaudeAISubscriptionOAuth(t *testing.T) {
	var output bytes.Buffer
	err := login([]string{"claude"}, &output)
	if err == nil || !strings.Contains(err.Error(), "not a supported Gator login") || !strings.Contains(err.Error(), "gator connect claude") {
		t.Fatalf("Claude login error = %v", err)
	}
}
