package worksession

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

// DialogueTaskOrigin is provenance for a separately owned execution. It grants
// no tools or approvals and contains no replay or credentials. Its hashes bind
// the child's admission to the original turn's immutable input envelope.
type DialogueTaskOrigin struct {
	Version              int       `json:"version"`
	ParentConversationID string    `json:"parent_conversation_id"`
	TurnID               string    `json:"turn_id"`
	TurnSequence         uint64    `json:"turn_sequence"`
	ChildConversationID  string    `json:"child_conversation_id"`
	OptionsSHA256        string    `json:"options_sha256"`
	TextSHA256           string    `json:"text_sha256"`
	SourceSHA256         string    `json:"source_sha256"`
	CreatedAt            time.Time `json:"created_at"`
}

func dialogueHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validDialogueID(id string) bool {
	if len(id) != 64 || strings.ToLower(id) != id {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// ReadDialogue looks up one exact admitted turn. It never advances, retries or
// dispatches it. Duplicated identities in storage fail closed.
func (s Store) ReadDialogue(conversation, id string) (DialogueTurn, error) {
	if !validDialogueID(id) {
		return DialogueTurn{}, errors.New("invalid conversation message identity")
	}
	root, err := s.dialogueRoot(conversation, false)
	if err != nil {
		return DialogueTurn{}, err
	}
	defer root.Close()
	names, err := dialogueNames(root)
	if err != nil {
		return DialogueTurn{}, err
	}
	var match string
	for _, name := range names {
		if strings.HasSuffix(name, "-"+id+".json") {
			if match != "" {
				return DialogueTurn{}, errors.New("conversation message identity is duplicated")
			}
			match = name
		}
	}
	if match == "" {
		return DialogueTurn{}, os.ErrNotExist
	}
	return readDialogue(root, match, conversation)
}

func dialogueTaskDecision(data json.RawMessage) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(data, &value) != nil || len(value) != 1 {
		return false
	}
	var kind string
	return json.Unmarshal(value["kind"], &kind) == nil && kind == "task"
}

func taskCreationKey(parent, turn string) string {
	return "dialogue-task-" + dialogueHash([]byte(parent+"\x00"+turn))
}

func taskOrigin(parent Conversation, turn DialogueTurn, child Conversation) DialogueTaskOrigin {
	return DialogueTaskOrigin{Version: 1, ParentConversationID: parent.ID, TurnID: turn.ID, TurnSequence: turn.Sequence, ChildConversationID: child.ID,
		OptionsSHA256: dialogueHash(turn.Options), TextSHA256: dialogueHash([]byte(turn.Text)), SourceSHA256: dialogueHash([]byte(parent.SourcePath)), CreatedAt: child.CreatedAt}
}

func (s Store) retainTaskOrigin(origin DialogueTaskOrigin) error {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(origin.ChildConversationID)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("task owner is not an ordinary conversation")
	}
	child, err := root.OpenRoot(origin.ChildConversationID)
	if err != nil {
		return err
	}
	defer child.Close()
	lock, err := filelock.OpenRootFile(child, ".task-origin-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return err
	}
	defer filelock.Unlock(lock)
	var prior DialogueTaskOrigin
	err = readCreationJSON(child, "task-origin.json", &prior)
	if errors.Is(err, os.ErrNotExist) {
		return writeCreationJSON(child, "task-origin.json", origin)
	}
	if err != nil {
		return err
	}
	if prior != origin {
		return errors.New("task origin changed; do not retarget its captured inputs")
	}
	return nil
}

// BindDialogueTask creates at most one empty child owner and then durably links
// it to the ready turn. claimed is true only for the winning ready->dispatching
// transition; a retry is an inspection, never permission to launch again. The
// caller holds the ordinary profile guard used for conversation creation.
func (s Store) BindDialogueTask(conversation, id, expectedSource string, expected uint64, now time.Time) (DialogueTurn, Conversation, bool, error) {
	turn, err := s.ReadDialogue(conversation, id)
	if err != nil {
		return turn, Conversation{}, false, err
	}
	parent, err := s.Load(conversation)
	if err != nil {
		return turn, Conversation{}, false, err
	}
	if expectedSource == "" || parent.SourcePath != expectedSource || !dialogueTaskDecision(turn.Decision) {
		return turn, Conversation{}, false, errors.New("the task's original source or routing decision changed")
	}
	if turn.TaskChat != "" && (turn.State == "dispatching" || turn.State == "delegated") {
		child, err := s.Load(turn.TaskChat)
		if err == nil {
			var origin *DialogueTaskOrigin
			origin, err = s.DialogueTask(turn.TaskChat)
			if err == nil && origin == nil {
				err = errors.New("the bound task origin is missing")
			}
		}
		return turn, child, false, err
	}
	if turn.State != "ready" || turn.Revision != expected || expected == 0 {
		return turn, Conversation{}, false, ErrDialogueChanged
	}
	if now.IsZero() {
		now = time.Now()
	}
	if now.Before(turn.UpdatedAt) {
		now = turn.UpdatedAt
	}
	child, err := s.CreateChatOnce(parent.SourcePath, taskCreationKey(parent.ID, turn.ID), now)
	if err != nil {
		return turn, child, false, err
	}
	if child.SourcePath != parent.SourcePath {
		return turn, child, false, errors.New("task owner changed its workspace")
	}
	if err := s.retainTaskOrigin(taskOrigin(parent, turn, child)); err != nil {
		return turn, child, false, err
	}
	turn.State, turn.TaskChat, turn.UpdatedAt = "dispatching", child.ID, now.UTC()
	bound, err := s.UpdateDialogue(turn, expected)
	if errors.Is(err, ErrDialogueChanged) {
		current, readErr := s.ReadDialogue(conversation, id)
		if readErr == nil && current.TaskChat == child.ID && (current.State == "dispatching" || current.State == "delegated") && bytes.Equal(current.Decision, turn.Decision) {
			origin, originErr := s.DialogueTask(child.ID)
			readErr = originErr
			if readErr == nil && origin == nil {
				readErr = errors.New("the bound task origin is missing")
			}
			return current, child, false, readErr
		}
	}
	return bound, child, err == nil, err
}

// DialogueTask validates both ends of a child link before the host may project
// controls or prepare its admission. Ordinary chats have no such origin.
func (s Store) DialogueTask(childID string) (*DialogueTaskOrigin, error) {
	child, err := s.Load(childID)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var origin DialogueTaskOrigin
	err = readCreationJSON(root, filepath.Join(childID, "task-origin.json"), &origin)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if origin.Version != 1 || origin.ChildConversationID != childID || validID(origin.ParentConversationID) != nil || origin.ParentConversationID == childID || !validDialogueID(origin.TurnID) || origin.TurnSequence == 0 || !origin.CreatedAt.Equal(child.CreatedAt) {
		return nil, errors.New("invalid task origin")
	}
	parent, err := s.Load(origin.ParentConversationID)
	if err != nil {
		return nil, err
	}
	dialogue, err := s.dialogueRoot(parent.ID, false)
	if err != nil {
		return nil, err
	}
	defer dialogue.Close()
	turn, err := readDialogue(dialogue, fmt.Sprintf("%020d-%s.json", origin.TurnSequence, origin.TurnID), parent.ID)
	if err != nil {
		return nil, err
	}
	if turn.TaskChat != childID || !dialogueTaskDecision(turn.Decision) || parent.SourcePath != child.SourcePath || origin != taskOrigin(parent, turn, child) {
		return nil, errors.New("task origin does not match its original message and inputs")
	}
	return &origin, nil
}
