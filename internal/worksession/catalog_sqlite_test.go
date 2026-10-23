package worksession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func indexedPage(t *testing.T, s Store, query ConversationQuery) ConversationPage {
	t.Helper()
	page, err := s.PageConversations(query)
	if err != nil || page.IndexFallback {
		t.Fatalf("SQLite query failed: %+v %v", page, err)
	}
	return page
}

func TestSQLiteCatalogReconcilesHistoryWithoutIndexingPrivateReplay(t *testing.T) {
	s, id, dir := requestTestStore(t)
	c, err := s.AddRevision(id, Revision{ID: "rev-index", SnapshotID: "snap-test", Objective: "Find a bird", BundlePath: "/retained/result", Status: "completed", FinalText: "The JADE heron, 保留这个决定, chose a *literal* 100% plan and said \"hello\".", Replay: &ReplayState{Version: Version, Provider: "private-provider-marker", Configuration: json.RawMessage(`{"system":"private-config-marker"}`)}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"jade heron", "保留这个决定", "决定", "*literal*", "100%", "\"hello\""} {
		page := indexedPage(t, s, ConversationQuery{Text: term, Content: true})
		if len(page.Items) != 1 || page.Items[0].Match != "messages" || page.Items[0].Snippet == "" || page.Incomplete {
			t.Fatal(term, page)
		}
	}
	for _, term := range []string{"private-provider-marker", "private-config-marker", `heron OR bird`, `" OR 1=1 --`} {
		if page := indexedPage(t, s, ConversationQuery{Text: term, Content: true}); len(page.Items) != 0 {
			t.Fatal("search interpreted syntax or private state", term, page)
		}
	}
	if page := indexedPage(t, s, ConversationQuery{Text: "jade heron"}); len(page.Items) != 0 {
		t.Fatal("title search inspected messages", page)
	}
	db, close, err := s.openCatalogIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var text string
	if err = db.QueryRow("SELECT body FROM conversations WHERE id=?", id).Scan(&text); err != nil {
		t.Fatal(err)
	}
	close()
	if strings.Contains(text, "private-") || !strings.Contains(text, "JADE") {
		t.Fatal("incorrect indexed projection", text)
	}
	path := filepath.Join(dir, "gator", "cache", catalogIndexName)
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "SQLite format 3\x00") {
		t.Fatal("not a persistent SQLite database", err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatal("index is not private", info.Mode())
		}
	}
	// Editing a canonical revision, including corruption, invalidates the
	// previously indexed excerpt even when conversation metadata is unchanged.
	if err = os.WriteFile(filepath.Join(s.revisionsPath(id), c.HeadRevision+".json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	page := indexedPage(t, s, ConversationQuery{Text: "jade heron", Content: true})
	if len(page.Items) != 0 || !page.Incomplete {
		t.Fatal("stale content survived canonical corruption", page)
	}
	if err = os.RemoveAll(filepath.Join(s.root, id)); err != nil {
		t.Fatal(err)
	}
	if page = indexedPage(t, s, ConversationQuery{Scope: "all", Content: true, Text: "bird"}); len(page.Items) != 0 {
		t.Fatal("deleted chat survived index reconciliation", page)
	}
	db, close, err = s.openCatalogIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM conversation_text").Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted excerpts retained in FTS", count, err)
	}
}

func TestSQLiteCatalogIncludesCurrentAdmissionAndForgetsDiscardedText(t *testing.T) {
	s, id, _ := requestTestStore(t)
	r := retainedRequest(t, "queued-user-marker")
	r.Options = json.RawMessage(`{"private":"credential-option-marker"}`)
	journal, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{r}})
	if err != nil {
		t.Fatal(err)
	}
	if page := indexedPage(t, s, ConversationQuery{Text: r.Text, Content: true}); len(page.Items) != 1 {
		t.Fatal(page)
	}
	if page := indexedPage(t, s, ConversationQuery{Text: "credential-option-marker", Content: true}); len(page.Items) != 0 {
		t.Fatal("admission options searchable", page)
	}
	journal.Requests[0].State = "discarded"
	journal.Requests[0].UpdatedAt = journal.Requests[0].UpdatedAt.Add(time.Second)
	if _, err = s.SaveRequests(id, journal); err != nil {
		t.Fatal(err)
	}
	// Rename the automatic title too, so only the removed message was eligible.
	c, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(c), Action: "rename", Title: "Empty topic"}); err != nil {
		t.Fatal(err)
	}
	if page := indexedPage(t, s, ConversationQuery{Text: r.Text, Content: true}); len(page.Items) != 0 {
		t.Fatal("discarded text remained searchable", page)
	}
}

