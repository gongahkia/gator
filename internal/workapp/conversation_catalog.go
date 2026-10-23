package workapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/worksession"
)

type conversationCatalog struct {
	Query    worksession.ConversationQuery
	Page     worksession.ConversationPage
	Anchors  []string
	Busy     bool
	Mutating bool
	Issue    string
	Notice   string
}
type catalogLoaded struct {
	Generation uint64
	Page       worksession.ConversationPage
	Err        error
	Changed    *worksession.Conversation
}

func (m *Model) cloneCatalog() {
	if m.catalog == nil {
		return
	}
	copy := *m.catalog
	copy.Anchors = append([]string(nil), copy.Anchors...)
	m.catalog = &copy
}
func (m *Model) cancelCatalog() {
	if m.catalogCancel != nil {
		m.catalogCancel()
		m.catalogCancel = nil
	}
	m.catalogSequence++
	m.catalog = nil
}
func (m *Model) queryCatalog(change ...worksession.ConversationChange) tea.Cmd {
	if m.catalog == nil || m.config.QueryConversations == nil {
		return nil
	}
	m.cloneCatalog()
	if m.catalogCancel != nil {
		m.catalogCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.catalogCancel = cancel
	m.catalogSequence++
	generation, query, load, organize := m.catalogSequence, m.catalog.Query, m.config.QueryConversations, m.config.OrganizeConversation
	m.catalog.Busy, m.catalog.Mutating, m.catalog.Issue = true, len(change) > 0, ""
	if len(change) > 0 {
		m.catalogSaving = generation
	}
	if len(change) == 0 {
		m.catalog.Page = worksession.ConversationPage{}
	}
	m.entries, m.selected = nil, 0
	return func() tea.Msg {
		result := catalogLoaded{Generation: generation}
		if len(change) > 0 {
			if organize == nil {
				return catalogLoaded{Generation: generation, Err: errorsUnavailable("conversation organization")}
			}
			c, err := organize(change[0])
			if err != nil {
				return catalogLoaded{Generation: generation, Err: err}
			}
			result.Changed = &c
		}
		result.Page, result.Err = load(ctx, query)
		return result
	}
}
func (m Model) finishCatalog(result catalogLoaded) (tea.Model, tea.Cmd) {
	if result.Generation == m.catalogSaving {
		m.catalogSaving = 0
	}
	if result.Changed != nil {
		if result.Changed.ID == m.conversation {
			m.title = result.Changed.Title
		}
		m.refreshBridgeConversations()
	}
	if m.catalog == nil || result.Generation != m.catalogSequence || !m.launcher || m.launcherMode != "conversations" {
		return m, nil
	}
	m.cloneCatalog()
	m.catalog.Busy, m.catalog.Mutating = false, false
	if m.catalogCancel != nil {
		m.catalogCancel()
		m.catalogCancel = nil
	}
	if result.Err != nil {
		m.catalog.Issue = result.Err.Error()
		// Keep the reviewed rows after a rejected organization change, so an
		// in-progress title stays editable. Its stored version still guards CAS.
		m.entries = nil
		for _, item := range m.catalog.Page.Items {
			m.entries = append(m.entries, entry{title: item.Title, subtitle: item.Source, source: item.Source, id: item.ID, kind: "conversation"})
		}
		return m, nil
	}
	m.catalog.Page, m.catalog.Issue = result.Page, ""
	m.entries = make([]entry, 0, len(result.Page.Items))
	for _, item := range result.Page.Items {
		m.entries = append(m.entries, entry{title: item.Title, subtitle: item.Source, source: item.Source, id: item.ID, kind: "conversation"})
	}
	if result.Changed != nil {
		m.catalog.Notice = "Conversation updated."
	}
	return m, nil
}

func (m Model) catalogPanel() uisnapshot.Panel {
	c := m.catalog
	identity, _ := json.Marshal(struct {
		Generation uint64
		Query      worksession.ConversationQuery
		Items      []worksession.ConversationItem
	}{m.catalogSequence, c.Query, c.Page.Items})
	p := uisnapshot.Panel{Kind: "picker", Title: "Conversations", Query: c.Query.Text, Selected: m.selected, ActionIdentity: string(identity),
		Catalog: &uisnapshot.Catalog{Loading: c.Busy, Page: len(c.Anchors) + 1, HasNext: c.Page.Next != "", HasPrevious: len(c.Anchors) > 0, CanOrganize: m.config.OrganizeConversation != nil},
		Filter:  &uisnapshot.Filter{Value: c.Query.Scope, Options: []uisnapshot.Option{{Value: "active", Label: "Active"}, {Value: "archived", Label: "Archived"}, {Value: "all", Label: "All chats"}}}}
	for _, item := range c.Page.Items {
		if c.Busy {
			break
		}
		p.Rows = append(p.Rows, uisnapshot.Row{Identity: item.ID + ":" + item.Version, Label: item.Title, Source: item.Source, Detail: item.Snippet, UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), Pinned: item.Pinned, Archived: item.Archived})
	}
	if c.Issue != "" {
		p.Notice, p.NoticeTone = c.Issue, "error"
	} else {
		var notes []string
		if c.Notice != "" {
			notes = append(notes, c.Notice)
		}
		if c.Page.Incomplete || c.Page.Skipped > 0 {
			notes = append(notes, "Search coverage is incomplete. Some records changed, could not be read, or exceed the bounded index. Refresh to continue indexing.")
		}
		if c.Page.IndexFallback {
			notes = append(notes, "The chat index is unavailable. Showing results from retained conversation files.")
		}
		p.Notice = strings.Join(notes, " ")
	}
	return p
}

func (m Model) applyCatalogPanel(event BridgePanelAction) (tea.Model, tea.Cmd) {
	if len(event.Fields) > 0 {
		return m, nil
	}
	m.cloneCatalog()
	c := m.catalog
	if c.Mutating || m.catalogSaving != 0 {
		m.bridgeNotice = "Wait for this conversation change to finish."
		return m, nil
	}
	switch event.Action {
	case "query":
		c.Query.Text = event.Text
		c.Query.After = ""
		c.Anchors = nil
		c.Notice = ""
	case "filter":
		if event.Text != "active" && event.Text != "archived" && event.Text != "all" {
			return m, nil
		}
		c.Query.Scope = event.Text
		c.Query.After = ""
		c.Anchors = nil
		c.Notice = ""
	case "next":
		if c.Busy || c.Page.Next == "" {
			return m, nil
		}
		c.Anchors = append(c.Anchors, c.Query.After)
		c.Query.After = c.Page.Next
	case "previous":
		if c.Busy || len(c.Anchors) == 0 {
			return m, nil
		}
		c.Query.After = c.Anchors[len(c.Anchors)-1]
		c.Anchors = c.Anchors[:len(c.Anchors)-1]
	case "refresh":
	case "rename", "pin", "unpin", "archive", "restore":
		if c.Busy || event.Row == nil || *event.Row < 0 || *event.Row >= len(c.Page.Items) || m.config.OrganizeConversation == nil {
			return m, nil
		}
		item := c.Page.Items[*event.Row]
		change := worksession.ConversationChange{ID: item.ID, Version: item.Version, Action: event.Action, Title: event.Text}
		cmd := m.queryCatalog(change)
		return m, cmd
	default:
		m.bridgeNotice = fmt.Sprintf("Unknown conversation action %q.", event.Action)
		return m, nil
	}
	cmd := m.queryCatalog()
	return m, cmd
}
