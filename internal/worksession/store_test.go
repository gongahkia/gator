package worksession

import (
	"strings"
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

func TestLineageFollowsOnlyTheSelectedBranch(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	conversation, err := store.Create("Quarterly report", "/source", "snap-123", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []Revision{
		{ID: "rev-root", SnapshotID: "snap-123", Objective: "root", BundlePath: "/bundle/root", Status: "completed", CreatedAt: now},
		{ID: "rev-left", ParentRevisionID: "rev-root", SnapshotID: "snap-123", Objective: "left", BundlePath: "/bundle/left", Status: "completed", CreatedAt: now.Add(time.Minute)},
		{ID: "rev-right", ParentRevisionID: "rev-root", SnapshotID: "snap-123", Objective: "right", BundlePath: "/bundle/right", Status: "completed", CreatedAt: now.Add(2 * time.Minute)},
		{ID: "rev-right-child", ParentRevisionID: "rev-right", SnapshotID: "snap-123", Objective: "right child", BundlePath: "/bundle/right-child", Status: "completed", CreatedAt: now.Add(3 * time.Minute)},
	} {
		if _, err := store.AddRevision(conversation.ID, revision); err != nil {
			t.Fatal(err)
		}
	}
	lineage, err := store.Lineage(conversation.ID, "rev-right-child")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, revision := range lineage {
		ids = append(ids, revision.ID)
	}
	if got, want := strings.Join(ids, ","), "rev-root,rev-right,rev-right-child"; got != want {
		t.Fatalf("lineage = %q, want %q", got, want)
	}
}
