package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestWorkCLIAndTUIHistoryShareWorkHistoryModel(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := sessions.Create("History", t.TempDir(), "snap-history", time.Now())
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
	var cli bytes.Buffer
	if err := workSessionCommand([]string{"history", conversation.ID}, nil, &cli, nil); err != nil {
		t.Fatal(err)
	}
	tuiText, err := workHistoryText(history, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cli.String() != tuiText {
		t.Fatalf("CLI history = %q, TUI history = %q", cli.String(), tuiText)
	}
}
