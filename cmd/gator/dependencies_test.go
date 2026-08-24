package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/dependency"
)

func TestWriteDependencyDoctorOffersOfficialGuidanceForMissingPrerequisite(t *testing.T) {
	var output bytes.Buffer
	statuses := []dependency.Status{{Requirement: dependency.Requirement{
		ID:      "ollama",
		Name:    "Ollama",
		Purpose: "reviewed local coding models",
		HelpURL: "https://ollama.com/download",
		Instructions: func(string) []string {
			return []string{"Install Ollama from its official download page."}
		},
	}}}
	if err := writeDependencyDoctor(&output, statuses); err != nil {
		t.Fatalf("write dependency doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Ollama: missing") || !strings.Contains(text, "https://ollama.com/download") || !strings.Contains(text, "official download page") {
		t.Fatalf("dependency doctor output = %q", text)
	}
}
