package goal

import (
	"strings"
	"testing"
	"time"
)

func TestReviewedGoalStatusRejectsChangedResponsibilitiesAndPersists(t *testing.T) {
	stateDir := t.TempDir()
	store, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Venue plan", "Choose the next venue step", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest, err := record.ReviewDigest()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"", "wrong", strings.ToUpper(digest)} {
		if _, err := store.SetStatusReviewed(record.ID, Paused, expected); err == nil {
			t.Fatal("accepted missing or malformed review")
		}
	}
	if _, err := store.SetContext(record.ID, "The venue changed; keep this correction"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetStatusReviewed(record.ID, Paused, digest); err == nil {
		t.Fatal("accepted stale goal review")
	}
	current, err := store.Load(record.ID)
	if err != nil || current.Status != Active || current.Context != "The venue changed; keep this correction" {
		t.Fatal("stale review changed the goal", current, err)
	}
	digest, _ = current.ReviewDigest()
	paused, err := store.SetStatusReviewed(record.ID, Paused, digest)
	if err != nil || paused.Status != Paused {
		t.Fatal(paused, err)
	}
	if _, err := store.SetStatusReviewed(record.ID, Active, digest); err == nil {
		t.Fatal("reused pre-pause review")
	}
	digest, _ = paused.ReviewDigest()
	if _, err := store.SetStatusReviewed(record.ID, Completed, digest); err == nil {
		t.Fatal("status control completed a goal")
	}
	resumed, err := store.SetStatusReviewed(record.ID, Active, digest)
	if err != nil || resumed.Context != paused.Context || resumed.Status != Active {
		t.Fatal(resumed, err)
	}
	reopened, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.Load(record.ID)
	if err != nil || saved.Status != Active || saved.Context != resumed.Context {
		t.Fatal(saved, err)
	}
	completed, err := store.SetStatus(record.ID, Completed)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ = completed.ReviewDigest()
	if _, err := store.SetStatusReviewed(record.ID, Active, digest); err == nil {
		t.Fatal("Following resumed a completed goal")
	}
}

func TestGoalRecordWritersShareTheReviewLock(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Shared goal", "Original outcome", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.lockRecord(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held {
			unlock()
		}
	}()
	done := make(chan error, 2)
	go func() { _, err := store.SetContext(record.ID, "Concurrent corrected outcome"); done <- err }()
	go func() { _, err := store.SetStatus(record.ID, Paused); done <- err }()
	select {
	case err := <-done:
		t.Fatal("record writer bypassed held review lock", err)
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	held = false
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("record writer did not finish")
		}
	}
	saved, err := store.Load(record.ID)
	if err != nil || saved.Status != Paused || saved.Context != "Concurrent corrected outcome" {
		t.Fatal("concurrent update lost a field", saved, err)
	}
}
