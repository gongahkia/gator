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
