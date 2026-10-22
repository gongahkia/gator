package main

import (
	"fmt"
	"sync"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
)

func TestWorkModelSelectionPreservesConcurrentConnectionSetup(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 20; i++ {
				if err := (workModelDefaults{settings: store}).SetDefaults("openai", "gpt-5"); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 20; i++ {
			id := fmt.Sprintf("service-%d", i)
			if err := store.Update(func(s *config.Settings) error {
				s.Connectors = append(s.Connectors, connector.Descriptor{Version: 1, ID: id, Name: id, Kind: connector.KindHTTPJSON, Resource: "https://example.com/" + id, Authentication: connector.AuthNone})
				return nil
			}); err != nil {
				t.Error(err)
			}
		}
	}()
	close(start)
	wg.Wait()
	saved, err := store.Load()
	if err != nil || len(saved.Connectors) != 20 || saved.Defaults.Model != "gpt-5" {
		t.Fatal("model selection overwrote concurrent configuration", len(saved.Connectors), err)
	}
}
