package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestRestoreRetainsChatCreationReceiptWithoutCreatingAnotherChat(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	sessions, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := sessions.CreateChatOnce(source, "retained-phone-request", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "chat-backup.tar.gz")
	manifest, err := store.Export(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for path := range manifest.Files {
		if strings.HasPrefix(path, "gator/conversations/.creations/") && strings.HasSuffix(path, ".json") {
			found = true
		}
	}
	if !found {
		t.Fatal("backup omitted the creation receipt")
	}
	plan, err := store.PreviewRestore(context.Background(), archive, filepath.Join(t.TempDir(), "restored"), nil)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Restore(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err = worksession.Open(restored.Plan.Target)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := sessions.CreateChatOnce(source, "retained-phone-request", time.Now())
	if err != nil || retried.ID != chat.ID {
		t.Fatalf("restored retry: %+v, %v", retried, err)
	}
	if chats, err := sessions.List(100); err != nil || len(chats) != 1 {
		t.Fatalf("duplicate after restore: %+v, %v", chats, err)
	}
}
