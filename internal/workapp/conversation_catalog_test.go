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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

func catalogAction(t *testing.T, m Model, action, text string, row *int) Model {
	t.Helper()
	next, cmd := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: action, Text: text, Row: row}, tea.KeyMsg{})
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("catalog action %q did not load", action)
	}
	next, _ = m.Update(cmd())
	return next.(Model)
}

func TestDesktopCatalogPagesSearchesAndOrganizesTheReviewedChat(t *testing.T) {
	dir := t.TempDir()
	s, err := worksession.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	for i := 0; i < 65; i++ {
		id := fmt.Sprintf("work-catalog-%02d", i)
		root := filepath.Join(dir, "gator", "conversations", id)
		if err = os.MkdirAll(filepath.Join(root, "revisions"), 0700); err != nil {
			t.Fatal(err)
		}
		c := worksession.Conversation{Version: 2, ID: id, Title: fmt.Sprintf("Topic %d", i), SourcePath: "/workspace", SnapshotID: "snap-first", CreatedAt: at, UpdatedAt: at.Add(time.Duration(i) * time.Minute)}
		data, _ := json.Marshal(c)
		if err = os.WriteFile(filepath.Join(root, "conversation.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.AddRevision("work-catalog-00", worksession.Revision{ID: "rev-body", SnapshotID: "snap-first", Objective: "A useful choice", FinalText: "Remember the jade decision", Status: "completed", BundlePath: "/result", CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	changed := 0
	m := New(Config{BridgeMode: true, ManagedSessions: true, QueryConversations: s.PageConversationsContext, OrganizeConversation: func(change worksession.ConversationChange) (worksession.Conversation, error) {
		changed++
		return s.OrganizeConversation(change)
	}})
	next, cmd := m.NavigateBridge("/conversations")
	m = next.(Model)
	if cmd == nil || !m.BridgeState().Panel.Catalog.Loading {
		t.Fatal("query blocked presentation instead of loading asynchronously")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	p := m.BridgeState().Panel
	if len(p.Rows) != 50 || !p.Catalog.HasNext || p.Rows[0].Label != "Topic 64" {
		t.Fatal(p)
	}
	m = catalogAction(t, m, "next", "", nil)
	p = m.BridgeState().Panel
	if p.Catalog.Page != 2 || !p.Catalog.HasPrevious || len(p.Rows) != 15 {
		t.Fatal(p)
	}
	m = catalogAction(t, m, "previous", "", nil)
	if p = m.BridgeState().Panel; p.Catalog.Page != 1 || p.Rows[0].Label != "Topic 64" {
		t.Fatal(p)
	}
	m = catalogAction(t, m, "query", "jade decision", nil)
	p = m.BridgeState().Panel
	if len(p.Rows) != 1 || p.Rows[0].Label != "Topic 0" || !strings.Contains(p.Rows[0].Detail, "jade decision") || p.Catalog.Page != 1 {
		t.Fatal("body search was confined to first page", p)
	}
	stale := p.ID
	row := 0
	m = catalogAction(t, m, "rename", "A chosen title", &row)
	if changed != 1 {
		t.Fatal(changed)
	}
	next, cmd = m.ApplyBridgePanel(BridgePanelAction{ID: stale, Action: "archive", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if cmd != nil || changed != 1 || !strings.Contains(m.bridgeNotice, "view changed") {
		t.Fatal("stale row organized a different/current view")
	}
	m = catalogAction(t, m, "pin", "", &row)
	if !m.BridgeState().Panel.Rows[0].Pinned {
		t.Fatal("pin missing")
	}
	m = catalogAction(t, m, "archive", "", &row)
	if len(m.BridgeState().Panel.Rows) != 0 {
		t.Fatal("archive remained active")
	}
	m = catalogAction(t, m, "filter", "archived", nil)
	p = m.BridgeState().Panel
	if len(p.Rows) != 1 || !p.Rows[0].Archived || !p.Rows[0].Pinned {
		t.Fatal("archived chat lost", p)
	}
	m = catalogAction(t, m, "restore", "", &row)
	if len(m.BridgeState().Panel.Rows) != 0 {
		t.Fatal("restored chat remained archived")
	}
	m = catalogAction(t, m, "filter", "active", nil)
	p = m.BridgeState().Panel
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: p.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	if request := next.(Model).BridgeState().SessionRequest; request == nil || request.ConversationID != "work-catalog-00" {
		t.Fatal("opened wrong search result", request)
	}
	c, err := s.Load("work-catalog-00")
	if err != nil || c.Title != "A chosen title" || c.HeadRevision != "rev-body" || c.Archived {
		t.Fatal("presentation changed retained execution", c, err)
	}
}

func TestDesktopCatalogIgnoresOldAsyncSearchAndCancelsWhenClosed(t *testing.T) {
	var contexts []context.Context
	m := New(Config{BridgeMode: true, QueryConversations: func(ctx context.Context, q worksession.ConversationQuery) (worksession.ConversationPage, error) {
		contexts = append(contexts, ctx)
		return worksession.ConversationPage{Items: []worksession.ConversationItem{{ID: "work-" + q.Text, Title: q.Text, UpdatedAt: time.Now()}}}, nil
	}})
	next, first := m.NavigateBridge("/conversations")
	m = next.(Model)
	old := first()
	next, newer := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "query", Text: "latest"}, tea.KeyMsg{})
	m = next.(Model)
	if contexts[0].Err() == nil {
		t.Fatal("superseded read was not cancelled")
	}
	next, _ = m.Update(old)
	m = next.(Model)
	if !m.catalog.Busy || len(m.entries) != 0 {
		t.Fatal("old search replaced newer state")
	}
	next, _ = m.Update(newer())
	m = next.(Model)
	if len(m.entries) != 1 || m.entries[0].title != "latest" {
		t.Fatal("new result missing")
	}
	next, pending := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "refresh"}, tea.KeyMsg{})
	m = next.(Model)
	_ = pending()
	next, _ = m.NavigateBridge("/chat")
	m = next.(Model)
	if contexts[len(contexts)-1].Err() == nil || m.catalog != nil || m.launcher {
		t.Fatal("closed view retained query authority")
	}
}

func TestDesktopCatalogRetainsPendingChangeAfterLeavingTheView(t *testing.T) {
	changes := 0
	m := New(Config{BridgeMode: true, QueryConversations: func(context.Context, worksession.ConversationQuery) (worksession.ConversationPage, error) {
		return worksession.ConversationPage{Items: []worksession.ConversationItem{{ID: "work-one", Title: "One", Version: "reviewed", UpdatedAt: time.Now()}}}, nil
	}, OrganizeConversation: func(change worksession.ConversationChange) (worksession.Conversation, error) {
		changes++
		if change.ID != "work-one" || change.Version != "reviewed" {
			t.Fatal("wrong reviewed mutation", change)
		}
		return worksession.Conversation{ID: change.ID, Title: "One", Pinned: true}, nil
	}})
	next, cmd := m.NavigateBridge("/conversations")
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	row := 0
	next, pending := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "pin", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if pending == nil || !m.BridgeState().CatalogBusy || changes != 0 {
		t.Fatal("change was not retained before dispatch")
	}
	next, _ = m.NavigateBridge("/chat")
	m = next.(Model)
	if !m.BridgeState().CatalogBusy || m.catalog != nil {
		t.Fatal("leaving the view abandoned an admitted metadata change")
	}
	next, _ = m.Update(pending())
	m = next.(Model)
	if m.BridgeState().CatalogBusy || changes != 1 || m.launcher {
		t.Fatal("completed change did not release its runtime")
	}
}
