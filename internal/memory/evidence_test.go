package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestReconfirmationPersistsEvidenceWithoutRefreshingOnRecall(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	in := ChatInput{Subject: "preference.length", Kind: "preference", Quote: "  I prefer short replies.  ", ConversationID: "work-one", RunID: "run-one"}
	first, err := s.RememberChat(in)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	replay, err := s.RememberChat(in)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay changed evidence: %#v %v", replay, err)
	}
	in.ConversationID, in.RunID = "work-two", "run-two"
	second, err := s.RememberChat(in)
	if err != nil || second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) || !second.LastConfirmedAt.Equal(now) || len(second.Evidence) != 2 {
		t.Fatalf("confirmation: %#v %v", second, err)
	}
	if second.Evidence[0].RunID != "run-one" || second.Evidence[1].RunID != "run-two" {
		t.Fatal("lost provenance")
	}
	// A wall-clock adjustment cannot invalidate optimistic revision ordering.
	now = first.CreatedAt.Add(-time.Hour)
	in.RunID = "run-three"
	third, err := s.RememberChat(in)
	if err != nil || !third.UpdatedAt.After(second.UpdatedAt) {
		t.Fatal("clock regression", err)
	}
	before, _ := os.ReadFile(s.path(first.ID))
	now = first.CreatedAt.AddDate(1, 0, 0)
	if _, err := s.Search("short replies", "", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Profile(nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path(first.ID))
	if string(before) != string(after) {
		t.Fatal("recall refreshed evidence")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(first.ID)
	if err != nil || !reflect.DeepEqual(loaded.Evidence, third.Evidence) {
		t.Fatal("evidence did not survive restart", err)
	}
	// An older source replay remains idempotent while present in the bounded log.
	in.RunID, in.ConversationID = "run-one", "work-one"
	replay, err = s.RememberChat(in)
	if err != nil || !replay.UpdatedAt.Equal(third.UpdatedAt) {
		t.Fatal("older observation refreshed evidence", err)
	}
}

func TestEvidenceBoundedAndCorrectionDoesNotCopyPriorText(t *testing.T) {
	s, _ := Open(t.TempDir())
	in := ChatInput{Subject: "profile.language", Kind: "fact", Quote: "I speak English.", ConversationID: "work-one", RunID: "run-0"}
	var record Record
	var err error
	for i := 0; i < 20; i++ {
		in.RunID = fmt.Sprintf("run-%d", i)
		record, err = s.RememberChat(in)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(record.Evidence) != maxMemoryEvidence || record.Evidence[0].RunID != "run-12" {
		t.Fatal("unbounded or unordered provenance")
	}
	record, err = s.Correct(record.ID, "I speak French.", "user")
	if err != nil {
		t.Fatal(err)
	}
	last := record.Evidence[len(record.Evidence)-1]
	if last.Basis != "user-corrected" || last.RunID != "" || last.TextSHA256 != textDigest(record.Text) {
		t.Fatal("misattributed correction", last)
	}
	data, _ := os.ReadFile(s.path(record.ID))
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if record.Origin.Quote != "" {
		t.Fatal("retained superseded text")
	}
}

func TestLegacyEvidenceIsUnknownUntilNewConfirmation(t *testing.T) {
	s, _ := Open(t.TempDir())
	record, err := s.Add("preference", "I prefer short replies.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	record.Evidence = nil
	record.LastObservedAt, record.LastConfirmedAt = time.Time{}, time.Time{}
	if err := s.write(record, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(record.ID)
	if err != nil || !loaded.LastConfirmedAt.IsZero() {
		t.Fatal("invented legacy evidence", err)
	}
	corrected, err := s.Correct(record.ID, "I prefer detailed replies.", "user")
	if err != nil || len(corrected.Evidence) != 1 || corrected.LastConfirmedAt.IsZero() {
		t.Fatal("missing new evidence", err)
	}
	invalid := corrected
	invalid.LastConfirmedAt = corrected.UpdatedAt.Add(time.Hour)
	if err := validate(invalid); err == nil {
		t.Fatal("accepted future confirmation")
	}
}
