package worksession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workactivity"
)

func TestActivitySidecarPreservesRevisionSchemaAndSurvivesRestore(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	conversation, err := store.Create("Topic", "/source", "snap-test", now)
	if err != nil {
		t.Fatal(err)
	}
	activity := workactivity.New(now)
	activity.Record(agent.Event{Kind: agent.EventText, Text: "Checked the notes", At: now})
	activity.Finish(now.Add(time.Second), "completed", "The answer")
	_, err = store.AddRevision(conversation.ID, Revision{ID: "rev-one", SnapshotID: "snap-test", Objective: "Read the notes", BundlePath: "/bundle", Status: "completed", FinalText: "The answer", Activity: activity, InputNames: []string{"brief.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.revisionsPath(conversation.ID), "rev-one.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["activity"]; exists {
		t.Fatal("optional activity changed the strict revision schema")
	}
	if _, exists := fields["InputNames"]; exists {
		t.Fatal("input filenames changed the strict revision schema")
	}
	restored, err := store.LoadRevision(conversation.ID, "rev-one")
	if err != nil || restored.Activity == nil || restored.Activity.Entries[0].Text != "Checked the notes" {
		t.Fatalf("display activity was lost: %#v, %v", restored, err)
	}
	if len(restored.InputNames) != 1 || restored.InputNames[0] != "brief.txt" {
		t.Fatal("restored attachment names were lost")
	}
	if err := os.WriteFile(store.activityPath(conversation.ID, "rev-one"), []byte("damaged display data"), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err = store.LoadRevision(conversation.ID, "rev-one")
	if err != nil || restored.Activity != nil || restored.FinalText != "The answer" {
		t.Fatal("damaged display data prevented restoring the answer")
	}
	// Optional display storage failure must not change whether work succeeds.
	if err := os.RemoveAll(filepath.Join(store.root, conversation.ID, "activity")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.root, conversation.ID, "activity"), []byte("blocked directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddRevision(conversation.ID, Revision{ID: "rev-two", ParentRevisionID: "rev-one", SnapshotID: "snap-test", Objective: "Follow up", BundlePath: "/bundle/two", Status: "completed", Activity: activity}); err != nil {
		t.Fatal("display storage failure changed the work outcome:", err)
	}
}
