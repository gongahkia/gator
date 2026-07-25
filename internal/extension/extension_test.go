package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
)

type testProvider struct{ id, version string }

func (p testProvider) ID() string                                        { return p.id }
func (p testProvider) APIVersion() string                                { return p.version }
func (p testProvider) Supports(domain.Stage) bool                        { return true }
func (p testProvider) Invoke(context.Context, Request) (Response, error) { return Response{}, nil }

func TestRegistryRejectsIncompatibleVersion(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProvider(testProvider{id: "provider", version: "v0"}); err == nil {
		t.Fatal("expected version error")
	}
	if err := registry.RegisterProvider(testProvider{id: "provider", version: APIVersion}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProvider(testProvider{id: "provider", version: APIVersion}); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestProcessPluginLoadsAndRegisters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin.sh")
	script := "#!/bin/sh\nread line\nprintf '%s\\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"api_version\":\"v1\",\"providers\":[{\"id\":\"external\",\"stages\":[\"planner\"]}]}}'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(script))
	processes, err := LoadProcessPlugins([]config.ProcessPlugin{{ID: "test", Command: path, SHA256: hex.EncodeToString(digest[:]), Methods: []string{"norbot.initialize", "provider.invoke"}}})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterProcessPlugins(context.Background(), registry, processes); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Providers["external"]; !ok {
		t.Fatal("provider capability was not registered")
	}
}
