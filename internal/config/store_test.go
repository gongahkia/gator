package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreReturnsDefaultsUntilConfigured(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	if !reflect.DeepEqual(settings, Default()) {
		t.Fatalf("settings = %#v, want defaults", settings)
	}
}

func TestStoreSavesPrivateAtomicSettings(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	settings := Default()
	settings.Defaults = Defaults{Provider: "anthropic", Model: "claude-sonnet"}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(loaded, settings) {
		t.Fatalf("loaded = %#v, want %#v", loaded, settings)
	}
	info, err := os.Stat(filepath.Join(root, "gator", "config.json"))
	if err != nil {
		t.Fatalf("stat configuration: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration permissions = %o", info.Mode().Perm())
	}
}

func TestStoreRejectsUnknownVersion(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save(Settings{Version: 99}); err == nil {
		t.Fatal("saved unsupported version")
	}
}

func TestStoreRejectsInvalidExtensionTrust(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	settings := Default()
	settings.ExtensionTrusts = []ExtensionTrust{{Repository: "/workspace/project", Hash: "not-a-hash"}}
	if err := store.Save(settings); err == nil {
		t.Fatal("saved invalid extension trust")
	}
}

func TestStoreRejectsInvalidCustomProvider(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	settings := Default()
	settings.CustomProviders = []CustomProvider{{
		ID: "OpenAI", BaseURL: "https://example.com/v1/chat/completions", Models: []string{"x"}, DefaultModel: "x",
	}}
	if err := store.Save(settings); err == nil {
		t.Fatal("uppercase custom provider ID was accepted")
	}
	settings.CustomProviders = []CustomProvider{{
		ID: "team-gateway", BaseURL: "https://user:pass@example.com/v1/chat/completions", Models: []string{"x"}, DefaultModel: "x",
	}}
	if err := store.Save(settings); err == nil {
		t.Fatal("URL userinfo was accepted")
	}
	settings.CustomProviders = []CustomProvider{{
		ID: "team-gateway", BaseURL: "https://example.com/v1/chat/completions", APIKeyEnv: "not-an-env", Models: []string{"x"}, DefaultModel: "x",
	}}
	if err := store.Save(settings); err == nil {
		t.Fatal("lowercase API key env was accepted")
	}
	settings.CustomProviders = []CustomProvider{{
		ID: "team-gateway", BaseURL: "https://example.com/v1/chat/completions", Models: []string{"x"}, DefaultModel: "missing",
	}}
	if err := store.Save(settings); err == nil {
		t.Fatal("default model outside the catalog was accepted")
	}
}

func TestStorePersistsNonSecretProviderOptions(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	settings := Default()
	settings.ProviderOptions = map[string]map[string]string{
		"google-vertex": {"project": "project-123", "location": "us-central1"},
	}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save provider options: %v", err)
	}
	options := settings.OptionsForProvider("google-vertex")
	options["project"] = "mutated"
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if got := loaded.OptionsForProvider("google-vertex"); got["project"] != "project-123" || got["location"] != "us-central1" {
		t.Fatalf("provider options = %#v", got)
	}
}
