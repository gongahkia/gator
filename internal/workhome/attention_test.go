package workhome

import "testing"

func TestNextPrioritizesItemsRequiringReview(t *testing.T) {
	actions := (Attention{PendingSuggestions: 2, UnreadResults: 1, ActiveRuns: 1, ActiveGoals: 1, RecentWork: true}).Next()
	want := []string{"/suggestions", "/inbox", "/activity", "/goals", "/history"}
	if len(actions) != len(want) {
		t.Fatalf("actions = %#v", actions)
	}
	for i, command := range want {
		if actions[i].Command != command {
			t.Fatalf("action %d = %#v, want %s", i, actions[i], command)
		}
	}
	if actions := (Attention{}).Next(); len(actions) != 1 || actions[0].Command != "type below" {
		t.Fatalf("empty home = %#v", actions)
	}
}
