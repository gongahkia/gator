package main

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalWatchDiffNamesAddedUpdatedAndRemovedItems(t *testing.T) {
	policy := "policy"
	prior := jobs.Attempt{Status: "completed", NotificationPolicySHA256: policy, WatchItems: []jobs.WatchItem{
		{Source: "google/gmail_list", ID: "mail-1", SHA256: "old"},
		{Source: "google/calendar_events", ID: "event-1", SHA256: "old"},
	}}
	current := []jobs.WatchItem{
		{Source: "google/gmail_list", ID: "mail-1", SHA256: "new"},
		{Source: "google/gmail_list", ID: "mail-2", SHA256: "new"},
	}
	changes := diffGoalWatchItems([]jobs.Attempt{prior}, current, policy)
	if len(changes) != 3 || changes[0].Kind != "removed" || changes[1].Kind != "updated" || changes[2].Kind != "added" {
		t.Fatalf("watch changes = %#v", changes)
	}
	context := goalWatchChangeContext(changes)
	for _, expected := range []string{`"kind":"removed"`, `"kind":"updated"`, `"kind":"added"`, "does not prove relevance"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("watch change prompt missing %q: %s", expected, context)
		}
	}
}
