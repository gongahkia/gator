package worksession

import (
	"testing"
	"time"
)

func TestRevisionTreeMovesHeadWithoutDeletingBranches(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	conversation, err := store.Create("Quarterly report", "/source", "snap-123", now)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = store.AddRevision(conversation.ID, Revision{ID: "rev-root", SnapshotID: "snap-123", Objective: "draft", BundlePath: "/bundle/root", Status: "completed", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"rev-a", "rev-b"} {
		if _, err := store.AddRevision(conversation.ID, Revision{ID: id, ParentRevisionID: "rev-root", SnapshotID: "snap-123", Objective: id, BundlePath: "/bundle/" + id, Status: "completed", CreatedAt: now.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	children, err := store.Children(conversation.ID, "rev-root")
	if err != nil || len(children) != 2 {
		t.Fatalf("children = %#v, %v", children, err)
	}
	moved, err := store.MoveHead(conversation.ID, "rev-root")
	if err != nil || moved.HeadRevision != "rev-root" {
		t.Fatalf("MoveHead = %#v, %v", moved, err)
	}
	children, _ = store.Children(conversation.ID, "rev-root")
	if len(children) != 2 {
		t.Fatalf("moving head deleted a branch: %#v", children)
	}
}
