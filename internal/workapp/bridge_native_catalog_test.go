package workapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func nativeCatalogFixture(t *testing.T, count int) (Model, worksession.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := worksession.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("work-native-%04d", i)
		path := filepath.Join(dir, "gator", "conversations", id)
		if err = os.MkdirAll(filepath.Join(path, "revisions"), 0700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(worksession.Conversation{Version: 2, ID: id, Title: fmt.Sprintf("Topic %04d", i), SourcePath: "/workspace", SnapshotID: "snap-fixture", CreatedAt: at, UpdatedAt: at.Add(time.Duration(i) * time.Minute)})
		if err = os.WriteFile(filepath.Join(path, "conversation.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{BridgeMode: true, ManagedSessions: true, SessionStore: &s, QueryConversations: s.PageConversationsContext, OrganizeConversation: s.OrganizeConversation})
	m.conversation = "work-native-0000"
	m.title = "Topic 0000"
	m.input = "untouched input"
	m.running = true
	return m, s, dir
}
func nativeCatalogQuery(t *testing.T, m Model, id, text, scope, after string) Model {
	t.Helper()
	next, cmd := m.ApplyNativeCatalog(NativeCatalogAction{RequestID: id, Action: "query", Text: text, Scope: scope, After: after})
	if cmd == nil {
		t.Fatal("query did not start", next.(Model).nativeCatalogQueryResult)
	}
	next, _ = next.Update(cmd())
	return next.(Model)
}
func nativeCatalogAction(t *testing.T, m Model, a NativeCatalogAction) Model {
	t.Helper()
	next, cmd := m.ApplyNativeCatalog(a)
	if cmd != nil {
		next, _ = next.Update(cmd())
	}
	return next.(Model)
}

func TestNativeCatalogPagesContentScopesAndNonRecentOpenWithoutEnteringLauncher(t *testing.T) {
	m, s, _ := nativeCatalogFixture(t, 65)
	_, err := s.AddRevision("work-native-0000", worksession.Revision{ID: "rev-fixture", SnapshotID: "snap-fixture", Objective: "public objective", FinalText: "a jade decision", Status: "completed", BundlePath: "/fixture", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogQuery(t, m, "page-first", "", "active", "")
	first := m.nativeCatalogQueryResult
	if len(first.Page.Items) != 50 || first.Page.Next == "" || m.launcher || !m.running || m.input != "untouched input" || m.conversation != "work-native-0000" {
		t.Fatal("query altered the working owner", first)
	}
	m = nativeCatalogQuery(t, m, "page-second", "", "active", first.Page.Next)
	if len(m.nativeCatalogQueryResult.Page.Items) != 15 {
		t.Fatal("pagination lost records")
	}
	m = nativeCatalogQuery(t, m, "body-search", "jade decision", "all", "")
	result := m.nativeCatalogQueryResult
	item := result.Page.Items[0]
	if len(result.Page.Items) != 1 || !strings.Contains(item.Snippet, "jade decision") {
		t.Fatal("not content search", result)
	}
	// This record is older than the 24-row recents boundary.
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "open-older", Action: "open", PageID: result.PageID, ID: item.ID, Version: item.Version})
	if m.nativeCatalogActionResult.Error != "" || m.nativeCatalogActionResult.Item.ID != item.ID || m.sessionRequest != nil || !m.running || m.conversation != "work-native-0000" {
		t.Fatal("open must return validated evidence without switching/admitting", m.nativeCatalogActionResult)
	}
	c, err := s.OrganizeConversation(worksession.ConversationChange{ID: item.ID, Version: item.Version, Action: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogQuery(t, m, "archived-search", "jade decision", "archived", "")
	archived := m.nativeCatalogQueryResult
	if len(archived.Page.Items) != 1 || !archived.Page.Items[0].Archived {
		t.Fatal(archived)
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "open-archived", Action: "open", PageID: archived.PageID, ID: c.ID, Version: worksession.ConversationDisplayVersion(c)})
	if m.nativeCatalogActionResult.Error != "" {
		t.Fatal(m.nativeCatalogActionResult)
	}
	c, err = s.Load(c.ID)
	if err != nil || !c.Archived {
		t.Fatal("opening restored archived state", c, err)
	}
	m = nativeCatalogQuery(t, m, "active-again", "jade decision", "active", "")
	if len(m.nativeCatalogQueryResult.Page.Items) != 0 {
		t.Fatal("scope leaked archived record")
	}
}

func TestNativeCatalogRenameCASReviewAndDuplicateSuppression(t *testing.T) {
	m, s, _ := nativeCatalogFixture(t, 3)
	m = nativeCatalogQuery(t, m, "initial-query", "", "all", "")
	item := m.nativeCatalogQueryResult.Page.Items[0]
	pageID := m.nativeCatalogQueryResult.PageID
	if _, err := s.OrganizeConversation(worksession.ConversationChange{ID: item.ID, Version: item.Version, Action: "rename", Title: "Terminal changed it"}); err != nil {
		t.Fatal(err)
	}
	request := NativeCatalogAction{RequestID: "rename-stale", Action: "rename", PageID: pageID, ID: item.ID, Version: item.Version, Title: "Native attempted title"}
	m = nativeCatalogAction(t, m, request)
	if !m.nativeCatalogActionResult.Conflict || m.nativeCatalogActionResult.Error == "" {
		t.Fatal("stale version accepted")
	}
	c, _ := s.Load(item.ID)
	if c.Title != "Terminal changed it" {
		t.Fatal("concurrent title overwritten")
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "inspect-changed", Action: "inspect", PageID: pageID, ID: item.ID, Version: item.Version})
	checked := m.nativeCatalogActionResult
	if checked.Item.Title != "Terminal changed it" || checked.Item.Version == item.Version {
		t.Fatal("canonical review missing", checked)
	}
	request = NativeCatalogAction{RequestID: "rename-fresh", Action: "rename", PageID: checked.PageID, ID: item.ID, Version: checked.Item.Version, Title: " Native confirmed title "}
	next, cmd := m.ApplyNativeCatalog(request)
	m = next.(Model)
	next, duplicate := m.ApplyNativeCatalog(request)
	m = next.(Model)
	if duplicate != nil || cmd == nil {
		t.Fatal("pending duplicate reexecuted")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.nativeCatalogActionResult.Item.Title != "Native confirmed title" {
		t.Fatal(m.nativeCatalogActionResult)
	}
	c, _ = s.Load(item.ID)
	revision := c.MetadataRevision
	next, duplicate = m.ApplyNativeCatalog(request)
	m = next.(Model)
	c, _ = s.Load(item.ID)
	if duplicate != nil || c.MetadataRevision != revision {
		t.Fatal("confirmed duplicate reexecuted")
	}
	request.RequestID = "stale-page-again"
	m = nativeCatalogAction(t, m, request)
	if m.nativeCatalogActionResult.Error == "" {
		t.Fatal("old page identity survived rename")
	}
}

func TestNativeCatalogDestinationVerificationRefreshesACachedCanonicalTitle(t *testing.T) {
	m, s, _ := nativeCatalogFixture(t, 1)
	c, err := s.Load(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.OrganizeConversation(worksession.ConversationChange{ID: c.ID, Version: worksession.ConversationDisplayVersion(c), Action: "rename", Title: "Changed by another client"})
	if err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "verify-renamed-cache", Action: "verify", ID: c.ID, Version: worksession.ConversationDisplayVersion(c)})
	if m.nativeCatalogActionResult.Error != "" || m.title != c.Title || !m.running {
		t.Fatal("canonical selection retained a stale title", m.title, m.nativeCatalogActionResult)
	}
}

func TestNativeCatalogSupersessionCancelAndResultIsolation(t *testing.T) {
	m, _, _ := nativeCatalogFixture(t, 1)
	var contexts []context.Context
	m.config.QueryConversations = func(ctx context.Context, q worksession.ConversationQuery) (worksession.ConversationPage, error) {
		contexts = append(contexts, ctx)
		return worksession.ConversationPage{Items: []worksession.ConversationItem{}}, nil
	}
	next, old := m.ApplyNativeCatalog(NativeCatalogAction{RequestID: "old-query", Action: "query", Scope: "active"})
	m = next.(Model)
	oldResult := old()
	reused, duplicate := m.ApplyNativeCatalog(NativeCatalogAction{RequestID: "old-query", Action: "query", Scope: "all"})
	if duplicate != nil || reused.(Model).nativeCatalogQueryResult.Error == "" {
		t.Fatal("query identity reused for changed scope")
	}
	next, newer := m.ApplyNativeCatalog(NativeCatalogAction{RequestID: "new-query", Action: "query", Scope: "archived"})
	m = next.(Model)
	if contexts[0].Err() == nil {
		t.Fatal("superseded Go read not cancelled")
	}
	next, _ = m.Update(oldResult)
	m = next.(Model)
	if m.nativeCatalogQueryResult != nil {
		t.Fatal("stale result published")
	}
	next, _ = m.Update(newer())
	m = next.(Model)
	if m.nativeCatalogQueryResult.RequestID != "new-query" {
		t.Fatal("new query lost")
	}
	next, pending := m.ApplyNativeCatalog(NativeCatalogAction{RequestID: "cancel-query", Action: "query", Scope: "all"})
	m = next.(Model)
	pendingResult := pending()
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "cancel-old-only", Action: "cancel", CancelID: "new-query"})
	if contexts[2].Err() != nil {
		t.Fatal("stale cancellation cancelled newer read")
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "cancel-current", Action: "cancel", CancelID: "cancel-query"})
	if contexts[2].Err() == nil {
		t.Fatal("explicit cancellation failed")
	}
	next, _ = m.Update(pendingResult)
	m = next.(Model)
	if m.nativeCatalogQueryResult != nil && m.nativeCatalogQueryResult.RequestID == "cancel-query" {
		t.Fatal("cancelled result appeared")
	}
}

