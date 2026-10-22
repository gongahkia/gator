package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestGatorAppConnectionSelectionPersistsWithoutChangingCurrentAuthority(t *testing.T) {
	c := appControllerForTest(t)
	settings, _ := c.settings.Load()
	settings.Connectors = []connector.Descriptor{{Version: connector.DescriptorVersion, ID: "reports", Name: "Reports", Kind: connector.KindHTTPJSON, Resource: "https://example.com/reports.json", Authentication: connector.AuthNone}}
	if err := c.settings.Save(settings); err != nil {
		t.Fatal(err)
	}
	store, _ := worksession.Open(c.stateDir)
	conversation, err := store.Create("Test", t.TempDir(), "snap-fixture", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	change := workrun.AppChange{ConversationID: conversation.ID, Setting: "connections", Value: json.RawMessage(`{"ids":["reports"]}`)}
	mutation, err := c.Prepare(context.Background(), change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	options, err := loadWorkTUIConversationOptions(store, conversation.ID)
	if err != nil || len(options.ConnectorIDs) != 1 || options.ConnectorIDs[0] != "reports" {
		t.Fatalf("saved selections lost: %#v %v", options, err)
	}
	if len(c.request.ConnectorIDs) != 0 {
		t.Fatal("selection widened the current run's authority")
	}
	if _, err := mutation.Apply(context.Background()); err == nil {
		t.Fatal("stale connection selection was applied again")
	}
}

func TestGatorAppDataDeletionOnlyOpensCanonicalReview(t *testing.T) {
	c := appControllerForTest(t)
	directory := filepath.Join(c.stateDir, "gator", "knowledge")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "kept.md")
	if err := os.WriteFile(file, []byte("Keep until a final review is approved."), 0600); err != nil {
		t.Fatal(err)
	}
	mutation, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "data_action", Value: json.RawMessage(`{"action":"delete","scope":"knowledge"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.AfterRun {
		t.Fatal("data review ran while the current work was active")
	}
	result, err := mutation.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.(appScreenResult).Screen.Panel.Kind != "confirmation" {
		t.Fatal("deletion did not produce the canonical final review")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("data was deleted before confirmation")
	}
}
