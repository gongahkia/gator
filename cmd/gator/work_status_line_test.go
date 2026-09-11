package main

import (
	"reflect"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestSaveWorkStatusLinePersistsItemsHideAndReset(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	save := saveWorkStatusLine(store)
	items := []string{"model", "model-access", "current-dir"}
	if err := save(&items); err != nil {
		t.Fatalf("save items: %v", err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	if settings.TUI.StatusLine == nil || !reflect.DeepEqual(*settings.TUI.StatusLine, items) {
		t.Fatalf("status line = %#v, want %#v", settings.TUI.StatusLine, items)
	}

	empty := []string{}
	if err := save(&empty); err != nil {
		t.Fatalf("hide: %v", err)
	}
	settings, err = store.Load()
	if err != nil {
		t.Fatalf("load hidden: %v", err)
	}
	if settings.TUI.StatusLine == nil || len(*settings.TUI.StatusLine) != 0 {
		t.Fatalf("hidden status line = %#v", settings.TUI.StatusLine)
	}

	if err := save(nil); err != nil {
		t.Fatalf("reset: %v", err)
	}
	settings, err = store.Load()
	if err != nil {
		t.Fatalf("load reset: %v", err)
	}
	if settings.TUI.StatusLine != nil {
		t.Fatalf("reset status line = %#v, want nil", settings.TUI.StatusLine)
	}
}
