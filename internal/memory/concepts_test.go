package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKnownAliasesShareConflictAndForgetIdentity(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	in := ChatInput{Subject: "preference.length", Kind: "preference", Quote: "I prefer short replies.", ConversationID: "work-one", RunID: "run-one"}
	first, err := s.RememberChat(in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Subject != "preference.response_length" {
		t.Fatal("not canonical", first.Subject)
	}
	in.Subject = "preference.reply_length"
	in.Quote = "I prefer detailed replies."
	if _, err := s.RememberChat(in); err == nil {
		t.Fatal("alias bypassed conflict")
	}
	in.ReplaceID = first.ID
	in.ExpectedUpdated = first.UpdatedAt.Format(time.RFC3339Nano)
	updated, err := s.RememberChat(in)
	if err != nil || updated.ID != first.ID {
		t.Fatal(updated, err)
	}
	if err := s.Forget(updated.ID); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(dir)
	in.ReplaceID = ""
	in.ExpectedUpdated = ""
	for _, subject := range []string{"preference.length", "preference.reply_length", "preference.response_length"} {
		in.Subject = subject
		if _, err := reopened.RememberChat(in); err == nil {
			t.Fatal("alias resurfaced", subject)
		}
	}
	in.Explicit = true
	if _, err := reopened.RememberChat(in); err != nil {
		t.Fatal("explicit restoration failed", err)
	}
}

func TestLegacyAliasConflictIsVisibleExcludedAndRemovedTogether(t *testing.T) {
	s, _ := Open(t.TempDir())
	first, err := s.Add("preference", "I prefer short replies.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Add("preference", "I prefer detailed replies.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	first.Subject = "preference.length"
	second.Subject = "preference.response_length"
	if err := s.write(first, false); err != nil {
		t.Fatal(err)
	}
	if err := s.write(second, false); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path(first.ID))
	list, err := s.List("", false)
	if err != nil || len(list) != 2 || list[0].RecallIssue == "" || list[1].Recallable() {
		t.Fatal("hidden legacy conflict", list, err)
	}
	if selected, err := s.Search("replies", "", 8); err != nil || len(selected) != 0 {
		t.Fatal("conflict recalled", selected, err)
	}
	if selected, err := s.Profile(nil); err != nil || len(selected) != 0 {
		t.Fatal("conflict in profile", selected, err)
	}
	if text, _ := RenderSelection(list); text != "" {
		t.Fatal("conflict rendered", text)
	}
	if inspected, err := s.Inspect(first.ID); err != nil || inspected.RecallIssue == "" {
		t.Fatal("direct inspection hid conflict", inspected, err)
	}
	after, _ := os.ReadFile(s.path(first.ID))
	if string(before) != string(after) {
		t.Fatal("read rewrote legacy evidence")
	}
	in := ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: first.Text, ConversationID: "work-one", RunID: "run-one", Explicit: true}
	if _, err := s.RememberChat(in); err == nil {
		t.Fatal("silently selected one legacy record")
	}
	if err := s.Forget(first.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if _, err := s.Load(id); !os.IsNotExist(err) {
			t.Fatal("known alias remained", id, err)
		}
	}
}

func TestLegacyAliasTombstoneSurvivesCanonicalization(t *testing.T) {
	s, _ := Open(t.TempDir())
	path := s.subjectPath("", "profile.home_city")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("blocked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	in := ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-one", RunID: "run-one"}
	if _, err := s.RememberChat(in); err == nil {
		t.Fatal("legacy tombstone ignored")
	}
	in.Explicit = true
	if _, err := s.RememberChat(in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("legacy marker not restored", err)
	}
	in.GoalID = "goal-0123456789abcdef01234567"
	in.Explicit = false
	if _, err := s.RememberChat(in); err != nil {
		t.Fatal("independent goal blocked", err)
	}
}
