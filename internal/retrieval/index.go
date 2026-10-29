package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

// Document is eligible saved knowledge after its owner has checked scope,
// expiry, and source freshness. No text is stored in the vector index.
type Document struct{ ID, Text string }
type Hit struct {
	ID         string
	Similarity float64
}
type indexEntry struct {
	Hash   string    `json:"hash"`
	Vector []float32 `json:"vector"`
}
type indexFile struct {
	Version  int                   `json:"version"`
	Embedder string                `json:"embedder"`
	Entries  map[string]indexEntry `json:"entries"`
}

// Search incrementally updates one provider/scope index, then returns semantic
// candidates. A caller can continue lexically when this returns an error.
func Search(ctx context.Context, stateDir, scope, query string, documents []Document, embedder Embedder, limit int) ([]Hit, error) {
	if embedder == nil || strings.TrimSpace(query) == "" {
		return nil, errors.New("semantic search is unavailable")
	}
	if len(documents) > 4096 {
		return nil, errors.New("semantic index is above its bounded size")
	}
	root, err := indexRoot(stateDir)
	if err != nil {
		return nil, err
	}
	unlock, err := lockIndex(ctx, root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	name := indexName(embedder.ID(), scope)
	path := filepath.Join(root, name)
	index := indexFile{Version: 1, Embedder: embedder.ID(), Entries: map[string]indexEntry{}}
	if info, statErr := os.Stat(path); statErr == nil {
		if info.Size() > 32<<20 {
			return nil, errors.New("semantic index exceeds 32 MiB")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(data, &index) != nil || index.Version != 1 || index.Embedder != embedder.ID() || index.Entries == nil {
			index = indexFile{Version: 1, Embedder: embedder.ID(), Entries: map[string]indexEntry{}}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	current := map[string]bool{}
	missingKeys, missingText := []string{}, []string{}
	for _, document := range documents {
		if document.ID == "" || len(document.ID) > 128 {
			return nil, errors.New("invalid semantic document ID")
		}
		if _, err := os.Stat(forgottenPath(root, document.ID)); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for part, chunk := range chunks(document.Text) {
			key := fmt.Sprintf("%s#%d", document.ID, part)
			current[key] = true
			digest := sha256.Sum256([]byte(chunk))
			hash := hex.EncodeToString(digest[:])
			if prior, ok := index.Entries[key]; ok && prior.Hash == hash && len(prior.Vector) > 0 {
				continue
			}
			missingKeys = append(missingKeys, key)
			missingText = append(missingText, chunk)
			index.Entries[key] = indexEntry{Hash: hash}
		}
	}
	removed := false
	for key := range index.Entries {
		if !current[key] {
			delete(index.Entries, key)
			removed = true
		}
	}
	if len(missingKeys) > 0 {
		for start := 0; start < len(missingKeys); start += 16 {
			end := min(start+16, len(missingKeys))
			vectors, err := embedder.Embed(ctx, missingText[start:end])
			if err != nil {
				return nil, err
			}
			if len(vectors) != end-start {
				return nil, errors.New("semantic embedding count mismatch")
			}
			for i, vector := range vectors {
				if err := validVector(vector); err != nil {
					return nil, err
				}
				entry := index.Entries[missingKeys[start+i]]
				entry.Vector = vector
				index.Entries[missingKeys[start+i]] = entry
			}
		}
		if err := saveIndex(path, index); err != nil {
			return nil, err
		}
	} else if removed {
		if err := saveIndex(path, index); err != nil {
			return nil, err
		}
	}
	if len(index.Entries) == 0 {
		return nil, nil
	}
	vectors, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, errors.New("semantic query embedding count mismatch")
	}
	if err := validVector(vectors[0]); err != nil {
		return nil, err
	}
	best := map[string]float64{}
	for key, entry := range index.Entries {
		id, _, _ := strings.Cut(key, "#")
		if len(entry.Vector) != len(vectors[0]) {
			continue
		}
		score := cosine(entry.Vector, vectors[0])
		if score > best[id] {
			best[id] = score
		}
	}
	hits := make([]Hit, 0, len(best))
	for id, score := range best {
		if score >= 0.45 {
			hits = append(hits, Hit{id, score})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Similarity == hits[j].Similarity {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Similarity > hits[j].Similarity
	})
	if limit < 0 {
		limit = 0
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func chunks(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	result := []string{}
	for start := 0; start < len(runes); {
		end := min(start+1500, len(runes))
		result = append(result, string(runes[start:end]))
		if end == len(runes) {
			break
		}
		start = end - 150
	}
	return result
}

func validVector(vector []float32) error {
	if len(vector) < 16 || len(vector) > 4096 {
		return errors.New("invalid embedding dimension")
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("invalid embedding value")
		}
	}
	return nil
}
func cosine(a, b []float32) float64 {
	var dot, aa, bb float64
	for i, value := range a {
		x, y := float64(value), float64(b[i])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}
func indexRoot(stateDir string) (string, error) {
	if stateDir == "" {
		return "", errors.New("semantic state directory is required")
	}
	root := filepath.Join(stateDir, "gator", "retrieval")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return "", err
	}
	return root, nil
}

func lockIndex(ctx context.Context, root string) (func(), error) {
	file, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := filelock.Lock(file, filelock.Exclusive|filelock.Nonblocking); err == nil {
			return func() { _ = filelock.Unlock(file); _ = file.Close() }, nil
		} else if !errors.Is(err, filelock.ErrWouldBlock) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func indexName(embedder, scope string) string {
	hash := sha256.Sum256([]byte(embedder + "\x00" + scope))
	return hex.EncodeToString(hash[:]) + ".json"
}
func forgottenPath(root, id string) string {
	hash := sha256.Sum256([]byte(id))
	return filepath.Join(root, ".forgotten-"+hex.EncodeToString(hash[:]))
}
func saveIndex(path string, index indexFile) error {
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("semantic index exceeds 32 MiB")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".index-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// DropID removes a forgotten record's vectors from every backend/scope index.
func DropID(stateDir, id string) error {
	return clearID(stateDir, id, true)
}

// InvalidateID clears cached vectors for a live record without excluding it
// from later searches. Call only after saving the authoritative record, while
// holding its owner's mutation lock. This also repairs deletion markers left
// by older save/correction paths; a genuinely forgotten record must use DropID.
func InvalidateID(stateDir, id string) error {
	return clearID(stateDir, id, false)
}

func clearID(stateDir, id string, forgotten bool) error {
	if id == "" || len(id) > 128 {
		return errors.New("invalid semantic document ID")
	}
	root, err := indexRoot(stateDir)
	if err != nil {
		return err
	}
	unlock, err := lockIndex(context.Background(), root)
	if err != nil {
		return err
	}
	defer unlock()
	if forgotten {
		marker, err := os.OpenFile(forgottenPath(root, id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if marker != nil {
			if err := marker.Close(); err != nil {
				return err
			}
		}
	}
	files, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, file.Name())
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Size() > 32<<20 {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var index indexFile
		if json.Unmarshal(data, &index) != nil {
			if err := os.Remove(path); err != nil {
				return err
			}
			continue
		}
		changed := false
		for key := range index.Entries {
			if strings.HasPrefix(key, id+"#") {
				delete(index.Entries, key)
				changed = true
			}
		}
		if changed {
			if err := saveIndex(path, index); err != nil {
				return err
			}
		}
	}
	if !forgotten {
		if err := os.Remove(forgottenPath(root, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Timeout keeps embedding setup from blocking a normal Work turn indefinitely.
func Timeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 15*time.Second)
}