func TestSQLiteCatalogRepairsOnlyDisposableCorruptionAndFallsBack(t *testing.T) {
	s, id, dir := requestTestStore(t)
	indexedPage(t, s, ConversationQuery{})
	path := filepath.Join(dir, "gator", "cache", catalogIndexName)
	if err := os.WriteFile(path, []byte("broken SQLite index"), 0600); err != nil {
		t.Fatal(err)
	}
	if page := indexedPage(t, s, ConversationQuery{}); len(page.Items) != 1 || page.Items[0].ID != id {
		t.Fatal("cache repair lost retained conversation", page)
	}
	// A future schema is retained for a newer binary, with canonical fallback.
	db, close, err := s.openCatalogIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	close()
	page, err := s.PageConversations(ConversationQuery{})
	if err != nil || !page.IndexFallback || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	if _, err = s.Load(id); err != nil {
		t.Fatal("cache failure damaged original chat", err)
	}
	if err = os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Dir(path), []byte("unavailable cache"), 0600); err != nil {
		t.Fatal(err)
	}
	page, err = s.PageConversations(ConversationQuery{})
	if err != nil || !page.IndexFallback || len(page.Items) != 1 {
		t.Fatal("unavailable cache prevented history browsing", page, err)
	}
}

func TestSQLiteCatalogDoesNotFollowCacheSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs Windows-specific privilege")
	}
	s, _, dir := requestTestStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "gator", "cache")); err != nil {
		t.Fatal(err)
	}
	page, err := s.PageConversations(ConversationQuery{})
	if err != nil || !page.IndexFallback || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("index escaped profile", entries, err)
	}
}

func TestSQLiteCatalogConcurrentReadersAndOrganization(t *testing.T) {
	s, id, _ := requestTestStore(t)
	indexedPage(t, s, ConversationQuery{})
	var wg sync.WaitGroup
	errors := make(chan error, 5)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				page, err := s.PageConversations(ConversationQuery{})
				if err != nil {
					errors <- err
					return
				}
				if len(page.Items) != 1 || page.IndexFallback {
					errors <- fmt.Errorf("unexpected page: %+v", page)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 4; i++ {
			c, err := s.Load(id)
			if err != nil {
				errors <- err
				return
			}
			_, err = s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(c), Action: "rename", Title: fmt.Sprintf("Topic %d", i)})
			if err != nil {
				errors <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if page := indexedPage(t, s, ConversationQuery{}); len(page.Items) != 1 || page.Items[0].Title != "Topic 3" {
		t.Fatal("last canonical organization not indexed", page)
	}
}

func BenchmarkConversationCatalog(b *testing.B) {
	s, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	at := time.Now().UTC()
	// Avoid setup fsync costs in a read benchmark; these are valid retained
	// records with realistic private replay bytes that must never be indexed.
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("work-%04d", i)
		if err = os.MkdirAll(s.revisionsPath(id), 0700); err != nil {
			b.Fatal(err)
		}
		c := Conversation{Version: Version, ID: id, Title: "Planning conversation", SourcePath: "/project", HeadRevision: "rev-first", UpdatedAt: at.Add(time.Duration(i) * time.Second), CreatedAt: at}
		data, _ := json.Marshal(c)
		if err = os.WriteFile(filepath.Join(s.root, id, "conversation.json"), data, 0600); err != nil {
			b.Fatal(err)
		}
		revision := map[string]any{"conversation_id": id, "objective": "plan the week", "final_text": "A retained jade heron decision", "replay": map[string]any{"private": strings.Repeat("x", 8192)}}
		data, _ = json.Marshal(revision)
		if err = os.WriteFile(filepath.Join(s.revisionsPath(id), "rev-first.json"), data, 0600); err != nil {
			b.Fatal(err)
		}
	}
	q := ConversationQuery{Text: "not-in-any-chat", Content: true}
	if page, err := s.PageConversations(q); err != nil || page.IndexFallback || page.Incomplete {
		b.Fatal(page, err)
	}
	b.Run("files", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := s.pageConversationFiles(q); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("sqlite-warm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if page, err := s.PageConversations(q); err != nil || page.IndexFallback {
				b.Fatal(page, err)
			}
		}
	})
}
