package main

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalWatchRetainsCheckedDecision(t *testing.T) {
	refs := watchReferenceSet{plan: map[string]bool{"goal plan step step-1": true}}
	changes := []jobs.WatchChange{{Source: "google/gmail_list", ID: "mail-1", Kind: "updated"}}
	text := `{"attention":true,"urgent":false,"summary":"A deadline changed.","next_step":"Review the deadline.","rationale":"The watched item changed.","evidence":["goal plan step step-1"],"change_ids":["google/gmail_list mail-1"]}`
	decision := evaluateGoalWatchResult(text, nil, nil, refs, changes)
	if decision.Status != "attention" || decision.NextStep != "Review the deadline." || len(decision.ChangeIDs) != 1 || len(decision.Evidence) != 1 {
		t.Fatalf("checked decision: %#v", decision)
	}
	bad := strings.Replace(text, "mail-1", "invented", 1)
	decision = evaluateGoalWatchResult(bad, nil, nil, refs, changes)
	if decision.Status != "review" || !strings.Contains(decision.Summary, "change ID unverified") {
		t.Fatalf("unverified change was hidden: %#v", decision)
	}
	quiet := evaluateGoalWatchResult(`{"attention":false,"summary":"No material change.","rationale":"Inputs match."}`, nil, nil, refs, changes)
	if quiet.Status != "quiet" {
		t.Fatalf("quiet decision: %#v", quiet)
	}
}
