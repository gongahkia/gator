package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestThemeCommandPersistsNamedTheme(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := themeCommand([]string{"set", "contrast"}, &output); err != nil {
		t.Fatalf("set theme: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("default store: %v", err)
	}
	settings, err := store.Load()
	if err != nil || settings.Theme != "contrast" {
		t.Fatalf("settings = %#v, err = %v", settings, err)
	}
	if err := themeCommand([]string{"set", "rainbow"}, &output); err == nil || !strings.Contains(err.Error(), "unknown theme") {
		t.Fatalf("invalid theme error = %v", err)
	}
}
