package worksession

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
)

var creationIdentity = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{15,95}$`)

type chatCreation struct {
	Version        int       `json:"version"`
	RequestID      string    `json:"request_id"`
	ConversationID string    `json:"conversation_id"`
	SourceSHA256   string    `json:"source_sha256"`
	CreatedAt      time.Time `json:"created_at"`
	Publishing     bool      `json:"publishing"`
}

// CreateChatOnce creates an empty chat for a durable client request. The caller
// holds the profile guard, as for CreateChat. A process lock serializes retries.
// Publish a complete directory only after saving its receipt: a missing published
// directory is never recreated after deletion. No source capture or work runs.
func (s Store) CreateChatOnce(source, requestID string, now time.Time) (Conversation, error) {
	if !creationIdentity.MatchString(requestID) || !filepath.IsAbs(source) {
		return Conversation{}, errors.New("invalid new-chat request")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return Conversation{}, err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".creation-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Conversation{}, err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return Conversation{}, err
	}
	defer filelock.Unlock(lock)
	if err := creationDirectory(root, ".creations"); err != nil {
		return Conversation{}, err
	}
	sum := sha256.Sum256([]byte(requestID))
	key := hex.EncodeToString(sum[:])
	sourceSum := sha256.Sum256([]byte(source))
	sourceHash := hex.EncodeToString(sourceSum[:])
	receiptPath, stage := filepath.Join(".creations", key+".json"), filepath.Join(".creations", "stage-"+key)
	var receipt chatCreation
	err = readCreationJSON(root, receiptPath, &receipt)
	if errors.Is(err, os.ErrNotExist) {
		if now.IsZero() {
			now = time.Now()
		}
		receipt = chatCreation{Version: 1, RequestID: requestID, ConversationID: newID("work"), SourceSHA256: sourceHash, CreatedAt: now.UTC()}
		if err := writeCreationJSON(root, receiptPath, receipt); err != nil {
			return Conversation{}, err
		}
	} else if err != nil {
		return Conversation{}, err
	}
	if receipt.Version != 1 || receipt.RequestID != requestID || !catalogIdentity.MatchString(receipt.ConversationID) || receipt.SourceSHA256 != sourceHash || receipt.CreatedAt.IsZero() {
		return Conversation{}, errors.New("new-chat request changed or its receipt is invalid")
	}
	// Keep the durable retry receipt free of source paths and conversation text.
	// The source is supplied by the configured host and checked against its digest.
	c := Conversation{Version: Version, ID: receipt.ConversationID, Title: "New chat", SourcePath: source, CreatedAt: receipt.CreatedAt, UpdatedAt: receipt.CreatedAt}
	if !receipt.Publishing {
		if _, err := root.Lstat(c.ID); !errors.Is(err, os.ErrNotExist) {
			return Conversation{}, errors.New("new-chat identity already exists")
		}
		if err := creationDirectory(root, stage); err != nil {
			return Conversation{}, err
		}
		if err := creationDirectory(root, filepath.Join(stage, "revisions")); err != nil {
			return Conversation{}, err
		}
		if err := writeCreationJSON(root, filepath.Join(stage, "conversation.json"), c); err != nil {
			return Conversation{}, err
		}
		receipt.Publishing = true
		if err := writeCreationJSON(root, receiptPath, receipt); err != nil {
			return Conversation{}, err
		}
	}
	info, err := root.Lstat(c.ID)
	if errors.Is(err, os.ErrNotExist) {
		// The staged directory survives interruption before rename. Once rename
		// succeeded, its absence means we must not recreate a subsequently deleted chat.
		staged, stageErr := root.Lstat(stage)
		if stageErr != nil || !staged.IsDir() {
			return Conversation{}, errors.New("the created chat is no longer available; start a new request")
		}
		if err := root.Rename(stage, c.ID); err != nil {
			return Conversation{}, err
		}
		for _, path := range []string{".", ".creations"} {
			if err := syncCreationDirectory(root, path); err != nil {
				return Conversation{}, err
			}
		}
	} else if err != nil {
		return Conversation{}, err
	} else if !info.IsDir() {
		return Conversation{}, errors.New("created chat is not a directory")
	}
	var current Conversation
	if err := readCreationJSON(root, filepath.Join(c.ID, "conversation.json"), &current); err != nil {
		return Conversation{}, err
	}
	if current.ID != c.ID || !current.CreatedAt.Equal(c.CreatedAt) {
		return Conversation{}, errors.New("created chat identity changed")
	}
	return current, nil
}

func creationDirectory(root *os.Root, name string) error {
	if err := root.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("chat creation directory is not an ordinary directory")
	}
	return syncCreationDirectory(root, filepath.Dir(name))
}

func readCreationJSON(root *os.Root, name string, value any) error {
	file, err := filelock.OpenRootFile(root, name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 32*1024 {
		return errors.New("chat creation metadata is too large")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 32*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid chat creation metadata")
	}
	return nil
}

func writeCreationJSON(root *os.Root, name string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(name), "."+newID("creation"))
	file, err := filelock.OpenRootFile(root, temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err := file.Write(append(payload, '\n')); err != nil {
		file.Close()
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := root.Rename(temporary, name); err != nil {
		return err
	}
	return syncCreationDirectory(root, filepath.Dir(name))
}

func syncCreationDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}
