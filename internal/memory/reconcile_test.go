package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func reconciliationInput(keep Record, others ...Record) Reconciliation {
	input := Reconciliation{KeepID: keep.ID, Revisions: map[string]string{keep.ID: keep.UpdatedAt.Format(time.RFC3339Nano)}}
	for _, record := range others {
		input.Revisions[record.ID] = record.UpdatedAt.Format(time.RFC3339Nano)
	}
	return input
}

func reconciliationRecord(t *testing.T, s Store, subject, text, chat, scope string) Record {
	t.Helper()
	record, err := s.RememberChat(ChatInput{Subject: subject, Kind: "preference", Quote: text, ConversationID: chat, RunID: "run-" + chat, GoalID: scope})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestReconciliationBindsGeneralAliasesAndPreservesForgetSources(t *testing.T) {
	state := t.TempDir()
	s, _ := Open(state)
	first := reconciliationRecord(t, s, "communication.detail_level", "I prefer detailed replies.", "work-first", "")
	second := reconciliationRecord(t, s, "answer.verbosity", "Keep my replies concise.", "work-second", "")
	input := reconciliationInput(second, first)
	input.Aliases = []string{"preference.response_length", "novel.communication_setting"}
	kept, err := s.Reconcile(input)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Text != second.Text || kept.Subject != second.Subject || kept.ID != second.ID || kept.ConceptID == "" || !kept.LastConfirmedAt.Equal(second.LastConfirmedAt) {
		t.Fatal("merge rewrote meaning or refreshed confirmation", kept)
	}
	reopened, _ := Open(state)
	list, err := reopened.List("", true)
	if err != nil || len(list) != 1 || list[0].ConceptID != kept.ConceptID {
		t.Fatal(list, err)
	}
	change := ChatInput{Subject: "novel.communication_setting", Kind: "preference", Quote: "I prefer long replies.", ConversationID: "work-third", RunID: "run-third"}
	if _, err := reopened.RememberChat(change); err == nil {
		t.Fatal("registered alias bypassed guarded replacement")
	}
	change.ReplaceID, change.ExpectedUpdated = kept.ID, kept.UpdatedAt.Format(time.RFC3339Nano)
	updated, err := reopened.RememberChat(change)
	if err != nil || updated.ID != kept.ID || updated.Subject != kept.Subject {
		t.Fatal(updated, err)
	}
	if err := reopened.Forget(updated.ID); err != nil {
		t.Fatal(err)
	}
	for _, chat := range []string{"work-first", "work-second", "work-third"} {
		if blocked, err := reopened.HistorySuppressed(chat, ""); err != nil || !blocked {
			t.Fatal("lost source association", chat, err)
		}
	}
	for _, subject := range []string{first.Subject, second.Subject, "preference.length", "novel.communication_setting"} {
		change.Subject, change.ReplaceID, change.ExpectedUpdated = subject, "", ""
		if _, err := reopened.RememberChat(change); err == nil {
			t.Fatal("forgotten alias restored silently", subject)
		}
	}
	data, _ := os.ReadFile(filepath.Join(reopened.root, ".concepts"))
	for _, record := range []Record{first, second} {
		if strings.Contains(string(data), record.Subject) || strings.Contains(string(data), record.Text) {
			t.Fatal("binding registry retained personal plaintext")
		}
	}
	change.Explicit = true
	if _, err := reopened.RememberChat(change); err != nil {
		t.Fatal("explicit restore failed", err)
	}
	change.Explicit, change.GoalID = false, "goal-0123456789abcdef01234567"
	if _, err := reopened.RememberChat(change); err != nil {
		t.Fatal("independent scope blocked", err)
	}
}

func TestReconciliationRejectsStaleIncompleteAndCrossScopeReviews(t *testing.T) {
	s, _ := Open(t.TempDir())
	first := reconciliationRecord(t, s, "preference.first", "First words.", "work-first", "")
	second := reconciliationRecord(t, s, "preference.second", "Second words.", "work-second", "")
	stale := reconciliationInput(second, first)
	changed, err := s.Correct(first.ID, "Changed first words.", first.Source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reconcile(stale); err == nil {
		t.Fatal("stale review accepted")
	}
	if _, err := os.Stat(filepath.Join(s.root, ".concepts")); !os.IsNotExist(err) {
		t.Fatal("rejected review published bindings", err)
	}
	other := reconciliationRecord(t, s, "preference.third", "Third words.", "work-third", "goal-0123456789abcdef01234567")
	if _, err := s.Reconcile(reconciliationInput(second, other)); err == nil {
		t.Fatal("cross-scope merge accepted")
	}
	bound, err := s.Reconcile(reconciliationInput(second, changed))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a process stopping after common identity publication but before
	// consuming an old file. Normal recall must quarantine the duplicates.
	if err := s.write(changed, true); err != nil {
		t.Fatal(err)
	}
	list, err := s.List("", false)
	if err != nil || len(list) != 2 || list[0].Recallable() || list[1].Recallable() {
		t.Fatal("interrupted merge produced authoritative duplicates", list, err)
	}
	incomplete := reconciliationInput(bound)
	incomplete.Aliases = []string{"another.alias"}
	if _, err := s.Reconcile(incomplete); err == nil {
		t.Fatal("unreviewed group member consumed")
	}
	if err := s.Forget(bound.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(changed.ID); !os.IsNotExist(err) {
		t.Fatal("forget left interrupted alias record", err)
	}
}

func TestAliasOnlyBindingPreservesAgeAndCannotLiftSuppression(t *testing.T) {
	s, _ := Open(t.TempDir())
	record := reconciliationRecord(t, s, "custom.property", "I prefer a split keyboard.", "work-first", "")
	input := reconciliationInput(record)
	input.Aliases = []string{"equipment.keyboard"}
	kept, err := s.Reconcile(input)
	if err != nil || !kept.LastConfirmedAt.Equal(record.LastConfirmedAt) {
		t.Fatal(kept, err)
	}
	if err := s.blockSubject("", "other.forgotten"); err != nil {
		t.Fatal(err)
	}
	input = reconciliationInput(kept)
	input.Aliases = []string{"other.forgotten"}
	if _, err := s.Reconcile(input); err == nil {
		t.Fatal("alias binding lifted withdrawal")
	}
	var bindings conceptBindings
	data, _ := os.ReadFile(filepath.Join(s.root, ".concepts"))
	if err := json.Unmarshal(data, &bindings); err != nil {
		t.Fatal(err)
	}
	if bindings.resolve("", record.Subject) != bindings.resolve("", "equipment.keyboard") {
		t.Fatal("alias-only binding lost")
	}
}
