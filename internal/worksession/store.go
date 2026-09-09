// Package worksession stores durable, non-Git Work conversations and revisions.
package worksession

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/projectcapture"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Version = 2

type Conversation struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	SourcePath   string    `json:"source_path"`
	SnapshotID   string    `json:"snapshot_id"`
	HeadRevision string    `json:"head_revision_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ReplayState struct {
	Version           int             `json:"version"`
	Provider          string          `json:"provider,omitempty"`
	Messages          []agent.Message `json:"messages,omitempty"`
	Configuration     json.RawMessage `json:"configuration,omitempty"`
	CompactionVersion int             `json:"compaction_version,omitempty"`
	Summary           string          `json:"summary,omitempty"`
}

type Revision struct {
	AcceptedCode     map[string]string      `json:"accepted_code,omitempty"`
	Project          *projectcapture.Bundle `json:"project_configuration,omitempty"`
	Replay           *ReplayState           `json:"replay,omitempty"`
	Version          int                    `json:"version"`
	ID               string                 `json:"id"`
	ConversationID   string                 `json:"conversation_id"`
	ParentRevisionID string                 `json:"parent_revision_id,omitempty"`
	SnapshotID       string                 `json:"snapshot_id"`
	Objective        string                 `json:"objective"`
	BundlePath       string                 `json:"bundle_path"`
	Status           string                 `json:"status"`
	FinalText        string                 `json:"final_text,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
}

type Store struct{ root string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("work session state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(absolute, "gator", "conversations")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create conversation store: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return Store{}, err
	}
	return Store{root: root}, nil
}

func (s Store) Create(title, sourcePath, snapshotID string, now time.Time) (Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New work"
	}
	if len(title) > 256 || strings.TrimSpace(sourcePath) == "" || !strings.HasPrefix(snapshotID, "snap-") {
		return Conversation{}, errors.New("conversation identity is invalid")
	}
	if now.IsZero() {
		now = time.Now()
	}
	conversation := Conversation{Version: Version, ID: newID("work"), Title: title, SourcePath: sourcePath, SnapshotID: snapshotID, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if err := os.MkdirAll(s.revisionsPath(conversation.ID), 0o700); err != nil {
		return Conversation{}, err
	}
	if err := s.writeConversation(conversation); err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func (s Store) Load(id string) (Conversation, error) {
	if err := validID(id); err != nil {
		return Conversation{}, err
	}
	var conversation Conversation
	if err := readJSON(filepath.Join(s.root, id, "conversation.json"), &conversation); err != nil {
		return Conversation{}, err
	}
	if (conversation.Version != 1 && conversation.Version != Version) || conversation.ID != id {
		return Conversation{}, errors.New("conversation metadata is invalid")
	}
	return conversation, nil
}

func (s Store) List(limit int) ([]Conversation, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var conversations []Conversation
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		conversation, err := s.Load(entry.Name())
		if err == nil {
			conversations = append(conversations, conversation)
		}
	}
	sort.Slice(conversations, func(i, j int) bool { return conversations[i].UpdatedAt.After(conversations[j].UpdatedAt) })
	if limit > 0 && len(conversations) > limit {
		conversations = conversations[:limit]
	}
	return conversations, nil
}

// AddRevision publishes an immutable revision and advances the movable head.
func (s Store) AddRevision(conversationID string, revision Revision) (Conversation, error) {
	conversation, err := s.Load(conversationID)
	if err != nil {
		return Conversation{}, err
	}
	if revision.Version == 0 {
		revision.Version = Version
	}
	if revision.ID == "" {
		revision.ID = newID("rev")
	}
	revision.ConversationID = conversationID
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = time.Now().UTC()
	}
	if err := validateRevision(revision); err != nil {
		return Conversation{}, err
	}
	if revision.ParentRevisionID != "" {
		parent, err := s.LoadRevision(conversationID, revision.ParentRevisionID)
		if err != nil || parent.ConversationID != conversationID {
			return Conversation{}, errors.New("revision parent does not belong to the conversation")
		}
	}
	path := filepath.Join(s.revisionsPath(conversationID), revision.ID+".json")
	if _, err := os.Lstat(path); err == nil {
		return Conversation{}, fmt.Errorf("revision %q already exists", revision.ID)
	}
	if err := writeJSON(path, revision); err != nil {
		return Conversation{}, err
	}
	conversation.HeadRevision = revision.ID
	conversation.SnapshotID = revision.SnapshotID
	conversation.UpdatedAt = revision.CreatedAt
	if err := s.writeConversation(conversation); err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func (s Store) LoadRevision(conversationID, revisionID string) (Revision, error) {
	if err := validID(conversationID); err != nil {
		return Revision{}, err
	}
	if err := validID(revisionID); err != nil {
		return Revision{}, err
	}
	var revision Revision
	if err := readJSON(filepath.Join(s.revisionsPath(conversationID), revisionID+".json"), &revision); err != nil {
		return Revision{}, err
	}
	if err := validateRevision(revision); err != nil || revision.ID != revisionID || revision.ConversationID != conversationID {
		return Revision{}, errors.New("revision metadata is invalid")
	}
	return revision, nil
}

func (s Store) Revisions(conversationID string) ([]Revision, error) {
	if _, err := s.Load(conversationID); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.revisionsPath(conversationID))
	if err != nil {
		return nil, err
	}
	var revisions []Revision
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		revision, err := s.LoadRevision(conversationID, strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			revisions = append(revisions, revision)
		}
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].CreatedAt.Before(revisions[j].CreatedAt) })
	return revisions, nil
}

func (s Store) Children(conversationID, revisionID string) ([]Revision, error) {
	revisions, err := s.Revisions(conversationID)
	if err != nil {
		return nil, err
	}
	children := make([]Revision, 0)
	for _, revision := range revisions {
		if revision.ParentRevisionID == revisionID {
			children = append(children, revision)
		}
	}
	return children, nil
}

func (s Store) MoveHead(conversationID, revisionID string) (Conversation, error) {
	conversation, err := s.Load(conversationID)
	if err != nil {
		return Conversation{}, err
	}
	revision, err := s.LoadRevision(conversationID, revisionID)
	if err != nil {
		return Conversation{}, err
	}
	conversation.HeadRevision = revision.ID
	conversation.SnapshotID = revision.SnapshotID
	conversation.UpdatedAt = time.Now().UTC()
	if err := s.writeConversation(conversation); err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func (s Store) writeConversation(conversation Conversation) error {
	if err := os.MkdirAll(filepath.Join(s.root, conversation.ID), 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.root, conversation.ID, "conversation.json"), conversation)
}

func (s Store) revisionsPath(id string) string { return filepath.Join(s.root, id, "revisions") }

func validateRevision(revision Revision) error {
	if (revision.Version != 1 && revision.Version != Version) || validID(revision.ID) != nil || validID(revision.ConversationID) != nil || !strings.HasPrefix(revision.SnapshotID, "snap-") || strings.TrimSpace(revision.Objective) == "" || strings.TrimSpace(revision.BundlePath) == "" || strings.TrimSpace(revision.Status) == "" {
		return errors.New("revision metadata is invalid")
	}
	if revision.ParentRevisionID != "" && validID(revision.ParentRevisionID) != nil {
		return errors.New("revision parent is invalid")
	}
	return nil
}

func validID(id string) error {
	if id == "" || len(id) > 96 || strings.ContainsAny(id, "/\\\x00\r\n") {
		return errors.New("invalid Work identity")
	}
	return nil
}

func newID(prefix string) string {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	return prefix + "-" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(value)
}

func readJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".work-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
