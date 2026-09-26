package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/worktui"
)

func TestWorkCLIAndTUIHistoryReadTheSameGlobalWorkRecords(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	conversation, err := sessions.Create("History", project, "snap-history", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	if _, err := history.Start(workhistory.Start{ID: "work-history-ui", Objective: "Inspect retained history", Mode: action.Inspect, ExternalActions: action.Forbid, ConversationID: conversation.ID, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish("work-history-ui", workhistory.Finish{ConversationID: conversation.ID, RevisionID: "work-history-ui", SnapshotID: "snap-history", VerificationStatus: workhistory.VerificationPassed, Succeeded: true, FinishedAt: started.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Start(workhistory.Start{ID: "work-history-global", Objective: "Prepare global history", Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: started.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish("work-history-global", workhistory.Finish{RevisionID: "work-history-global", SnapshotID: "snap-history", VerificationStatus: workhistory.VerificationPassed, Succeeded: true, FinishedAt: started.Add(3 * time.Second)}); err != nil {
		t.Fatal(err)
	}
	var cli bytes.Buffer
	if err := workSessionCommand([]string{"list"}, nil, &cli, nil); err != nil {
		t.Fatal(err)
	}
	deliveries, err := delivery.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	learnings, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	model := worktui.New(worktui.Config{CurrentFolder: project, HistoryStore: &history, DeliveryStore: &deliveries, LearningStore: &learnings, SessionStore: &sessions})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("history")})
	model = updated.(worktui.Model)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("global History unexpectedly scheduled external work")
	}
	view := updated.(worktui.Model).View()
	for _, objective := range []string{"Inspect retained history", "Prepare global history"} {
		if !strings.Contains(cli.String(), objective) || !strings.Contains(view, objective) {
			t.Fatalf("CLI/TUI history differs for %q:\nCLI=%s\nTUI=%s", objective, cli.String(), view)
		}
	}
}

func TestWorkHistoryUsesProductTermsForFeedbackAndDerivedLearnings(t *testing.T) {
	state := t.TempDir()
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	if _, err := history.Start(workhistory.Start{ID: "work-history-summary", Objective: "Prepare the weekly report", Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish("work-history-summary", workhistory.Finish{ArtifactStatus: "completed", VerificationStatus: workhistory.VerificationPassed, Succeeded: true, FinishedAt: started.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	learnings, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := learnings.RecordObservation(learning.ObservationInput{ID: "observation-history", Signal: learning.UserAccepted, WorkID: "work-history-summary"}); err != nil {
		t.Fatal(err)
	}
	if _, err := learnings.Create(learning.Create{ID: "learning-history", Type: learning.Preference, Key: "reports", Content: "Use concise weekly sections.", Scope: learning.Scope{Kind: learning.Global}, Origin: learning.Inferred, Confidence: 90, Provenance: learning.Provenance{WorkIDs: []string{"work-history-summary"}}}); err != nil {
		t.Fatal(err)
	}
	records, err := history.List(0)
	if err != nil {
		t.Fatal(err)
	}
	text, err := formatWorkHistory(state, records)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Prepare the weekly report", "Status: completed · verification passed · completed", "Delivery: not delivered", "Artifacts: none", "Feedback: accepted", "Learnings: candidate “Use concise weekly sections.”"} {
		if !strings.Contains(text, want) {
			t.Fatalf("history omitted %q: %s", want, text)
		}
	}
}
