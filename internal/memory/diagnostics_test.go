package memory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCorruptRecordLeavesHealthyRecallAndProfileWithVisibleDiagnostics(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := store.Add("preference", "Prefer concise harbor briefings", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := store.Add("fact", "Synthetic damaged record", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"text":"incomplete`)
	if err := os.WriteFile(store.path(bad.ID), broken, 0600); err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func() ([]Record, error){
		"list":    func() ([]Record, error) { return store.List("", false) },
		"search":  func() ([]Record, error) { return store.Search("harbor", "", 8) },
		"profile": func() ([]Record, error) { return store.Profile(nil) },
	} {
		t.Run(name, func(t *testing.T) {
			records, err := read()
			var issues *ReadIssues
			if len(records) != 1 || records[0].ID != healthy.ID || !errors.As(err, &issues) || issues.Count != 1 || issues.Items[0].File != bad.ID+".json" {
				t.Fatalf("read lost healthy evidence or diagnostics: %#v %v", records, err)
			}
		})
	}
	if _, err := store.RememberChat(ChatInput{Subject: "preference.brief", Kind: "preference", Quote: "Prefer concise harbor briefings", ConversationID: "work-test", RunID: "run-test"}); ReadWarning(err) == "" {
		t.Fatal("save ignored an unreadable potentially conflicting subject", err)
	}
	if _, err := store.Load(bad.ID); err == nil {
		t.Fatal("direct read accepted damaged data")
	}
	retained, err := os.ReadFile(store.path(bad.ID))
	if err != nil || string(retained) != string(broken) {
		t.Fatal("diagnostics changed damaged evidence", err)
	}
	if err := os.RemoveAll(store.root); err != nil {
		t.Fatal(err)
	}
	if records, err := store.Search("harbor", "", 8); err == nil || ReadWarning(err) != "" || len(records) > 0 {
		t.Fatal("inaccessible store became a partial success", err)
	}
}

func TestReadDiagnosticsBoundDetailsWithoutDiscardingTotal(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for range 40 {
		record, err := store.Add("fact", "Synthetic record", "", "fixture", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store.root, record.ID+".json"), []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.List("", true)
	var issues *ReadIssues
	if len(records) != 0 || !errors.As(err, &issues) || issues.Count != 40 || len(issues.Items) != 32 {
		t.Fatalf("unbounded or missing diagnostics: %#v %v", records, err)
	}
}
