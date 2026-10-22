package localmodel

import "testing"

func TestWindowsHostResources(t *testing.T) {
	t.Setenv("OLLAMA_MODELS", t.TempDir())
	host := DetectHost()
	if host.TotalMemoryBytes == 0 || !host.AvailableMemoryKnown || !host.AvailableDiskKnown || host.ModelDirectory == "" {
		t.Fatalf("host resource inspection failed: %#v", host)
	}
}
