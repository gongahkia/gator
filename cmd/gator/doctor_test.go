package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/auth"
)

func TestDoctorDescribesVertexADCInsteadOfGatorCredential(t *testing.T) {
	t.Setenv("GATOR_VERTEX_ACCESS_TOKEN", "vertex-access-token")
	var output bytes.Buffer
	if err := doctor([]string{"--provider", "google-vertex"}, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Google Application Default Credentials") || !strings.Contains(text, "configured") || strings.Contains(text, "Gator credential") {
		t.Fatalf("doctor output = %q", text)
	}
}

func TestDoctorDescribesAzureResponsesEntraTokenWithoutExposingIt(t *testing.T) {
	t.Setenv("AZURE_OPENAI_AUTH_TOKEN", "do-not-print-this-token")
	var output bytes.Buffer
	if err := doctor([]string{"--provider", "azure-openai-responses"}, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "AZURE_OPENAI_AUTH_TOKEN") || !strings.Contains(text, "set in environment") || strings.Contains(text, "do-not-print-this-token") {
		t.Fatalf("doctor output = %q", text)
	}
}

func TestDoctorReportsStoredAzureResponsesBearerTokenWithoutExposingIt(t *testing.T) {
	stateDirectory := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDirectory)
	t.Setenv("AZURE_OPENAI_AUTH_TOKEN", "")
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	if err := credentials.Put("azure-openai-responses", auth.Credential{Type: "bearer_token", Access: "do-not-print-this-token"}); err != nil {
		t.Fatalf("store credential: %v", err)
	}
	var output bytes.Buffer
	if err := doctor([]string{"--provider", "azure-openai-responses"}, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "stored") || strings.Contains(text, "do-not-print-this-token") {
		t.Fatalf("doctor output = %q", text)
	}
}

func TestDoctorReportsAmbientBedrockAuthenticationWithoutExposingCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "do-not-print-this-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "do-not-print-this-secret")
	var output bytes.Buffer
	if err := doctor([]string{"--provider", "amazon-bedrock"}, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "AWS static credentials in environment") || strings.Contains(text, "do-not-print-this-key") || strings.Contains(text, "do-not-print-this-secret") {
		t.Fatalf("doctor output = %q", text)
	}
}
