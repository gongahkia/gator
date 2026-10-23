package worksession

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/workspace"
)

const catalogPageSize = 50
const catalogRecords = 10000
const catalogMetadataBytes = 16 * 1024 * 1024
const catalogSearchBytes = 32 * 1024 * 1024

var catalogIdentity = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)

type ConversationQuery struct {
	Text    string
	Scope   string // active (default), archived, or all
	Content bool   // Include retained user/assistant text, never private replay/options.
	After   string
	Limit   int
}
type ConversationPage struct {
	Items      []ConversationItem `json:"items"`
	Next       string             `json:"next,omitempty"`
	Incomplete bool               `json:"incomplete,omitempty"`
	Skipped    int                `json:"skipped,omitempty"`
}
type ConversationItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
	Pinned    bool      `json:"pinned"`
	Archived  bool      `json:"archived"`
	Version   string    `json:"version"`
	Match     string    `json:"match,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
}
type catalogCursor struct {
	Query  string    `json:"query"`
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Pinned bool      `json:"pinned"`
}

func ConversationDisplayVersion(c Conversation) string {
	payload, _ := json.Marshal(struct {
		ID, Title                     string
		Pinned, Archived, TitleEdited bool
		Revision                      uint64
	}{c.ID, c.Title, c.Pinned, c.Archived, c.TitleEdited, c.MetadataRevision})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
func normalizeConversationQuery(q ConversationQuery) (ConversationQuery, error) {
	q.Text = strings.TrimSpace(q.Text)
	if len(q.Text) > 512 || !utf8.ValidString(q.Text) || strings.IndexFunc(q.Text, unicode.IsControl) >= 0 || len(q.After) > 1024 {
		return q, errors.New("invalid conversation search")
	}
	if q.Scope == "" {
		q.Scope = "active"
	}
	if q.Scope != "active" && q.Scope != "archived" && q.Scope != "all" {
		return q, errors.New("choose active, archived or all conversations")
	}
	if q.Limit == 0 {
		q.Limit = catalogPageSize
	}
	if q.Limit < 1 || q.Limit > catalogPageSize {
		return q, errors.New("conversation pages contain at most 50 items")
	}
	return q, nil
}
func catalogEarlier(a, b catalogCursor) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	if !a.At.Equal(b.At) {
		return a.At.After(b.At)
	}
	return a.ID < b.ID
}
func cursorFor(c Conversation) catalogCursor {
	return catalogCursor{ID: c.ID, At: c.UpdatedAt, Pinned: c.Pinned}
}

// PageConversations does bounded local reads, never starts work or gives a
// renderer replay state. The anchor is stable as newer chats arrive above it.
func (s Store) PageConversations(query ConversationQuery) (ConversationPage, error) {
	q, err := normalizeConversationQuery(query)
	if err != nil {
		return ConversationPage{}, err
	}
	identity, _ := json.Marshal(struct {
		Text, Scope string
		Content     bool
	}{q.Text, q.Scope, q.Content})
	sum := sha256.Sum256(identity)
	key := hex.EncodeToString(sum[:])
	var after catalogCursor
	if q.After != "" {
		data, err := base64.RawURLEncoding.DecodeString(q.After)
		if err != nil || json.Unmarshal(data, &after) != nil || after.Query != key || !catalogIdentity.MatchString(after.ID) || after.At.IsZero() {
			return ConversationPage{}, errors.New("the conversation page does not match this search")
		}
	}
	result := ConversationPage{Items: []ConversationItem{}}
	root, err := workspace.Open(s.root)
	if err != nil {
		return result, err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return result, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(catalogRecords + 1)
	if err != nil && err != io.EOF {
		return result, err
	}
	if len(entries) > catalogRecords {
		entries = entries[:catalogRecords]
		result.Incomplete = true
	}
	var records []Conversation
	bytesRead := 0
	for _, entry := range entries {
		if !entry.IsDir() || !catalogIdentity.MatchString(entry.Name()) {
			continue
		}
		data, err := root.ReadRegularFile(filepath.Join(entry.Name(), "conversation.json"), 32*1024)
		bytesRead += len(data)
		if bytesRead > catalogMetadataBytes {
			result.Incomplete = true
			break
		}
		var c Conversation
		if err != nil || json.Unmarshal(data, &c) != nil || (c.Version != 1 && c.Version != Version) || c.ID != entry.Name() || len(c.Title) > 256 || len(c.SourcePath) > 8192 || c.UpdatedAt.IsZero() {
			result.Skipped++
			continue
		}
		n, displayErr := readConversationDisplay(root, &c)
		bytesRead += n
		if bytesRead > catalogMetadataBytes {
			result.Incomplete = true
			break
		}
		if displayErr != nil {
			result.Skipped++
			continue
		}
		if q.Scope == "active" && c.Archived || q.Scope == "archived" && !c.Archived {
			continue
		}
		if q.After != "" && !catalogEarlier(after, cursorFor(c)) {
			continue
		}
		records = append(records, c)
	}
	sort.Slice(records, func(i, j int) bool { return catalogEarlier(cursorFor(records[i]), cursorFor(records[j])) })
	term := strings.ToLower(q.Text)
	remaining := catalogSearchBytes
	for _, c := range records {
		item := ConversationItem{ID: c.ID, Title: c.Title, Source: c.SourcePath, UpdatedAt: c.UpdatedAt, Pinned: c.Pinned, Archived: c.Archived, Version: ConversationDisplayVersion(c)}
		if term != "" {
			if strings.Contains(strings.ToLower(c.Title), term) {
				item.Match = "title"
			} else if strings.Contains(strings.ToLower(c.SourcePath), term) {
				item.Match = "folder"
			} else if q.Content {
				snippet, partial := s.matchConversationText(root, c, term, &remaining)
				result.Incomplete = result.Incomplete || partial
				if snippet == "" {
					continue
				}
				item.Match = "messages"
				item.Snippet = snippet
			} else {
				continue
			}
		}
		if len(result.Items) == q.Limit {
			last := result.Items[len(result.Items)-1]
			cursor := catalogCursor{Query: key, ID: last.ID, At: last.UpdatedAt, Pinned: last.Pinned}
			data, _ := json.Marshal(cursor)
			result.Next = base64.RawURLEncoding.EncodeToString(data)
			break
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s Store) matchConversationText(root workspace.Root, c Conversation, term string, remaining *int) (string, bool) {
	partial := false
	match := func(text string) string {
		lower := strings.ToLower(text)
		at := strings.Index(lower, term)
		if at < 0 {
			return ""
		}
		// Case folding can change byte widths; clipping uses rune positions from
		// the actual text and never slices through a UTF-8 sequence.
		textRunes := []rune(text)
		before := len([]rune(lower[:at]))
		start := max(0, before-48)
		end := min(len(textRunes), before+len([]rune(term))+160)
		if start > len(textRunes) {
			start = 0
		}
		if end < start {
			end = start
		}
		snippet := strings.Join(strings.Fields(string(textRunes[start:end])), " ")
		if start > 0 {
			snippet = "…" + snippet
		}
		if end < len(textRunes) {
			snippet += "…"
		}
		return snippet
	}
	read := func(path string) ([]byte, error) {
		if *remaining <= 0 {
			return nil, errors.New("search byte budget reached")
		}
		limit := min(*remaining, 2*1024*1024)
		data, err := root.ReadRegularFile(path, int64(limit))
		*remaining -= len(data)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			*remaining -= limit
		}
		return data, err
	}
	// A first interrupted request may have no revision yet. Only text leaves
	// this loader; captured model choices, upload grants and replay are omitted.
	requests, err := read(filepath.Join(c.ID, "requests.json"))
	if err == nil {
		var journal struct {
			Requests []struct {
				Text  string `json:"text"`
				State string `json:"state"`
			} `json:"requests"`
		}
		if json.Unmarshal(requests, &journal) == nil {
			for i := len(journal.Requests) - 1; i >= 0; i-- {
				if journal.Requests[i].State == "discarded" {
					continue
				}
				if snippet := match(journal.Requests[i].Text); snippet != "" {
					return snippet, partial
				}
			}
		} else {
			partial = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		partial = true
	}
	id := c.HeadRevision
	seen := map[string]bool{}
	for i := 0; id != "" && i < 64; i++ {
		if !catalogIdentity.MatchString(id) || seen[id] {
			return "", true
		}
		seen[id] = true
		data, err := read(filepath.Join(c.ID, "revisions", id+".json"))
		if err != nil {
			return "", true
		}
		var revision struct {
			ConversationID string `json:"conversation_id"`
			Parent         string `json:"parent_revision_id"`
			Objective      string `json:"objective"`
			FinalText      string `json:"final_text"`
		}
		if json.Unmarshal(data, &revision) != nil || revision.ConversationID != c.ID {
			return "", true
		}
		for _, text := range []string{revision.Objective, revision.FinalText} {
			if snippet := match(text); snippet != "" {
				return snippet, partial
			}
		}
		id = revision.Parent
	}
	return "", partial || id != ""
}

type ConversationChange struct{ ID, Version, Action, Title string }

var ErrConversationChanged = errors.New("this conversation changed; refresh it before organizing it")

// OrganizeConversation changes presentation metadata under the same lock as
// revision/head updates. Archiving never deletes a chat, its memory or work.
func (s Store) OrganizeConversation(change ConversationChange) (Conversation, error) {
	if !catalogIdentity.MatchString(change.ID) {
		return Conversation{}, errors.New("invalid conversation identity")
	}
	lock, err := s.requestLock(change.ID, ".metadata.lock", false)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	c, err := s.Load(change.ID)
	if err != nil {
		return c, err
	}
	if change.Version == "" || ConversationDisplayVersion(c) != change.Version {
		return c, ErrConversationChanged
	}
	if c.MetadataRevision == math.MaxUint64 {
		return c, errors.New("conversation metadata version exhausted")
	}
	switch change.Action {
	case "rename":
		title := strings.TrimSpace(change.Title)
		if title == "" || len(title) > 256 || !utf8.ValidString(title) || strings.IndexFunc(title, unicode.IsControl) >= 0 {
			return c, errors.New("use a title of 1–256 bytes on one line")
		}
		c.Title, c.TitleEdited = title, true
	case "pin":
		c.Pinned = true
	case "unpin":
		c.Pinned = false
	case "archive":
		c.Archived = true
	case "restore":
		c.Archived = false
	default:
		return c, fmt.Errorf("unknown conversation change %q", change.Action)
	}
	c.MetadataRevision++
	display := conversationDisplay{Version: 1, Revision: c.MetadataRevision, Pinned: c.Pinned, Archived: c.Archived, Title: c.Title, TitleEdited: c.TitleEdited}
	if err := writeJSON(filepath.Join(s.root, c.ID, "display.json"), display); err != nil {
		return Conversation{}, err
	}
	// Best-effort title mirror for older releases. The sidecar is authoritative
	// after its atomic save, so a mirror failure cannot report a rejected edit.
	_ = s.writeConversation(c)
	return c, nil
}

type conversationDisplay struct {
	Version     int    `json:"version"`
	Revision    uint64 `json:"revision"`
	Pinned      bool   `json:"pinned,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
	Title       string `json:"title,omitempty"`
	TitleEdited bool   `json:"title_edited,omitempty"`
}

func (s Store) loadConversationDisplay(c *Conversation) (int, error) {
	root, err := workspace.Open(s.root)
	if err != nil {
		return 0, err
	}
	return readConversationDisplay(root, c)
}
func readConversationDisplay(root workspace.Root, c *Conversation) (int, error) {
	if !catalogIdentity.MatchString(c.ID) {
		return 0, errors.New("invalid conversation display identity")
	}
	data, err := root.ReadRegularFile(filepath.Join(c.ID, "display.json"), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read conversation organization: %w", err)
	}
	var display conversationDisplay
	if json.Unmarshal(data, &display) != nil || display.Version != 1 || display.Revision == 0 || len(display.Title) > 256 || !utf8.ValidString(display.Title) || display.TitleEdited && (strings.TrimSpace(display.Title) == "" || strings.IndexFunc(display.Title, unicode.IsControl) >= 0) {
		return len(data), errors.New("conversation organization is invalid")
	}
	c.Pinned, c.Archived, c.TitleEdited, c.MetadataRevision = display.Pinned, display.Archived, display.TitleEdited, display.Revision
	if display.TitleEdited {
		c.Title = display.Title
	}
	return len(data), nil
}
