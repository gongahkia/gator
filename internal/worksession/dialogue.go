package worksession

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/filelock"
)

// DialogueTurn retains conversational admission separately from executing Work
// requests/revisions. Captured inputs and the client identity are immutable.
// A model decision is intent only; dispatching is written before touching a
// live task. A lost owner never grants permission to replay that dispatch.
type DialogueTurn struct {
	Version        int             `json:"version"`
	Revision       uint64          `json:"revision"`
	Sequence       uint64          `json:"sequence"`
	ID             string          `json:"id"`
	ClientID       string          `json:"client_id"`
	ConversationID string          `json:"conversation_id"`
	Text           string          `json:"text"`
	Options        json.RawMessage `json:"options"`
	State          string          `json:"state"`
	Decision       json.RawMessage `json:"decision,omitempty"`
	Response       string          `json:"response,omitempty"`
	TaskChat       string          `json:"task_chat,omitempty"`
	Issue          string          `json:"issue,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

var ErrDialogueChanged = errors.New("the conversation message changed; inspect its current state")

const maxDialogueTurnBytes = 512 * 1024

func dialogueIdentity(clientID string) string {
	sum := sha256.Sum256([]byte(clientID))
	return hex.EncodeToString(sum[:])
}

func dialogueOptions(data json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 || len(data) > 128*1024 {
		return nil, errors.New("invalid captured conversation inputs")
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&object) != nil || object == nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("captured conversation inputs must be a JSON object")
	}
	return json.Marshal(object)
}

func dialogueFilename(turn DialogueTurn) string {
	return fmt.Sprintf("%020d-%s.json", turn.Sequence, turn.ID)
}

func validDialogueFilename(name string) bool {
	if len(name) != 90 || name[20] != '-' || !strings.HasSuffix(name, ".json") {
		return false
	}
	sequence, err := strconv.ParseUint(name[:20], 10, 64)
	_, hexErr := hex.DecodeString(name[21:85])
	return err == nil && sequence > 0 && hexErr == nil
}

func validateDialogueTurn(turn DialogueTurn) error {
	if turn.Version != 1 || turn.Revision == 0 || turn.Sequence == 0 || !creationIdentity.MatchString(turn.ClientID) || turn.ID != dialogueIdentity(turn.ClientID) || validID(turn.ConversationID) != nil || !utf8.ValidString(turn.Text) || strings.TrimSpace(turn.Text) == "" || len(turn.Text) > 128*1024 || len(turn.Options) > 128*1024 || len(turn.Options) == 0 || turn.Options[0] != '{' || !json.Valid(turn.Options) || len(turn.Response) > 64*1024 || !utf8.ValidString(turn.Response) || len(turn.Decision) > 96*1024 || len(turn.Decision) != 0 && !json.Valid(turn.Decision) || len(turn.Issue) > 4096 || turn.CreatedAt.IsZero() || turn.UpdatedAt.Before(turn.CreatedAt) || turn.TaskChat != "" && validID(turn.TaskChat) != nil {
		return errors.New("invalid retained conversation message")
	}
	switch turn.State {
	case "received", "routing":
		if len(turn.Decision) != 0 || turn.Response != "" || turn.TaskChat != "" || turn.Issue != "" {
			return errors.New("unrouted conversation message cannot claim an outcome")
		}
	case "ready", "dispatching":
		if len(turn.Decision) == 0 {
			return errors.New("conversation dispatch requires its retained decision")
		}
	case "clarification", "answered":
		if strings.TrimSpace(turn.Response) == "" {
			return errors.New("conversation reply is missing")
		}
	case "guided", "stopped", "delegated":
		if len(turn.Decision) == 0 || strings.TrimSpace(turn.Response) == "" || turn.State == "delegated" && turn.TaskChat == "" {
			return errors.New("conversation action requires its decision and acknowledgement")
		}
	case "failed", "interrupted":
		if strings.TrimSpace(turn.Issue) == "" {
			return errors.New("conversation interruption reason is missing")
		}
	default:
		return errors.New("invalid conversation message state")
	}
	return nil
}

func (store Store) dialogueRoot(conversation string, create bool) (*os.Root, error) {
	if _, err := store.Load(conversation); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(store.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	path := filepath.Join(conversation, "dialogue")
	if create {
		if err := creationDirectory(root, path); err != nil {
			return nil, err
		}
	}
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("conversation dialogue is not an ordinary directory")
	}
	return root.OpenRoot(path)
}

func dialogueNames(root *os.Root) ([]string, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if validDialogueFilename(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func readDialogue(root *os.Root, name, conversation string) (DialogueTurn, error) {
	var turn DialogueTurn
	file, err := filelock.OpenRootFile(root, name, os.O_RDONLY, 0)
	if err != nil {
		return turn, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDialogueTurnBytes+1))
	if err != nil || len(data) > maxDialogueTurnBytes {
		return turn, errors.New("conversation message exceeds its file limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&turn) != nil || decoder.Decode(new(any)) != io.EOF || validateDialogueTurn(turn) != nil || turn.ConversationID != conversation || dialogueFilename(turn) != name {
		return DialogueTurn{}, errors.New("conversation message identity or contents are invalid")
	}
	return turn, nil
}

func writeDialogue(root *os.Root, turn DialogueTurn) error {
	data, err := json.Marshal(turn)
	if err != nil || len(data)+1 > maxDialogueTurnBytes {
		return errors.New("conversation message exceeds its file limit")
	}
	return writeCreationJSON(root, dialogueFilename(turn), turn)
}

// AdmitDialogue commits one message before acknowledging it. Retries recover
// the same receipt even while a Work execution lease is held. It cannot mutate
// the Work request journal, replay, head revision or permission envelope.
func (store Store) AdmitDialogue(conversation, clientID, text string, options json.RawMessage, now time.Time) (DialogueTurn, error) {
	options, err := dialogueOptions(options)
	if err != nil {
		return DialogueTurn{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	turn := DialogueTurn{Version: 1, Revision: 1, Sequence: 1, ID: dialogueIdentity(clientID), ClientID: clientID, ConversationID: conversation, Text: text, Options: bytes.Clone(options), State: "received", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	if err := validateDialogueTurn(turn); err != nil {
		return DialogueTurn{}, err
	}
	root, err := store.dialogueRoot(conversation, true)
	if err != nil {
		return DialogueTurn{}, err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return DialogueTurn{}, err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return DialogueTurn{}, err
	}
	defer filelock.Unlock(lock)
	names, err := dialogueNames(root)
	if err != nil {
		return DialogueTurn{}, err
	}
	for _, name := range names {
		if !strings.HasSuffix(name, "-"+turn.ID+".json") {
			continue
		}
		prior, err := readDialogue(root, name, conversation)
		if err != nil {
			return DialogueTurn{}, err
		}
		if prior.ClientID != clientID || prior.Text != text || !bytes.Equal(prior.Options, options) {
			return DialogueTurn{}, errors.New("this message identity already belongs to different text or inputs")
		}
		return prior, nil
	}
	if len(names) > 0 {
		sequence, _ := strconv.ParseUint(names[len(names)-1][:20], 10, 64)
		if sequence == ^uint64(0) {
			return DialogueTurn{}, errors.New("conversation sequence exhausted")
		}
		turn.Sequence = sequence + 1
	}
	if err := writeDialogue(root, turn); err != nil {
		return DialogueTurn{}, err
	}
	return turn, nil
}

// DialoguePage returns a bounded, chronological page before an exclusive cursor.
// Older turns are retained rather than pruned when the visible page fills up.
func (store Store) DialoguePage(conversation string, before uint64, limit int) ([]DialogueTurn, bool, error) {
	if limit < 1 || limit > 100 {
		return nil, false, errors.New("conversation page limit must be between 1 and 100")
	}
	if _, err := store.Load(conversation); err != nil {
		return nil, false, err
	}
	root, err := store.dialogueRoot(conversation, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	defer root.Close()
	names, err := dialogueNames(root)
	if err != nil {
		return nil, false, err
	}
	var result []DialogueTurn
	for index := len(names) - 1; index >= 0; index-- {
		sequence, _ := strconv.ParseUint(names[index][:20], 10, 64)
		if before > 0 && sequence >= before {
			continue
		}
		if len(result) == limit {
			return reverseDialogue(result), true, nil
		}
		turn, err := readDialogue(root, names[index], conversation)
		if err != nil {
			return nil, false, err
		}
		result = append(result, turn)
	}
	return reverseDialogue(result), false, nil
}

func reverseDialogue(turns []DialogueTurn) []DialogueTurn {
	for left, right := 0, len(turns)-1; left < right; left, right = left+1, right-1 {
		turns[left], turns[right] = turns[right], turns[left]
	}
	return turns
}

func dialogueTransition(from, to string) bool {
	switch from {
	case "received":
		return to == "routing" || to == "interrupted" || to == "failed"
	case "routing":
		return to == "ready" || to == "answered" || to == "clarification" || to == "interrupted" || to == "failed"
	case "clarification":
		return to == "answered" || to == "interrupted" || to == "failed"
	case "ready":
		return to == "dispatching" || to == "interrupted" || to == "failed"
	case "dispatching":
		return to == "guided" || to == "delegated" || to == "stopped" || to == "interrupted" || to == "failed"
	default:
		return false
	}
}

// UpdateDialogue is compare-and-swap on one turn. Captured text/options and its
// admission identity cannot be rewritten by routing or completion callbacks.
func (store Store) UpdateDialogue(next DialogueTurn, expected uint64) (DialogueTurn, error) {
	var err error
	next.Options, err = dialogueOptions(next.Options)
	if err != nil {
		return DialogueTurn{}, err
	}
	if err := validateDialogueTurn(next); err != nil || expected == 0 || expected == ^uint64(0) {
		return DialogueTurn{}, errors.New("invalid conversation message update")
	}
	root, err := store.dialogueRoot(next.ConversationID, false)
	if err != nil {
		return DialogueTurn{}, err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return DialogueTurn{}, err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return DialogueTurn{}, err
	}
	defer filelock.Unlock(lock)
	prior, err := readDialogue(root, dialogueFilename(next), next.ConversationID)
	if err != nil {
		return DialogueTurn{}, err
	}
	if prior.Revision != expected || next.Revision != expected || prior.ID != next.ID || prior.ClientID != next.ClientID || prior.Text != next.Text || !bytes.Equal(prior.Options, next.Options) || !prior.CreatedAt.Equal(next.CreatedAt) || next.UpdatedAt.Before(prior.UpdatedAt) {
		return DialogueTurn{}, ErrDialogueChanged
	}
	if !dialogueTransition(prior.State, next.State) {
		return DialogueTurn{}, errors.New("conversation message cannot repeat or revive a completed dispatch")
	}
	next.Revision++
	if err := writeDialogue(root, next); err != nil {
		return DialogueTurn{}, err
	}
	return next, nil
}
