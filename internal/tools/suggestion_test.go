package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/suggestion"
)

func TestSuggestChangeQueuesScopedProposal(t *testing.T) {
	store, err := suggestion.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := SuggestChange{Store: store, WorkID: "work-1"}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"plan","plan_change":{"kind":"add_step","title":"Check source"}}`)); err == nil {
		t.Fatal("unbound goal plan proposal accepted")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"schedule","cron":"0 9 * * *","timezone":"UTC","task":"Check"}`)); err == nil {
		t.Fatal("unbound goal schedule proposal accepted")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"note","title":"Preference","body":"Use short briefs"}`)); err != nil {
		t.Fatal(err)
	}
	records, err := store.List("pending")
	if err != nil || len(records) != 1 || records[0].Kind != "note" || records[0].WorkID != "work-1" {
		t.Fatalf("queued = %#v, %v", records, err)
	}
}
