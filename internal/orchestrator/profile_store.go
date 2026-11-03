package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
)

var ErrAgentProfileChanged = errors.New("specialist definition changed; inspect it and try again")

type ProfileStore struct{ directory string }

func NewProfileStore(stateDir string) (ProfileStore, error) {
	if strings.TrimSpace(stateDir) == "" {
		return ProfileStore{}, errors.New("specialist profile state directory is required")
	}
	root, err := filepath.Abs(stateDir)
	return ProfileStore{directory: filepath.Join(root, "gator", "agent-profiles")}, err
}

func (s ProfileStore) read() ([]AgentDefinition, error) {
	root, err := s.open(false)
	if errors.Is(err, os.ErrNotExist) {
		return []AgentDefinition{}, nil
	}
	if err != nil {
		return nil, errors.New("specialist library is unavailable")
	}
	defer root.Close()
	return readProfileDocument(root)
}

func (s ProfileStore) open(create bool) (*os.Root, error) {
	if s.directory == "" {
		return nil, errors.New("specialist library is not configured")
	}
	parentPath, name := filepath.Dir(s.directory), filepath.Base(s.directory)
	if create {
		if err := privatefs.MkdirAll(parentPath); err != nil {
			return nil, err
		}
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if create {
		if err := parent.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if err := syncProfileDirectory(parent); err != nil {
			return nil, err
		}
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("specialist directory must not be a link")
	}
	return parent.OpenRoot(name)
}

func readProfileDocument(root *os.Root) ([]AgentDefinition, error) {
	info, err := root.Lstat("profiles.json")
	if errors.Is(err, os.ErrNotExist) {
		return []AgentDefinition{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, errors.New("invalid specialist library file")
	}
	file, err := root.Open("profiles.json")
	if err != nil {
		return nil, errors.New("specialist library is unreadable")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return nil, errors.New("specialist library exceeds its read limit")
	}
	var document struct {
		Version  int               `json:"version"`
		Profiles []AgentDefinition `json:"profiles"`
	}
	if err := decodeTask(data, &document); err != nil || document.Version != 1 || len(document.Profiles) > MaxAgentProfiles {
		return nil, errors.New("invalid specialist library")
	}
	seen := map[string]bool{}
	for _, definition := range document.Profiles {
		if definition.Validate() != nil || definition != definition.Normalize() || seen[definition.Name] {
			return nil, errors.New("invalid specialist library definition")
		}
		seen[definition.Name] = true
	}
	return document.Profiles, nil
}

func (s ProfileStore) List() ([]AgentDefinition, error) { return s.read() }

// Put uses content revision CAS. An empty expected digest creates only an
// absent name; concurrent chats cannot silently overwrite one another.
func (s ProfileStore) Put(definition AgentDefinition, expected string) error {
	return s.PutContext(context.Background(), definition, expected)
}

func (s ProfileStore) PutContext(ctx context.Context, definition AgentDefinition, expected string) error {
	definition = definition.Normalize()
	if err := definition.Validate(); err != nil {
		return err
	}
	return s.change(ctx, definition.Name, expected, &definition)
}

func (s ProfileStore) Delete(name, expected string) error {
	return s.DeleteContext(context.Background(), name, expected)
}

func (s ProfileStore) DeleteContext(ctx context.Context, name, expected string) error {
	if !specialistNamePattern.MatchString(name) || expected == "" {
		return errors.New("inspect the saved specialist before deleting it")
	}
	return s.change(ctx, name, expected, nil)
}

func (s ProfileStore) change(ctx context.Context, name, expected string, next *AgentDefinition) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.directory == "" {
		return errors.New("specialist library is not configured")
	}
	root, err := s.open(true)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".profiles.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		err := filelock.Lock(lock, filelock.Exclusive|filelock.Nonblocking)
		if err == nil {
			break
		}
		if !errors.Is(err, filelock.ErrWouldBlock) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("specialist library is busy; try again")
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer filelock.Unlock(lock)
	profiles, err := readProfileDocument(root)
	if err != nil {
		return err
	}
	index := -1
	for i, profile := range profiles {
		if profile.Name == name {
			index = i
			break
		}
	}
	if index < 0 && expected != "" || index >= 0 && profiles[index].Digest() != expected {
		return ErrAgentProfileChanged
	}
	if next == nil {
		if index < 0 {
			return ErrAgentProfileChanged
		}
		profiles = append(profiles[:index], profiles[index+1:]...)
	} else if index >= 0 {
		profiles[index] = *next
	} else {
		if len(profiles) >= MaxAgentProfiles {
			return errors.New("saved specialist limit reached")
		}
		profiles = append(profiles, *next)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	data, err := json.MarshalIndent(struct {
		Version  int               `json:"version"`
		Profiles []AgentDefinition `json:"profiles"`
	}{1, profiles}, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("saved specialist definitions exceed the 1 MiB library limit")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	nameTemp := ".profiles-" + hex.EncodeToString(random[:]) + ".tmp"
	file, err := root.OpenFile(nameTemp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(nameTemp)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Rename(nameTemp, "profiles.json"); err != nil {
		return err
	}
	return syncProfileDirectory(root)
}

func syncProfileDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}
