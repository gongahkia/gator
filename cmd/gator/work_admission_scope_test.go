package main

import (
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
)

func TestConversationAdmissionRetainsMissingReferencesWithoutExecutionAuthority(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	options := workapp.RunOptions{Mode: "inspect", ConnectorIDs: []string{"missing"}}
	input, err := captureConversationAdmission(workapp.ConversationInput{Version: 1, RunOptions: options})
	if err != nil || input.ScopeDigest == "" || input.ProcessingDigest == "" {
		t.Fatal("missing selection prevented conversational capture", err)
	}
	if err := verifyConversationScopeFor(input, true); err != nil {
		t.Fatal("unchanged unavailable selection blocked tool-free conversation", err)
	}
	if err := verifyConversationScope(input); err == nil {
		t.Fatal("missing selection acquired execution authority")
	}
	settings.Connectors = []connector.Descriptor{{Version: 1, ID: "missing", Name: "Repaired source", Kind: connector.KindHTTPJSON, Resource: "https://example.com", Authentication: connector.AuthNone}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	if err := verifyConversationScopeFor(input, true); err == nil {
		t.Fatal("repair silently widened an admitted conversation's scope")
	}
	repaired, err := captureConversationInput(workapp.ConversationInput{Version: 1, RunOptions: options})
	if err != nil || repaired.ProcessingDigest != input.ProcessingDigest || repaired.ScopeDigest == input.ScopeDigest {
		t.Fatal("repair changed processing or failed to change account scope", err)
	}
}