func TestNativeCatalogMissingChangedAndFallbackRecordsRemainExplicit(t *testing.T) {
	m, s, dir := nativeCatalogFixture(t, 3)
	cache := filepath.Join(dir, "gator", "cache")
	if err := os.WriteFile(cache, []byte("fixture cache unavailable"), 0600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "gator", "conversations", "work-broken")
	if err := os.MkdirAll(broken, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "conversation.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogQuery(t, m, "fallback-query", "", "all", "")
	page := m.nativeCatalogQueryResult
	if !page.Page.IndexFallback || page.Page.Skipped != 1 || len(page.Page.Items) != 3 {
		t.Fatal("coverage evidence lost", page)
	}
	item := page.Page.Items[0]
	if _, err := s.OrganizeConversation(worksession.ConversationChange{ID: item.ID, Version: item.Version, Action: "rename", Title: "Changed since page"}); err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "changed-open", Action: "open", PageID: page.PageID, ID: item.ID, Version: item.Version})
	if m.nativeCatalogActionResult.Error == "" {
		t.Fatal("changed selection accepted")
	}
	item = page.Page.Items[1]
	if err := os.RemoveAll(filepath.Join(dir, "gator", "conversations", item.ID)); err != nil {
		t.Fatal(err)
	}
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "removed-open", Action: "open", PageID: page.PageID, ID: item.ID, Version: item.Version})
	if m.nativeCatalogActionResult.Error == "" {
		t.Fatal("removed selection accepted")
	}
	// The destination verifies itself, even after origin-side validation succeeded.
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "wrong-owner", Action: "verify", ID: item.ID, Version: item.Version})
	if m.nativeCatalogActionResult.Error == "" {
		t.Fatal("destination identity was not bound")
	}
	c, err := s.Load(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	m.conversationLoadIssue = "unavailable retained lineage"
	m = nativeCatalogAction(t, m, NativeCatalogAction{RequestID: "unavailable-owner", Action: "verify", ID: c.ID, Version: worksession.ConversationDisplayVersion(c)})
	if !strings.Contains(m.nativeCatalogActionResult.Error, "could not be loaded") {
		t.Fatal("unavailable destination was accepted")
	}
}
