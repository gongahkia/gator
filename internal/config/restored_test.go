package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestoredProfileUsesFreshConfigUnlessExplicitlyOverridden(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	marker := filepath.Join(root, "gator", "data-control")
	if err := os.MkdirAll(marker, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(marker, "restored.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := ResolveDir("")
	if err != nil || dir != filepath.Join(root, "config") {
		t.Fatal("restored profile inherited global configuration", dir, err)
	}
	override := t.TempDir()
	t.Setenv("GATOR_CONFIG_DIR", override)
	dir, err = ResolveDir("")
	if err != nil || dir != override {
		t.Fatal("explicit configuration override was lost")
	}
}
