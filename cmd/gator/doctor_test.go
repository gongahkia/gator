package main

import (
	"bytes"
	"strings"
	"testing"
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
