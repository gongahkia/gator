package cmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/session"
)

func TestSessionListAndShow(t *testing.T) {
	cwd := t.TempDir()
	chdir(t, cwd)
	manifest, err := session.NewManifest("session-cmd-test", cwd, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Create(cwd, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(session.Event{Type: "session.started"}); err != nil {
		t.Fatal(err)
	}
	list := executeRoot(t, []string{"session", "list"}, "")
	if !strings.Contains(list, manifest.ID) {
		t.Fatalf("list = %q", list)
	}
	show := executeRoot(t, []string{"session", "show", manifest.ID, "--json"}, "")
	if !strings.Contains(show, `"session.started"`) {
		t.Fatalf("show = %q", show)
	}
	if _, err := os.Stat(store.Dir()); err != nil {
		t.Fatal(err)
	}
}
