package speech

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
)

const LocalProvider = "whisper-local"
const DefaultLocalModel = "base"

// Immutable upstream revision; sizes and SHA-256 come from its LFS metadata.
const whisperRevision = "5359861c739e955e79d9a303bcbc70fb988958b1"

type LocalModel struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"-"`
	Available bool   `json:"available"`
}

var whisperModels = []LocalModel{
	{ID: "tiny", Label: "Tiny · faster, lower accuracy", Bytes: 77691713, SHA256: "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21"},
	{ID: "base", Label: "Base · balanced", Bytes: 147951465, SHA256: "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe"},
	{ID: "small", Label: "Small · higher capacity, more memory", Bytes: 487601967, SHA256: "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b"},
}

func LocalModels() []LocalModel { return append([]LocalModel(nil), whisperModels...) }
func IsLocalModel(id string) bool {
	for _, m := range whisperModels {
		if m.ID == id {
			return true
		}
	}
	return false
}

type LocalStore struct {
	directory string
	catalog   []LocalModel
	client    *http.Client
}

func OpenLocalStore(stateDir string) (*LocalStore, error) {
	if err := privatefs.MkdirAll(stateDir); err != nil {
		return nil, err
	}
	base, err := os.OpenRoot(stateDir)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	relative := ""
	for _, part := range []string{"gator", "speech", "models"} {
		relative = filepath.Join(relative, part)
		if err := base.Mkdir(relative, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err := base.Lstat(relative)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("local voice storage must use ordinary private directories")
		}
	}
	directory := filepath.Join(stateDir, relative)
	if err := privatefs.Protect(directory); err != nil {
		return nil, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	return &LocalStore{directory: directory, catalog: LocalModels()}, nil
}
func (s *LocalStore) model(id string) (LocalModel, error) {
	for _, m := range s.catalog {
		if m.ID == id {
			return m, nil
		}
	}
	return LocalModel{}, errors.New("choose a listed local Whisper model")
}
func modelFilename(id string) string { return "ggml-" + id + ".bin" }
func (s *LocalStore) Models() []LocalModel {
	result := append([]LocalModel(nil), s.catalog...)
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return result
	}
	defer root.Close()
	for i, m := range result {
		info, err := root.Lstat(modelFilename(m.ID))
		result[i].Available = err == nil && info.Mode().IsRegular() && info.Size() == m.Bytes
	}
	return result
}

// VerifiedModel refuses links, incomplete downloads and changed weights before
// any recorded audio is handed to a native process. No model code is executed.
func (s *LocalStore) VerifiedModel(ctx context.Context, id string) (string, error) {
	model, err := s.model(id)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, modelFilename(id), os.O_RDONLY, 0600)
	if err != nil {
		return "", errors.New("download the selected Whisper model in Voice settings first")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != model.Bytes {
		return "", errors.New("local voice model is incomplete; download it again")
	}
	digest := sha256.New()
	count, err := io.Copy(digest, io.LimitReader(contextReader{ctx: ctx, reader: file}, model.Bytes+1))
	if err != nil {
		return "", err
	}
	if count != model.Bytes || hex.EncodeToString(digest.Sum(nil)) != model.SHA256 {
		return "", errors.New("local voice model failed its checksum; download it again")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return filepath.Join(s.directory, modelFilename(id)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

// Download is an explicit UI operation. Catalog IDs cannot supply URLs, paths,
// credentials or executable code. Inference never invokes this automatically.
func (s *LocalStore) Download(ctx context.Context, id string) error {
	model, err := s.model(id)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return err
	}
	defer root.Close()
	lease, err := filelock.OpenRootFile(root, ".download.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lease.Close()
	if err := filelock.Lock(lease, filelock.Exclusive|filelock.Nonblocking); err != nil {
		return errors.New("another voice model download is already running")
	}
	defer filelock.Unlock(lease)
	// Only this explicit download, under its cross-process lease, removes
	// abandoned chunks from a prior interrupted download. Never follow links.
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".download-") || len(name) != len(".download-")+24 {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(name, ".download-")); err != nil {
			continue
		}
		info, err := root.Lstat(name)
		if err == nil && info.Mode().IsRegular() {
			if err := root.Remove(name); err != nil {
				return err
			}
		}
	}
	if info, err := root.Lstat(modelFilename(id)); err == nil && !info.Mode().IsRegular() {
		return errors.New("local model destination must be an ordinary file")
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temporary := ".download-" + hex.EncodeToString(nonce[:])
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { file.Close(); _ = root.Remove(temporary) }()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://huggingface.co/ggerganov/whisper.cpp/resolve/"+whisperRevision+"/"+modelFilename(id), nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 10 * time.Minute}
	if s.client != nil {
		client = *s.client
	}
	client.CheckRedirect = func(next *http.Request, previous []*http.Request) error {
		host := next.URL.Hostname()
		if len(previous) > 8 || next.URL.Scheme != "https" || next.URL.User != nil || !(host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co") || host == "hf.co" || strings.HasSuffix(host, ".hf.co")) {
			return errors.New("voice model redirect is not allowed")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("voice model download could not connect; try again")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("voice model download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > model.Bytes {
		return errors.New("voice model download exceeds its reviewed size")
	}
	digest := sha256.New()
	count, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(response.Body, model.Bytes+1))
	if err != nil {
		return errors.New("voice model download was interrupted; try again")
	}
	if count != model.Bytes || hex.EncodeToString(digest.Sum(nil)) != model.SHA256 {
		return errors.New("voice model download failed its size/checksum check")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(temporary, modelFilename(id)); err != nil {
		return err
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}

// Binding contributes only to an opaque recording route, never browser-visible
// paths. A changed runtime/model file invalidates an in-progress recording.
func (s *LocalStore) Binding(id, executable string) string {
	value := whisperRevision + "\x00" + id + "\x00" + executable
	for _, path := range []string{filepath.Join(s.directory, modelFilename(id)), executable} {
		if info, err := os.Lstat(path); err == nil {
			value += fmt.Sprintf("\x00%d/%d/%s", info.Size(), info.ModTime().UnixNano(), info.Mode())
		}
	}
	return value
}
