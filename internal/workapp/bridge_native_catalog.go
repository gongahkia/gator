package workapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

// Native catalog operations never enter the launcher, change an owner, or admit work.
type NativeCatalogAction struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Text      string `json:"text,omitempty"`
	Scope     string `json:"scope,omitempty"`
	After     string `json:"after,omitempty"`
	PageID    string `json:"page_id,omitempty"`
	ID        string `json:"id,omitempty"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
	CancelID  string `json:"cancel_id,omitempty"`
}
type NativeCatalogResult struct {
	RequestID string                        `json:"request_id"`
	Action    string                        `json:"action"`
	PageID    string                        `json:"page_id,omitempty"`
	Page      *worksession.ConversationPage `json:"page,omitempty"`
	Item      *worksession.ConversationItem `json:"item,omitempty"`
	Error     string                        `json:"error,omitempty"`
	Conflict  bool                          `json:"conflict,omitempty"`
}
type nativeCatalogState struct {
	query  NativeCatalogAction
	page   worksession.ConversationPage
	pageID string
	cancel context.CancelFunc
	action NativeCatalogAction
	acting bool
}
type nativeCatalogLoaded struct {
	request NativeCatalogAction
	page    worksession.ConversationPage
	err     error
}
type nativeCatalogChanged struct {
	request NativeCatalogAction
	item    worksession.Conversation
	err     error
}

func (m *Model) cloneNativeCatalog() {
	if m.nativeCatalog == nil {
		m.nativeCatalog = &nativeCatalogState{}
		return
	}
	c := *m.nativeCatalog
	c.page.Items = append([]worksession.ConversationItem(nil), c.page.Items...)
	m.nativeCatalog = &c
}
func (m *Model) nativeCatalogIdentity() {
	data, _ := json.Marshal(struct {
		Request NativeCatalogAction
		Items   []worksession.ConversationItem
	}{m.nativeCatalog.query, m.nativeCatalog.page.Items})
	m.nativeCatalog.pageID = fmt.Sprintf("%x", sha256.Sum256(data))
}
func nativeCatalogItem(c worksession.Conversation) worksession.ConversationItem {
	return worksession.ConversationItem{ID: c.ID, Title: c.Title, Source: c.SourcePath, UpdatedAt: c.UpdatedAt, Pinned: c.Pinned, Archived: c.Archived, Version: worksession.ConversationDisplayVersion(c)}
}

func (m Model) ApplyNativeCatalog(a NativeCatalogAction) (tea.Model, tea.Cmd) {
	if !branchIdentity.MatchString(a.RequestID) || len(a.RequestID) > 64 {
		return m, nil
	}
	m.cloneNativeCatalog()
	result := &NativeCatalogResult{RequestID: a.RequestID, Action: a.Action}
	fail := func(message string) (tea.Model, tea.Cmd) {
		result.Error = message
		if a.Action == "query" || a.Action == "cancel" {
			m.nativeCatalogQueryResult = result
		} else {
			m.nativeCatalogActionResult = result
		}
		return m, nil
	}
	if !m.config.ManagedSessions || m.config.QueryConversations == nil || m.config.SessionStore == nil {
		return fail("The bundled Go owner does not support the native conversation catalog.")
	}
	switch a.Action {
	case "cancel":
		if a.CancelID == m.nativeCatalog.query.RequestID {
			if m.nativeCatalog.cancel != nil {
				m.nativeCatalog.cancel()
			}
			m.nativeCatalog.query = NativeCatalogAction{}
			m.nativeCatalog.pageID = ""
			m.nativeCatalog.page = worksession.ConversationPage{}
		}
		return m, nil
	case "query":
		if a.RequestID == m.nativeCatalog.query.RequestID {
			if a == m.nativeCatalog.query {
				return m, nil
			}
			return fail("Catalog request identity was reused. Refresh with a new request.")
		}
		if m.nativeCatalog.acting {
			return fail("Wait for the selected conversation operation to finish.")
		}
		if len(a.Text) > 512 || len(a.After) > 1024 || (a.Scope != "active" && a.Scope != "archived" && a.Scope != "all") {
			return fail("Check the conversation search and scope.")
		}
		if m.nativeCatalog.cancel != nil {
			m.nativeCatalog.cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		m.nativeCatalog.cancel = cancel
		m.nativeCatalog.query = a
		m.nativeCatalog.pageID = ""
		m.nativeCatalog.page = worksession.ConversationPage{}
		m.nativeCatalogQueryResult = nil
		query := worksession.ConversationQuery{Text: a.Text, Scope: a.Scope, After: a.After, Content: true, Limit: 50}
		load := m.config.QueryConversations
		return m, func() tea.Msg { page, err := load(ctx, query); return nativeCatalogLoaded{a, page, err} }
	case "verify":
		// The destination owner verifies its own canonical metadata after startup.
		if a.ID != m.conversation || a.Version == "" {
			return fail("The destination owner does not match the catalog selection.")
		}
		c, err := m.config.SessionStore.Load(a.ID)
		if err != nil {
			return fail("This conversation is unavailable: " + err.Error())
		}
		if worksession.ConversationDisplayVersion(c) != a.Version {
			return fail("This conversation changed. Refresh and choose it again.")
		}
		item := nativeCatalogItem(c)
		result.Item = &item
		m.nativeCatalogActionResult = result
		return m, nil
	case "open", "inspect", "rename":
		if m.nativeCatalog.acting {
			if a == m.nativeCatalog.action {
				return m, nil
			}
			return fail("Wait for the selected conversation operation to finish.")
		}
		if a == m.nativeCatalog.action && m.nativeCatalogActionResult != nil && m.nativeCatalogActionResult.RequestID == a.RequestID {
			return m, nil
		}
		if a.RequestID == m.nativeCatalog.action.RequestID {
			return fail("Conversation request identity was reused. Refresh and review.")
		}
		if a.PageID == "" || a.PageID != m.nativeCatalog.pageID {
			return fail("These catalog results changed. Refresh and choose the conversation again.")
		}
		found := false
		var selected worksession.ConversationItem
		for _, item := range m.nativeCatalog.page.Items {
			if item.ID == a.ID && item.Version == a.Version {
				found = true
				selected = item
				break
			}
		}
		if !found {
			return fail("The selected conversation is no longer in this catalog page.")
		}
		if a.Action == "rename" {
			if m.config.OrganizeConversation == nil {
				return fail("Conversation rename is unavailable.")
			}
			m.nativeCatalog.action = a
			m.nativeCatalog.acting = true
			m.nativeCatalogActionResult = nil
			organize := m.config.OrganizeConversation
			return m, func() tea.Msg {
				c, err := organize(worksession.ConversationChange{ID: a.ID, Version: a.Version, Action: "rename", Title: a.Title})
				return nativeCatalogChanged{a, c, err}
			}
		}
		c, err := m.config.SessionStore.Load(a.ID)
		if err != nil {
			return fail("This conversation is unavailable: " + err.Error())
		}
		if a.Action == "open" && (worksession.ConversationDisplayVersion(c) != a.Version || c.SourcePath != selected.Source) {
			result.Conflict = true
			return fail("This conversation changed. Refresh and choose it again.")
		}
		item := nativeCatalogItem(c)
		result.Item = &item
		if a.Action == "inspect" {
			item = m.replaceNativeCatalogItem(item)
			result.PageID = m.nativeCatalog.pageID
		}
		m.nativeCatalog.action = a
		m.nativeCatalogActionResult = result
		return m, nil
	default:
		return fail("Unsupported native catalog operation.")
	}
}
func (m *Model) replaceNativeCatalogItem(item worksession.ConversationItem) worksession.ConversationItem {
	for i, prior := range m.nativeCatalog.page.Items {
		if prior.ID == item.ID {
			item.Snippet, item.Match = prior.Snippet, prior.Match
			m.nativeCatalog.page.Items[i] = item
		}
	}
	m.nativeCatalogIdentity()
	return item
}
func (m Model) finishNativeCatalogLoad(v nativeCatalogLoaded) (tea.Model, tea.Cmd) {
	if m.nativeCatalog == nil || m.nativeCatalog.query != v.request {
		return m, nil
	}
	m.cloneNativeCatalog()
	result := &NativeCatalogResult{RequestID: v.request.RequestID, Action: "query"}
	if m.nativeCatalog.cancel != nil {
		m.nativeCatalog.cancel()
		m.nativeCatalog.cancel = nil
	}
	if v.err != nil {
		result.Error = v.err.Error()
	} else {
		m.nativeCatalog.page = v.page
		m.nativeCatalogIdentity()
		result.Page = &m.nativeCatalog.page
		result.PageID = m.nativeCatalog.pageID
	}
	m.nativeCatalogQueryResult = result
	return m, nil
}
func (m Model) finishNativeCatalogChange(v nativeCatalogChanged) (tea.Model, tea.Cmd) {
	if m.nativeCatalog == nil || m.nativeCatalog.action != v.request {
		return m, nil
	}
	m.cloneNativeCatalog()
	m.nativeCatalog.acting = false
	result := &NativeCatalogResult{RequestID: v.request.RequestID, Action: "rename"}
	if v.err != nil {
		result.Error = v.err.Error()
		result.Conflict = errors.Is(v.err, worksession.ErrConversationChanged)
	} else {
		item := nativeCatalogItem(v.item)
		result.Item = &item
		item = m.replaceNativeCatalogItem(item)
		result.PageID = m.nativeCatalog.pageID
		if v.item.ID == m.conversation {
			m.title = v.item.Title
		}
		m.applyNativeConversationMetadata(v.item)
	}
	m.nativeCatalogActionResult = result
	return m, nil
}

// Refresh display metadata only; never focus, reload history or replay a request.
func (m Model) RefreshBridgeConversationTitles(id string) Model {
	if m.config.SessionStore != nil {
		if c, err := m.config.SessionStore.Load(id); err == nil {
			m.applyNativeConversationMetadata(c)
		}
	}
	return m
}
func (m *Model) applyNativeConversationMetadata(c worksession.Conversation) {
	if c.ID == m.conversation {
		m.title = c.Title
	}
	items := append([]worksession.Conversation(nil), m.config.Conversations...)
	for i, item := range items {
		if item.ID == c.ID {
			items[i] = c
		}
	}
	m.config.Conversations = items
}
