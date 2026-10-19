package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestSourceRelinkRepairsMissingProjectsAndPausesAffectedChecks(t *testing.T) {
	s, _, source, outcome, goalID := restoreFixture(t)
	moved := filepath.Join(t.TempDir(), "moved-project")
	if err := os.Rename(source, moved); err != nil {
		t.Fatal(err)
	}
	moved, _ = filepath.EvalSymlinks(moved)
	plan, err := s.PreviewRelink(context.Background(), source, moved)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Edits) < 4 {
		t.Fatal("relink omitted related source metadata")
	}
	goals, _ := goal.Open(s.stateDir)
	g, _ := goals.Load(goalID)
	if g.SourcePath == moved {
		t.Fatal("relink preview changed the goal")
	}
	count, err := s.Relink(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != len(plan.Edits) {
		t.Fatal("not all references were updated")
	}
	g, err = goals.Load(goalID)
	if err != nil || g.SourcePath != moved {
		t.Fatal("goal still refers to missing folder", err)
	}
	sessions, _ := worksession.Open(s.stateDir)
	conversation, err := sessions.Load(outcome.ConversationID)
	if err != nil || conversation.SourcePath != moved || !conversation.SourceRefreshRequired {
		t.Fatal("conversation not ready for a fresh capture", err)
	}
	store, _ := jobs.Open(s.stateDir)
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 || definitions[0].Enabled || definitions[0].SourcePath != moved {
		t.Fatal("check was not safely relinked", err)
	}
	if _, err = os.Stat(filepath.Join(moved, "notes.md")); err != nil {
		t.Fatal("source content was changed")
	}
	current, err := s.SourceFolders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range current {
		if folder.Destination != moved || !folder.Available {
			t.Fatalf("stale source status: %#v", folder)
		}
	}
}
func TestRelinkRecoversAnInterruptedTransactionBeforeOpeningProfile(t *testing.T) {
	s, _, source, _, goalID := restoreFixture(t)
	destination := t.TempDir()
	plan, err := s.PreviewRelink(context.Background(), source, destination)
	if err != nil {
		t.Fatal(err)
	}
	journal := relinkJournal{Plan: plan}
	if err = privateJSON(filepath.Join(s.control, "relink-pending.json"), journal); err != nil {
		t.Fatal(err)
	}
	// Emulate a process dying after publishing only the first updated file.
	first := plan.Edits[0]
	if err = os.WriteFile(filepath.Join(s.root, first.Path), first.After, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range plan.Edits {
		data, err := os.ReadFile(filepath.Join(reopened.root, edit.Path))
		if err != nil || string(data) != string(edit.Before) {
			t.Fatal("recovery left mixed source references", err)
		}
	}
	if _, err = os.Stat(filepath.Join(s.control, "relink-pending.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("recovery journal remains")
	}
	goals, _ := goal.Open(s.stateDir)
	if _, err = goals.Load(goalID); err != nil {
		t.Fatal(err)
	}
}
func TestRelinkRejectsStaleReviewActiveWorkAndConflictingRecovery(t *testing.T) {
	s, _, source, _, _ := restoreFixture(t)
	plan, err := s.PreviewRelink(context.Background(), source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	release, err := state.Guard(s.stateDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Relink(context.Background(), plan.ID); !errors.Is(err, state.ErrBusy) {
		t.Fatal("relink raced live data", err)
	}
	release()
	path := filepath.Join(s.root, plan.Edits[0].Path)
	if err = os.WriteFile(path, append(plan.Edits[0].Before, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Relink(context.Background(), plan.ID); err == nil {
		t.Fatal("stale metadata review accepted")
	}
	if err = privateJSON(filepath.Join(s.control, "relink-pending.json"), relinkJournal{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(s.stateDir); !errors.Is(err, ErrRecovery) {
		t.Fatal("conflicting recovery was not blocked", err)
	}
	// Committed journals are cleaned without rolling back the finished mapping.
	if err = privateJSON(filepath.Join(s.control, "relink-pending.json"), relinkJournal{Plan: plan, Committed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(s.stateDir); err != nil {
		t.Fatal(err)
	}
}
func TestRelinkReviewExpires(t *testing.T) {
	s, _, source, _, _ := restoreFixture(t)
	plan, err := s.PreviewRelink(context.Background(), source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Expiry is part of the retained exact review rather than a mutable CLI flag.
	plan.CreatedAt = time.Now().Add(-2 * time.Hour)
	plan.ID = relinkID(plan)
	if err = privateJSON(filepath.Join(s.control, "plans", "relink-"+plan.ID+".json"), plan); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Relink(context.Background(), plan.ID); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal("expired source review accepted", err)
	}
}
