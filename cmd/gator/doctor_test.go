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
