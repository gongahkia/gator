package workhistory

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func TestRunningWorkLivenessFollowsProcessHeldLease(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	for _, id := range []string{"work-leased", "work-legacy"} {
		if _, err := store.Start(Start{ID: id, Objective: "Prepare a brief", Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: started}); err != nil {
			t.Fatal(err)
		}
	}
	if legacy, err := store.Load("work-legacy"); err != nil || legacy.LiveStatus != "unknown" {
		t.Fatalf("legacy run = %#v, %v", legacy, err)
	}
	lease, err := store.AcquireRun("work-leased")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireRun("work-leased"); err == nil {
		t.Fatal("second process lease acquired the same run")
	}
	if active, err := store.Load("work-leased"); err != nil || active.LiveStatus != "active" {
		t.Fatalf("active run = %#v, %v", active, err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if stopped, err := store.Load("work-leased"); err != nil || stopped.LiveStatus != "stopped" {
		t.Fatalf("stopped run = %#v, %v", stopped, err)
	}
}

func recordIDs(records []Record) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func TestStoreAcceptsLongUTF8WorkObjective(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	objective := strings.Repeat("Compare the selected files and explain the result. ", 5) + strings.Repeat("資料", 8)
	record, err := store.Start(Start{ID: "long-objective", Objective: objective, Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("ordinary long Work request was rejected: %v", err)
	}
	if len(record.ObjectiveSummary) > 160 || !utf8.ValidString(record.ObjectiveSummary) || !strings.HasSuffix(record.ObjectiveSummary, "…") {
		t.Fatalf("invalid bounded summary %q (%d bytes)", record.ObjectiveSummary, len(record.ObjectiveSummary))
	}
}
