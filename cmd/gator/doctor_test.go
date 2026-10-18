package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/localmodel"
)

func TestDoctorReportsLocalModelHostAndDisabledCatalogEntries(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	previous := inspectLocalModelHost
	inspectLocalModelHost = func() localmodel.Host {
		return localmodel.Host{
			OS:                   "linux",
			Architecture:         "amd64",
			TotalMemoryBytes:     16 << 30,
			AvailableMemoryBytes: 16 << 30,
			ModelDirectory:       "/models",
			ModelDirectorySource: "OLLAMA_MODELS",
			AvailableDiskBytes:   64 << 30,
		}
	}
	t.Cleanup(func() { inspectLocalModelHost = previous })
	var output bytes.Buffer
	if err := doctor(nil, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Local model host: linux/amd64") || !strings.Contains(text, "Ollama model storage: /models (OLLAMA_MODELS)") || !strings.Contains(text, "qwen3-coder-30b: disabled") || !strings.Contains(text, "Gator guardrail") {
		t.Fatalf("doctor local model output = %q", text)
	}
}

func TestDoctorReportsConfiguredWebSearchWithoutToken(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("BRAVE_SEARCH_API_KEY", "do-not-print-this-search-token")
	var output bytes.Buffer
	if err := doctor(nil, &output); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "Web search: configured (requires --network allow)") || strings.Contains(text, "do-not-print-this-search-token") {
		t.Fatalf("doctor web search output = %q", text)
	}
}

func TestDoctorStructuredResultCarriesDiagnosticState(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	previous := inspectLocalModelHost
	inspectLocalModelHost = func() localmodel.Host {
		return localmodel.Host{OS: "linux", Architecture: "amd64", TotalMemoryBytes: 8 << 30,
			ModelDirectory: "/models", AvailableDiskBytes: 32 << 30}
	}
	t.Cleanup(func() { inspectLocalModelHost = previous })
	result, err := doctorResult(nil)
	if err != nil {
		t.Fatalf("doctor result: %v", err)
	}
	if result.Title != "Gator doctor" || len(result.Sections) < 4 || result.Sections[3].Title != "Reviewed local models" {
		t.Fatalf("doctor result = %#v", result)
	}
	var disabled bool
	for _, item := range result.Sections[3].Items {
		if item.Label == "qwen3-30b" && item.Tone == "warning" && strings.HasPrefix(item.State, "disabled:") {
			disabled = true
		}
	}
	if !disabled {
		t.Fatalf("expected typed disabled model in %#v", result.Sections[3].Items)
	}
}
