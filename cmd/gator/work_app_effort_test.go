package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestGatorAppEffortUsesCanonicalTaskPreferencesAndRejectsStaleChanges(t *testing.T) {
	c := appControllerForTest(t)
	s, err := worksession.Open(c.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"reports"}
	if err := s.SavePreferences(chat.ID, worksession.Preferences{ConnectorIDs: &ids}, nil); err != nil {
		t.Fatal(err)
	}
	c.request.ConversationID, c.request.MaxSteps, c.request.Code.MaxSteps = chat.ID, 24, 16
	change := workrun.AppChange{ConversationID: chat.ID, Setting: "work_effort", Value: json.RawMessage(`{"effort":"high"}`)}
	mutation, err := c.Prepare(context.Background(), change)
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.AfterRun {
		t.Fatal("effort change can alter current work")
	}
	before, _ := s.LoadPreferences(chat.ID)
	if before.WorkEffort != nil {
		t.Fatal("preparation persisted effort")
	}
	view, err := mutation.Apply(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	actual := view.(map[string]any)
	if actual["effort"] != "high" || actual["work_steps"] != 48 || actual["code_steps"] != 32 || c.request.MaxSteps != 24 || c.request.Code.MaxSteps != 16 {
		t.Fatal("saved default widened original task or reported wrong budget", actual)
	}
	saved, _ := s.LoadPreferences(chat.ID)
	if saved.ConnectorIDs == nil || (*saved.ConnectorIDs)[0] != "reports" {
		t.Fatal("effort removed account selection")
	}
	connections, err := c.Prepare(context.Background(), workrun.AppChange{ConversationID: chat.ID, Setting: "connections", Value: json.RawMessage(`{"ids":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connections.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, _ = s.LoadPreferences(chat.ID)
	if saved.WorkEffort == nil || *saved.WorkEffort != "high" || saved.ConnectorIDs == nil || len(*saved.ConnectorIDs) != 0 {
		t.Fatal("source selection erased work budget", saved)
	}
	stale, err := c.Prepare(context.Background(), workrun.AppChange{ConversationID: chat.ID, Setting: "work_effort", Value: json.RawMessage(`{"effort":"standard"}`)})
	if err != nil {
		t.Fatal(err)
	}
	low := "low"
	next := saved
	next.WorkEffort = &low
	if err := s.SavePreferences(chat.ID, next, &saved); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.Apply(context.Background()); err == nil {
		t.Fatal("stale conversational preference overwrote manual choice")
	}
	for _, bad := range []string{`{}`, `{"effort":"max"}`, `{"effort":"high","max_steps":999}`} {
		if _, err := c.Prepare(context.Background(), workrun.AppChange{ConversationID: chat.ID, Setting: "work_effort", Value: json.RawMessage(bad)}); err == nil {
			t.Fatal("opaque/unsupported allowance accepted", bad)
		}
	}
}
