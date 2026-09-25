package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestWorkFeedbackCreatesInspectableCandidatesAndExplicitRememberedRules(t *testing.T) {
	state, workID := feedbackWork(t)
	var output bytes.Buffer
	if err := runWorkFeedbackCommand(state, []string{workID, "reject", "The", "format", "was", "wrong."}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Recorded reject feedback") {
		t.Fatalf("reject output = %q", output.String())
	}
	output.Reset()
	if err := runWorkFeedbackCommand(state, []string{workID, "correct", "--type", "preference", "--key", "package-manager", "Use", "pnpm."}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "proposed candidate") {
		t.Fatalf("correct output = %q", output.String())
	}
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].Status != learning.Candidate || records[0].Origin != learning.Inferred {
		t.Fatalf("candidate records = %#v, %v", records, err)
	}
	candidate := records[0]
	if projected, projectErr := store.Projection(learning.Context{Project: feedbackSource(t, state, workID)}); projectErr != nil || len(projected) != 0 {
		t.Fatalf("candidate projected before approval: %#v, %v", projected, projectErr)
	}
	if _, err := store.Enable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	if projected, projectErr := store.Projection(learning.Context{Project: feedbackSource(t, state, workID)}); projectErr != nil || len(projected) != 1 || projected[0].ID != candidate.ID {
		t.Fatalf("approved candidate did not affect future Work: %#v, %v", projected, projectErr)
	}
	if _, err := store.Disable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runWorkFeedbackCommand(state, []string{workID, "remember", "--type", "preference", "--key", "output-format", "Use", "Markdown."}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Remembered this as active learning") {
		t.Fatalf("remember output = %q", output.String())
	}
	records, err = store.List()
	if err != nil || len(records) != 2 || records[1].Origin != learning.UserAuthored || records[1].Status != learning.Active || len(records[1].Provenance.WorkIDs) != 1 || records[1].Provenance.WorkIDs[0] != workID {
		t.Fatalf("remembered learning = %#v, %v", records, err)
	}
	output.Reset()
	if err := runWorkFeedbackCommand(state, []string{workID, "list"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, signal := range []string{"user_rejected", "user_corrected", "user_remembered"} {
		if !strings.Contains(output.String(), signal) {
			t.Fatalf("feedback list omitted %q: %q", signal, output.String())
		}
	}
}

func TestWorkFeedbackDontLearnDoesNotCreateCandidate(t *testing.T) {
	state, workID := feedbackWork(t)
	if err := runWorkFeedbackCommand(state, []string{workID, "dont-learn", "This", "was", "one-off."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("dont-learn created learning: %#v, %v", records, err)
	}
	observations, err := store.ListObservations(workID)
	if err != nil || len(observations) != 1 || observations[0].Signal != learning.UserDeclinedLearning {
		t.Fatalf("dont-learn observation = %#v, %v", observations, err)
	}
}

func feedbackWork(t *testing.T) (string, string) {
	t.Helper()
	state, source := t.TempDir(), t.TempDir()
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := sessions.Create("Feedback Work", source, "snap-feedback", now)
	if err != nil {
		t.Fatal(err)
	}
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	workID := "work-feedback"
	if _, err := history.Start(workhistory.Start{ID: workID, Objective: "Prepare a report", Mode: action.Draft, ExternalActions: action.Forbid, ConversationID: conversation.ID, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish(workID, workhistory.Finish{ConversationID: conversation.ID, RevisionID: workID, SnapshotID: "snap-feedback", Succeeded: true, FinishedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	return state, workID
}

func feedbackSource(t *testing.T, state, workID string) string {
	t.Helper()
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Load(workID)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := sessions.Load(record.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	return conversation.SourcePath
}
