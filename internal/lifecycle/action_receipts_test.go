package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/workhistory"
)

func TestExternalReceiptsParticipateInBackupDeletionAndUncertainRetention(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now().UTC()
	old := now.Add(-90 * 24 * time.Hour)
	journal, err := actionlog.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := action.NewProposal("action-test", action.ConnectedMutate, "google", "gmail_send", "https://example.test/send", `{"to":"one@example.test"}`, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := journal.Begin("interrupted-chat", "interrupted-run", proposal, old)
	if err != nil {
		t.Fatal(err)
	}
	attempt.Close()
	history, err := workhistory.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := history.Start(workhistory.Start{ID: "interrupted-run", ConversationID: "interrupted-chat", Objective: "Reviewed send", Mode: action.Act, ExternalActions: action.Approve, StartedAt: old}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish("interrupted-run", workhistory.Finish{ConversationID: "interrupted-chat", Succeeded: false, FinishedAt: old.Add(time.Minute), VerificationStatus: workhistory.VerificationUnavailable}); err != nil {
		t.Fatal(err)
	}
	store, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.selectRetention(context.Background(), now, Policy{HistoryDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if strings.Contains(item.Path, "interrupted-run") {
			t.Fatal("age retention deleted unconfirmed external receipt or its run", item.Path)
		}
	}
	backup, err := store.Export(context.Background(), filepath.Join(t.TempDir(), "receipts.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for path := range backup.Files {
		if strings.HasPrefix(path, "gator/external-actions/") && strings.HasSuffix(path, ".json") {
			found = true
		}
		if strings.HasPrefix(path, "gator/external-actions/") && strings.HasSuffix(path, ".lock") {
			t.Fatal("backup retained a live capability lease", path)
		}
	}
	if !found {
		t.Fatal("approved request was absent from backup")
	}
	deletion, err := store.PreviewDelete(context.Background(), "history")
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range deletion.Items {
		if strings.HasPrefix(filepath.ToSlash(item.Path), "external-actions/") && strings.HasSuffix(item.Path, ".json") {
			found = true
		}
	}
	if !found {
		t.Fatal("history deletion omitted private approved previews")
	}
	if _, err := store.Apply(context.Background(), deletion.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := actionlog.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := reopened.List(""); err != nil || len(entries) != 0 {
		t.Fatal("history deletion left private request receipts", entries, err)
	}
}
