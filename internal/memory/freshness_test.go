package memory

import (
	"os"
	"testing"
	"time"
)

func TestChangeableFactAgesWithoutRecallsRenewingIt(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	in := ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-one", RunID: "run-one"}
	record, err := s.RememberChat(in)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := s.RememberChat(ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer short replies.", ConversationID: "work-one", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path(record.ID))
	now = now.Add(89 * 24 * time.Hour)
	if found, err := s.Search("Harbor City", "", 5); err != nil || len(found) != 1 {
		t.Fatal(found, err)
	}
	now = now.Add(24 * time.Hour)
	reopened, _ := Open(root)
	reopened.now = s.now
	if found, err := reopened.Search("Harbor City", "", 5); err != nil || len(found) != 0 {
		t.Fatal("stale fact recalled", found, err)
	}
	stale, err := reopened.Inspect(record.ID)
	if err != nil || stale.RecallIssue == "" || stale.ReviewAfter != now {
		t.Fatal(stale, err)
	}
	if found, err := reopened.Profile(nil); err != nil || len(found) != 1 || found[0].ID != stable.ID {
		t.Fatal("stable preference aged out", found, err)
	}
	after, _ := os.ReadFile(s.path(record.ID))
	if string(before) != string(after) {
		t.Fatal("recall changed evidence")
	}
	if _, err := reopened.RememberChat(in); err != nil {
		t.Fatal(err)
	}
	if stale, _ := reopened.Inspect(record.ID); stale.RecallIssue == "" {
		t.Fatal("replay renewed confirmation")
	}
	in.RunID = "run-two"
	refreshed, err := reopened.RememberChat(in)
	if err != nil || !refreshed.LastConfirmedAt.Equal(now) {
		t.Fatal(refreshed, err)
	}
	if current, _ := reopened.Inspect(record.ID); !current.Recallable() {
		t.Fatal("new statement did not refresh fact", current)
	}
}

func TestExplicitConfirmationRejectsStaleRevisionAndConceptConflicts(t *testing.T) {
	s, _ := Open(t.TempDir())
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	record, err := s.RememberChat(ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-one", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(91 * 24 * time.Hour)
	confirmed, err := s.ConfirmReviewed(record.ID, record.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil || confirmed.Text != record.Text || !confirmed.LastConfirmedAt.Equal(now) || confirmed.Evidence[len(confirmed.Evidence)-1].Basis != "user-confirmed" {
		t.Fatal(confirmed, err)
	}
	if _, err := s.ConfirmReviewed(record.ID, record.UpdatedAt.Format(time.RFC3339Nano)); err == nil {
		t.Fatal("old inspection accepted")
	}
	duplicate, _ := s.Add("fact", "I live elsewhere.", "", "user", time.Time{})
	duplicate.Subject = "profile.home_city"
	if err := s.write(duplicate, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmReviewed(record.ID, confirmed.UpdatedAt.Format(time.RFC3339Nano)); err == nil {
		t.Fatal("confirmation silently resolved collision")
	}
	if err := s.blockSubject("", record.Subject); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfirmReviewed(record.ID, confirmed.UpdatedAt.Format(time.RFC3339Nano)); err == nil {
		t.Fatal("confirmation restored forgotten fact")
	}
}

func TestFreshnessPreservesUnknownLegacyDatesAndDoesNotGuessSubjects(t *testing.T) {
	s, _ := Open(t.TempDir())
	record, _ := s.Add("fact", "I live in Harbor City.", "", "user", time.Time{})
	record.Subject = "profile.city"
	record.LastConfirmedAt, record.LastObservedAt, record.Evidence = time.Time{}, time.Time{}, nil
	if err := s.write(record, false); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.Inspect(record.ID); current.Recallable() || !current.ReviewAfter.IsZero() {
		t.Fatal("invented legacy confirmation", current)
	}
	record.Subject = "unclassified.fact"
	if err := s.write(record, false); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.Inspect(record.ID); !current.Recallable() {
		t.Fatal("guessed volatility from text", current)
	}
	record.Subject = "profile.city"
	record.UpdatedAt = s.clock().Add(48 * time.Hour)
	record.observe("user-confirmed", nil)
	if err := s.write(record, false); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.Inspect(record.ID); current.Recallable() {
		t.Fatal("future confirmation accepted", current)
	}
}
