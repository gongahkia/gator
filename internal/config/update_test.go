package config

import (
	"errors"
	"sync"
	"testing"
)

func TestSettingsUpdatesPreserveConcurrentEdits(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 20 {
		group.Go(func() {
			if err := store.Update(func(s *Settings) error { s.Snapshots.MaxFiles++; s.Theme = "mono"; return nil }); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	settings, err := store.Load()
	if err != nil || settings.Snapshots.MaxFiles != Default().Snapshots.MaxFiles+20 || settings.Theme != "mono" {
		t.Fatalf("concurrent updates lost settings: %#v, %v", settings, err)
	}
	if err := store.Update(func(s *Settings) error { s.Theme = "contrast"; return errors.New("cancelled") }); err == nil {
		t.Fatal("failed update reported success")
	}
	settings, _ = store.Load()
	if settings.Theme != "mono" {
		t.Fatal("failed callback persisted a partial update")
	}
}
