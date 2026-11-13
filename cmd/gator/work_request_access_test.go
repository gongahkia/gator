package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestAccountRereviewKeepsAccountsStillChosenByAnotherRequirement(t *testing.T) {
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
	settings.Connectors = []connector.Descriptor{
		{Version: 1, ID: "slack-a", Name: "Account A", Kind: connector.KindSlack, Resource: "https://example.com/a", Authentication: connector.AuthNone},
		{Version: 1, ID: "slack-b", Name: "Account B", Kind: connector.KindSlack, Resource: "https://example.com/b", Authentication: connector.AuthNone},
	}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	options := workapp.RunOptions{Mode: "inspect", ConnectorIDs: []string{"slack-a"}}
	input, err := captureConversationInput(workapp.ConversationInput{Version: 1, RunOptions: options})
	if err != nil {
		t.Fatal(err)
	}
	needs := []worksession.TaskAccessNeed{{ConnectorID: "slack-a", Kind: connector.KindSlack, Operation: "search"}, {ConnectorID: "slack-a", Kind: connector.KindSlack, Operation: "history"}}
	raw, _ := json.Marshal(options)
	r := worksession.Request{ID: "source-review", Options: raw, Recovery: &worksession.RequestRecovery{Needs: needs, ScopeDigest: input.ScopeDigest, ProcessingDigest: input.ProcessingDigest}, Plan: &worksession.TaskPlan{InputChanges: []worksession.PlanInputChange{{Access: &worksession.PlanAccessChange{Requirements: []worksession.TaskAccessSelection{{Need: needs[0], ConnectorID: "slack-a"}, {Need: needs[1], ConnectorID: "slack-a"}}}}}}}
	for _, selection := range [][]string{{"slack-a", "slack-b"}, {"slack-b", "slack-a"}} {
		review, err := reviewRequestAccess(r, selection)
		if err != nil {
			t.Fatal(err)
		}
		if !review.Ready || !reflect.DeepEqual(review.Connections, []string{"slack-a", "slack-b"}) {
			t.Fatal("one requirement removed another's chosen account", selection, review.Connections, review.Ready)
		}
	}
}
