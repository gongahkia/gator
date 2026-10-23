package inbox

import (
	"sync"
	"testing"
	"time"
)

func TestReviewedFeedbackSerializesWritersAndPreservesAttention(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.Add(Entry{Kind: "job", Title: "Deadline moved", Status: "completed", OriginConversationID: "chat-one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RateForConversation("chat-other", e.ID, "useful", 0, time.Now()); err == nil {
		t.Fatal("cross-chat rating accepted")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, rating := range []string{"useful", "wrong"} {
		wg.Add(1)
		go func(rating string) {
			defer wg.Done()
			_, err := s.RateForConversation("chat-one", e.ID, rating, 0, time.Now())
			results <- err
		}(rating)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent stale writes: %d succeeded", succeeded)
	}
	current, err := s.Load(e.ID)
	if err != nil || current.RatingVersion != 1 || !current.ReadAt.IsZero() || !current.DeliveredAt.IsZero() || !current.ResolvedAt.IsZero() {
		t.Fatalf("feedback changed attention: %+v %v", current, err)
	}
	cleared, err := s.RateReviewed(e.ID, "clear", current.RatingVersion, time.Now())
	if err != nil || cleared.Rating != "" || cleared.RatingVersion != 2 {
		t.Fatal("clear lost version", cleared, err)
	}
	if _, err := s.RateForConversation("chat-one", e.ID, "noise", 0, time.Now()); err == nil {
		t.Fatal("pre-clear stale rating accepted")
	}
	// A legacy writer must advance the same revision, and receipts must not.
	legacy, err := s.Rate(e.ID, "useful", time.Now())
	if err != nil || legacy.RatingVersion != 3 {
		t.Fatal("legacy rating bypassed revision", err)
	}
	if _, err := s.RateReviewed(e.ID, "wrong", 2, time.Now()); err == nil {
		t.Fatal("stale inbox review accepted")
	}
	receipt, err := s.Receipt("chat-one", e.ID, "resolve", 0, time.Now())
	if err != nil || receipt.RatingVersion != 3 || receipt.Rating != "useful" {
		t.Fatal("receipt lost feedback", err)
	}
}

func TestHealthNoticesCannotDistortAlertRatings(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	goal := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	for _, status := range []string{"completed", "failed", "interrupted"} {
		e, err := s.Add(Entry{Kind: "job", GoalID: goal, Title: "Watch", Status: status, OriginConversationID: "chat"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.RateForConversation("chat", e.ID, "useful", 0, time.Now())
		if (status == "completed") != (err == nil) {
			t.Fatalf("unexpected rating eligibility for %s: %v", status, err)
		}
	}
	summary, err := s.RatingsForGoal(goal)
	if err != nil || summary.Total != 1 || summary.Useful != 1 || summary.HealthNotices != 2 {
		t.Fatalf("health notices counted as alerts: %+v %v", summary, err)
	}
}
