package workhistory

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
)

func TestStoreListsRecordsDeterministically(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	for _, input := range []struct {
		id, conversation string
		at               time.Time
	}{
		{"work-b", "conversation-one", base},
		{"work-a", "conversation-one", base},
		{"work-c", "conversation-two", base.Add(time.Minute)},
	} {
		if _, err := store.Start(Start{ID: input.id, Objective: "Inspect the workspace", Mode: action.Inspect, ExternalActions: action.Forbid, ConversationID: input.conversation, StartedAt: input.at}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Finish(input.id, Finish{ConversationID: input.conversation, SnapshotID: "snap-" + input.id, VerificationStatus: VerificationPassed, Succeeded: true, FinishedAt: input.at.Add(time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := recordIDs(all), []string{"work-c", "work-b", "work-a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("global history = %v, want %v", got, want)
	}
	conversation, err := store.ListConversation("conversation-one", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := recordIDs(conversation), []string{"work-a", "work-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("conversation history = %v, want %v", got, want)
	}
	if info, err := filepath.Glob(filepath.Join(store.Root(), "*.json")); err != nil || len(info) != 3 {
		t.Fatalf("history files = %v, %v", info, err)
	}
}

func recordIDs(records []Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}
